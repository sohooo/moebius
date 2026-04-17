package comment

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/gitlab"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/severity"
)

func TestServicePost_CreatesNoteWhenMissing(t *testing.T) {
	client := &fakeNoteClient{}
	service := &Service{
		newClient: func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", Token: "token", TokenKind: gitlab.TokenKindPrivate}, nil
		},
	}

	result, err := service.Post(context.Background(), cli.Options{DiffMode: cli.DiffModeSemantic}, sampleReports())
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if result.Action != "created" {
		t.Fatalf("expected created action, got %q", result.Action)
	}
	if client.createdBody == "" {
		t.Fatalf("expected note creation")
	}
}

func TestServicePost_UpdatesExistingStickyNote(t *testing.T) {
	client := &fakeNoteClient{
		notes: []gitlab.Note{
			{ID: 99, Body: "# old\n\n" + output.StickyMarker},
		},
	}
	service := &Service{
		newClient: func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", Token: "token", TokenKind: gitlab.TokenKindPrivate}, nil
		},
	}

	result, err := service.Post(context.Background(), cli.Options{DiffMode: cli.DiffModeSemantic}, sampleReports())
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if result.Action != "updated" {
		t.Fatalf("expected updated action, got %q", result.Action)
	}
	if client.updatedID != 99 {
		t.Fatalf("expected note 99 to be updated, got %d", client.updatedID)
	}
}

func TestServicePost_SkipsUpdateWhenBodyMatches(t *testing.T) {
	t.Setenv("CI_PIPELINE_URL", "")
	t.Setenv("CI_JOB_URL", "")
	t.Setenv("CI_COMMIT_SHA", "")
	service := New()
	body, err := output.RenderCommentBodyWithOptions(sampleReports(), "semantic", output.NoteMetadata{
		DiffMode: "semantic",
	}, output.NoteRenderOptions{
		Mode:   cli.CommentModeFull,
		Status: "changes detected",
	})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	client := &fakeNoteClient{
		notes: []gitlab.Note{
			{ID: 41, Body: body},
		},
	}
	service.newClient = func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) { return client, nil }
	service.resolve = func(opts cli.Options) (gitlab.Target, error) {
		return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", Token: "token", TokenKind: gitlab.TokenKindPrivate}, nil
	}

	result, err := service.Post(context.Background(), cli.Options{DiffMode: cli.DiffModeSemantic}, sampleReports())
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if result.Action != "noop" {
		t.Fatalf("expected noop action, got %q", result.Action)
	}
	if client.updatedID != 0 || client.createdBody != "" {
		t.Fatalf("expected no create/update call")
	}
}

func TestServicePost_FallsBackToSummaryArtifactsWhenCommentTooLarge(t *testing.T) {
	client := &fakeNoteClient{}
	service := &Service{
		newClient: func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", Token: "token", TokenKind: gitlab.TokenKindPrivate}, nil
		},
	}

	result, err := service.Post(context.Background(), cli.Options{
		DiffMode:        cli.DiffModeSemantic,
		CommentMode:     cli.CommentModeFull,
		MaxCommentBytes: 400,
		BaseRef:         "master",
	}, sampleReports())
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if result.Action != "created" {
		t.Fatalf("expected created action, got %q", result.Action)
	}
	if !strings.Contains(client.createdBody, "Status:** report truncated") {
		t.Fatalf("expected truncated status in body:\n%s", client.createdBody)
	}
	if !strings.Contains(client.createdBody, "Compact summary mode") {
		t.Fatalf("expected compact summary footer in body:\n%s", client.createdBody)
	}
}

func TestServicePreflight_ReportsPermissionFailure(t *testing.T) {
	client := &fakeNoteClient{
		probeErr: &gitlab.APIError{Method: "POST", Path: "/projects/1/merge_requests/7/notes", Status: "403 Forbidden", StatusCode: 403, Body: "forbidden"},
	}
	service := &Service{
		newClient: func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{
				ProjectID:       "1",
				MergeRequestIID: "7",
				BaseURL:         "https://gitlab.example/api/v4",
				Token:           "token",
				TokenKind:       gitlab.TokenKindJob,
				TokenSource:     "CI_JOB_TOKEN",
			}, nil
		},
	}

	status, err := service.Preflight(context.Background(), cli.Options{})
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if status.Status != StatusError {
		t.Fatalf("expected error status, got %q", status.Status)
	}
	if len(status.Messages) == 0 || !strings.Contains(status.Messages[0], "CI_JOB_TOKEN is often read-only") {
		t.Fatalf("expected descriptive permission message, got %#v", status.Messages)
	}
}

func TestWriteStatusArtifact(t *testing.T) {
	dir := t.TempDir()
	status := StatusReport{
		Status:          StatusError,
		Stage:           "preflight",
		ProjectID:       "1",
		MergeRequestIID: "7",
		BaseURL:         "https://gitlab.example/api/v4",
		TokenKind:       gitlab.TokenKindPrivate,
		TokenSource:     "GITLAB_TOKEN",
		Messages:        []string{"missing permission"},
	}
	if err := WriteStatusArtifact(dir, status); err != nil {
		t.Fatalf("WriteStatusArtifact returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "comment-preflight.json"))
	if err != nil {
		t.Fatalf("read status artifact: %v", err)
	}
	if !strings.Contains(string(data), `"status": "error"`) || !strings.Contains(string(data), `"token_source": "GITLAB_TOKEN"`) {
		t.Fatalf("unexpected artifact contents: %s", string(data))
	}
}

type fakeNoteClient struct {
	notes       []gitlab.Note
	createdBody string
	updatedID   int
	updatedBody string
	listErr     error
	probeErr    error
}

func (f *fakeNoteClient) ListMergeRequestNotes(ctx context.Context, projectID, mrIID string) ([]gitlab.Note, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]gitlab.Note(nil), f.notes...), nil
}

func (f *fakeNoteClient) CreateMergeRequestNote(ctx context.Context, projectID, mrIID, body string) (gitlab.Note, error) {
	f.createdBody = body
	return gitlab.Note{ID: 1, Body: body}, nil
}

func (f *fakeNoteClient) UpdateMergeRequestNote(ctx context.Context, projectID, mrIID string, noteID int, body string) (gitlab.Note, error) {
	f.updatedID = noteID
	f.updatedBody = body
	return gitlab.Note{ID: noteID, Body: body}, nil
}

func (f *fakeNoteClient) ProbeCreateMergeRequestNoteAccess(ctx context.Context, projectID, mrIID string) error {
	return f.probeErr
}

func sampleReports() []output.ClusterReport {
	return []output.ClusterReport{
		{
			Name:    "kube-bravo",
			Changed: 1,
			Charts: []output.ChartReport{
				{
					Name:      "hello-world",
					Namespace: "demo",
					Resources: []output.ResourceReport{
						{
							State:      "changed",
							Kind:       "Deployment",
							Name:       "hello-world",
							Namespace:  "demo",
							Result:     outputSampleResult(),
							Assessment: severity.Assess(severity.Input{Kind: "Deployment", Name: "hello-world", Namespace: "demo", State: "changed", Changes: outputSampleResult().Changes}),
						},
					},
				},
			},
		},
	}
}

func outputSampleResult() diff.Result {
	return diff.Result{
		HasChanges: true,
		Changes: []diff.Change{{
			State: "changed",
			Path:  []diff.Segment{{Key: "spec"}, {Key: "replicas"}},
			Old:   2,
			New:   3,
		}},
	}
}
