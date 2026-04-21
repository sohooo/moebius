package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestClientMergeRequestNotesLifecycle(t *testing.T) {
	var seenAuth string
	client, err := New("https://gitlab.example/api/v4", "job-token", TokenKindJob)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenAuth = r.Header.Get("JOB-TOKEN")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/merge_requests/7/notes":
				return jsonResponse(http.StatusOK, `[{"id":5,"body":"hello"}]`), nil
			case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/merge_requests/7/notes":
				return jsonResponse(http.StatusOK, `{"id":6,"body":"created"}`), nil
			case r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/merge_requests/7/notes/5":
				return jsonResponse(http.StatusOK, `{"id":5,"body":"updated"}`), nil
			default:
				return textResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	notes, err := client.ListMergeRequestNotes(context.Background(), "1", "7")
	if err != nil {
		t.Fatalf("ListMergeRequestNotes returned error: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != 5 {
		t.Fatalf("unexpected notes: %#v", notes)
	}

	created, err := client.CreateMergeRequestNote(context.Background(), "1", "7", "body")
	if err != nil {
		t.Fatalf("CreateMergeRequestNote returned error: %v", err)
	}
	if created.ID != 6 {
		t.Fatalf("unexpected created note: %#v", created)
	}

	updated, err := client.UpdateMergeRequestNote(context.Background(), "1", "7", 5, "body")
	if err != nil {
		t.Fatalf("UpdateMergeRequestNote returned error: %v", err)
	}
	if updated.Body != "updated" {
		t.Fatalf("unexpected updated note: %#v", updated)
	}
	if seenAuth != "job-token" {
		t.Fatalf("expected JOB-TOKEN header, got %q", seenAuth)
	}
}

func TestClientMergeRequestDescriptionLifecycle(t *testing.T) {
	var seenAuth string
	var seenDescription string
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenAuth = r.Header.Get("PRIVATE-TOKEN")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/merge_requests/7":
				return jsonResponse(http.StatusOK, `{"description":"current"}`), nil
			case r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/merge_requests/7":
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode payload: %v", err)
				}
				seenDescription = payload["description"]
				return jsonResponse(http.StatusOK, `{"description":"updated"}`), nil
			default:
				return textResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	mr, err := client.GetMergeRequest(context.Background(), "1", "7")
	if err != nil {
		t.Fatalf("GetMergeRequest returned error: %v", err)
	}
	if mr.Description != "current" {
		t.Fatalf("unexpected description: %#v", mr)
	}
	updated, err := client.UpdateMergeRequestDescription(context.Background(), "1", "7", "next")
	if err != nil {
		t.Fatalf("UpdateMergeRequestDescription returned error: %v", err)
	}
	if updated.Description != "updated" {
		t.Fatalf("unexpected updated description: %#v", updated)
	}
	if seenDescription != "next" {
		t.Fatalf("expected description payload, got %q", seenDescription)
	}
	if seenAuth != "private-token" {
		t.Fatalf("expected private token header, got %q", seenAuth)
	}
}

func TestClientProbeUpdateMergeRequestDescriptionAccess(t *testing.T) {
	var methods []string
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			methods = append(methods, r.Method)
			switch r.Method {
			case http.MethodGet:
				return jsonResponse(http.StatusOK, `{"description":"current"}`), nil
			case http.MethodPut:
				return jsonResponse(http.StatusOK, `{"description":"current"}`), nil
			default:
				return textResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	if err := client.ProbeUpdateMergeRequestDescriptionAccess(context.Background(), "1", "7"); err != nil {
		t.Fatalf("ProbeUpdateMergeRequestDescriptionAccess returned error: %v", err)
	}
	if strings.Join(methods, ",") != "GET,PUT" {
		t.Fatalf("expected GET then PUT, got %v", methods)
	}
}

func TestClientUsesPrivateTokenHeader(t *testing.T) {
	var jobToken string
	var privateToken string
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			jobToken = r.Header.Get("JOB-TOKEN")
			privateToken = r.Header.Get("PRIVATE-TOKEN")
			return jsonResponse(http.StatusOK, `[]`), nil
		}),
	}

	if _, err := client.ListMergeRequestNotes(context.Background(), "1", "7"); err != nil {
		t.Fatalf("ListMergeRequestNotes returned error: %v", err)
	}
	if jobToken != "" {
		t.Fatalf("expected no JOB-TOKEN header, got %q", jobToken)
	}
	if privateToken != "private-token" {
		t.Fatalf("expected PRIVATE-TOKEN header, got %q", privateToken)
	}
}

func TestClientReturnsHelpfulAPIError(t *testing.T) {
	client, err := New("https://gitlab.example/api/v4", "job-token", TokenKindJob)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return textResponse(http.StatusForbidden, "forbidden"), nil
		}),
	}

	_, err = client.ListMergeRequestNotes(context.Background(), "1", "7")
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("expected status in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected response body in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "set GITLAB_TOKEN or use --gitlab-token") {
		t.Fatalf("expected token hint in error, got %v", err)
	}
}

func TestClientProbeCreateMergeRequestNoteAccessTreatsValidationErrorAsSuccess(t *testing.T) {
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			return textResponse(http.StatusBadRequest, `{"message":{"body":["is missing"]}}`), nil
		}),
	}

	if err := client.ProbeCreateMergeRequestNoteAccess(context.Background(), "1", "7"); err != nil {
		t.Fatalf("expected validation-style probe success, got %v", err)
	}
}

func TestClientCreateMergeRequestNoteHandlesLargeSuccessResponse(t *testing.T) {
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	largeBody := strings.Repeat("x", 12000)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			return jsonResponse(http.StatusOK, `{"id":6,"body":"`+largeBody+`"}`), nil
		}),
	}

	note, err := client.CreateMergeRequestNote(context.Background(), "1", "7", "body")
	if err != nil {
		t.Fatalf("CreateMergeRequestNote returned error: %v", err)
	}
	if note.ID != 6 {
		t.Fatalf("expected created note id 6, got %#v", note)
	}
	if got := strconv.Itoa(len(note.Body)); got != "12000" {
		t.Fatalf("expected body length 12000, got %s", got)
	}
}

func TestClientListMergeRequestNotesHandlesLargeResponse(t *testing.T) {
	client, err := New("https://gitlab.example/api/v4", "private-token", TokenKindPrivate)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	largeBody := strings.Repeat("x", 12000)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET, got %s", r.Method)
			}
			return jsonResponse(http.StatusOK, `[{"id":6,"body":"`+largeBody+`"}]`), nil
		}),
	}

	notes, err := client.ListMergeRequestNotes(context.Background(), "1", "7")
	if err != nil {
		t.Fatalf("ListMergeRequestNotes returned error: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != 6 {
		t.Fatalf("expected one note with id 6, got %#v", notes)
	}
	if got := strconv.Itoa(len(notes[0].Body)); got != "12000" {
		t.Fatalf("expected body length 12000, got %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	resp := textResponse(status, body)
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
