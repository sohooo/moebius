package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientMergeRequestNotesLifecycle(t *testing.T) {
	var seenAuth string
	client, err := New("https://gitlab.example/api/v4", "job-token")
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

func TestClientReturnsHelpfulAPIError(t *testing.T) {
	client, err := New("https://gitlab.example/api/v4", "job-token")
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
