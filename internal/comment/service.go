// Package comment posts sticky møbius reports back to GitLab merge requests.
package comment

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/gitlab"
	"github.com/sohooo/moebius/internal/output"
)

type NoteClient interface {
	ListMergeRequestNotes(ctx context.Context, projectID, mrIID string) ([]gitlab.Note, error)
	CreateMergeRequestNote(ctx context.Context, projectID, mrIID, body string) (gitlab.Note, error)
	UpdateMergeRequestNote(ctx context.Context, projectID, mrIID string, noteID int, body string) (gitlab.Note, error)
}

type Service struct {
	newClient func(baseURL, jobToken string) (NoteClient, error)
	resolve   func(opts cli.Options) (gitlab.Target, error)
}

type Result struct {
	Action  string
	Message string
}

func New() *Service {
	return &Service{
		newClient: func(baseURL, jobToken string) (NoteClient, error) {
			return gitlab.New(baseURL, jobToken)
		},
		resolve: gitlab.ResolveTarget,
	}
}

func (s *Service) Post(ctx context.Context, opts cli.Options, reports []output.ClusterReport) (Result, error) {
	target, err := s.resolve(opts)
	if err != nil {
		return Result{}, err
	}
	client, err := s.newClient(target.BaseURL, target.JobToken)
	if err != nil {
		return Result{}, err
	}

	meta := output.NoteMetadata{
		PipelineURL: os.Getenv("CI_PIPELINE_URL"),
		JobURL:      os.Getenv("CI_JOB_URL"),
		CommitSHA:   os.Getenv("CI_COMMIT_SHA"),
		BaseRef:     opts.BaseRef,
		DiffMode:    string(opts.DiffMode),
	}
	renderOpts := output.NoteRenderOptions{
		Mode:   opts.CommentMode,
		Status: commentStatus(reports),
	}
	body, err := output.RenderCommentBodyWithOptions(reports, diff.Mode(opts.DiffMode), meta, renderOpts)
	if err != nil {
		return Result{}, err
	}
	maxCommentBytes := opts.MaxCommentBytes
	if maxCommentBytes <= 0 {
		maxCommentBytes = 50000
	}
	if len(body) > maxCommentBytes {
		renderOpts.Mode = cli.CommentModeSummaryArtifacts
		renderOpts.IncludeArtifactsHint = true
		renderOpts.Status = "report truncated"
		body, err = output.RenderCommentBodyWithOptions(reports, diff.Mode(opts.DiffMode), meta, renderOpts)
		if err != nil {
			return Result{}, err
		}
	}

	notes, err := client.ListMergeRequestNotes(ctx, target.ProjectID, target.MergeRequestIID)
	if err != nil {
		return Result{}, err
	}
	for _, note := range notes {
		if !strings.Contains(note.Body, output.StickyMarker) {
			continue
		}
		if normalizeNoteBody(note.Body) == normalizeNoteBody(body) {
			return Result{
				Action:  "noop",
				Message: fmt.Sprintf("møbius MR note on !%s is already up to date", target.MergeRequestIID),
			}, nil
		}
		if _, err := client.UpdateMergeRequestNote(ctx, target.ProjectID, target.MergeRequestIID, note.ID, body); err != nil {
			return Result{}, err
		}
		return Result{
			Action:  "updated",
			Message: fmt.Sprintf("Updated møbius MR note on !%s", target.MergeRequestIID),
		}, nil
	}

	if _, err := client.CreateMergeRequestNote(ctx, target.ProjectID, target.MergeRequestIID, body); err != nil {
		return Result{}, err
	}
	return Result{
		Action:  "created",
		Message: fmt.Sprintf("Created møbius MR note on !%s", target.MergeRequestIID),
	}, nil
}

func normalizeNoteBody(body string) string {
	return strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
}

func commentStatus(reports []output.ClusterReport) string {
	for _, report := range reports {
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" || len(chart.Warnings) > 0 {
				return "warnings detected"
			}
		}
	}
	if len(reports) == 0 {
		return "no effective changes"
	}
	return "changes detected"
}
