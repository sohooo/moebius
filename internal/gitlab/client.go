package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	jobToken   string
	httpClient *http.Client
}

type Note struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

func New(baseURL, jobToken string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("gitlab base URL is required")
	}
	if jobToken == "" {
		return nil, fmt.Errorf("CI_JOB_TOKEN is required")
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		jobToken: jobToken,
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

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("JOB-TOKEN", c.jobToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("gitlab API %s %s failed with %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
