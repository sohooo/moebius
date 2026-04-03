package comment

import (
	"context"
	"testing"

	"mobius/internal/cli"
	"mobius/internal/gitlab"
	"mobius/internal/output"
)

func TestServicePost_CreatesNoteWhenMissing(t *testing.T) {
	client := &fakeNoteClient{}
	service := &Service{
		newClient: func(baseURL, jobToken string) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", JobToken: "token"}, nil
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
		newClient: func(baseURL, jobToken string) (NoteClient, error) { return client, nil },
		resolve: func(opts cli.Options) (gitlab.Target, error) {
			return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", JobToken: "token"}, nil
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
	service := New()
	body, err := output.RenderCommentBody(sampleReports(), "semantic", output.NoteMetadata{})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	client := &fakeNoteClient{
		notes: []gitlab.Note{
			{ID: 41, Body: body},
		},
	}
	service.newClient = func(baseURL, jobToken string) (NoteClient, error) { return client, nil }
	service.resolve = func(opts cli.Options) (gitlab.Target, error) {
		return gitlab.Target{ProjectID: "1", MergeRequestIID: "7", BaseURL: "https://gitlab.example/api/v4", JobToken: "token"}, nil
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

type fakeNoteClient struct {
	notes       []gitlab.Note
	createdBody string
	updatedID   int
	updatedBody string
}

func (f *fakeNoteClient) ListMergeRequestNotes(ctx context.Context, projectID, mrIID string) ([]gitlab.Note, error) {
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

func sampleReports() []output.ClusterReport {
	return []output.ClusterReport{
		{
			Name:    "kube-bravo",
			Changed: 1,
			Charts: []output.ChartReport{
				{
					Name:      "hello-world",
					Namespace: "demo",
				},
			},
		},
	}
}
