// Package gitlab provides the GitLab API client and CI target resolution.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	tokenKind  TokenKind
	httpClient *http.Client
}

type TokenKind string

const (
	TokenKindJob     TokenKind = "job"
	TokenKindPrivate TokenKind = "private"
)

type Note struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

type APIError struct {
	Method     string
	Path       string
	Status     string
	StatusCode int
	Body       string
	Hint       string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := fmt.Sprintf("gitlab API %s %s failed with %s: %s", e.Method, e.Path, e.Status, e.Body)
	if e.Hint != "" {
		message += "; " + e.Hint
	}
	return message
}

func New(baseURL, token string, tokenKind TokenKind) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("gitlab base URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("GitLab token is required")
	}
	if tokenKind == "" {
		tokenKind = TokenKindPrivate
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		tokenKind: tokenKind,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *Client) ListMergeRequestNotes(ctx context.Context, projectID, mrIID string) ([]Note, error) {
	var notes []Note
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes", url.PathEscape(projectID), url.PathEscape(mrIID))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

func (c *Client) CreateMergeRequestNote(ctx context.Context, projectID, mrIID, body string) (Note, error) {
	var note Note
	payload := map[string]string{"body": body}
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes", url.PathEscape(projectID), url.PathEscape(mrIID))
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &note); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (c *Client) UpdateMergeRequestNote(ctx context.Context, projectID, mrIID string, noteID int, body string) (Note, error) {
	var note Note
	payload := map[string]string{"body": body}
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes/%d", url.PathEscape(projectID), url.PathEscape(mrIID), noteID)
	if err := c.doJSON(ctx, http.MethodPut, path, payload, &note); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (c *Client) ProbeCreateMergeRequestNoteAccess(ctx context.Context, projectID, mrIID string) error {
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes", url.PathEscape(projectID), url.PathEscape(mrIID))
	statusCode, _, err := c.do(ctx, http.MethodPost, path, map[string]string{})
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil
	default:
		return err
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	_, data, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	switch c.tokenKind {
	case TokenKindJob:
		req.Header.Set("JOB-TOKEN", c.token)
	default:
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gitlab API %s %s request failed: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{
			Method:     method,
			Path:       path,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(data)),
		}
		if c.tokenKind == TokenKindJob && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			apiErr.Hint = "CI_JOB_TOKEN is often read-only for merge request notes, set GITLAB_TOKEN or use --gitlab-token"
		}
		return resp.StatusCode, nil, apiErr
	}
	return resp.StatusCode, data, nil
}
