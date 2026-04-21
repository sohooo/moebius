// Package comment posts sticky møbius reports back to GitLab merge requests.
package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	ProbeCreateMergeRequestNoteAccess(ctx context.Context, projectID, mrIID string) error
	GetMergeRequest(ctx context.Context, projectID, mrIID string) (gitlab.MergeRequest, error)
	UpdateMergeRequestDescription(ctx context.Context, projectID, mrIID, description string) (gitlab.MergeRequest, error)
	ProbeUpdateMergeRequestDescriptionAccess(ctx context.Context, projectID, mrIID string) error
}

const (
	descriptionStartMarker = "<!-- mobius:mr-diff:start -->"
	descriptionEndMarker   = "<!-- mobius:mr-diff:end -->"
)

type Service struct {
	newClient func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error)
	resolve   func(opts cli.Options) (gitlab.Target, error)
}

type Result struct {
	Action  string
	Message string
}

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusError   Status = "error"
)

type StatusReport struct {
	Status          Status           `json:"status"`
	Stage           string           `json:"stage"`
	ProjectID       string           `json:"project_id,omitempty"`
	MergeRequestIID string           `json:"merge_request_iid,omitempty"`
	BaseURL         string           `json:"gitlab_base_url,omitempty"`
	TokenKind       gitlab.TokenKind `json:"token_kind,omitempty"`
	TokenSource     string           `json:"token_source,omitempty"`
	Action          string           `json:"action,omitempty"`
	Messages        []string         `json:"messages,omitempty"`
}

func New() *Service {
	return &Service{
		newClient: func(baseURL, token string, tokenKind gitlab.TokenKind) (NoteClient, error) {
			return gitlab.New(baseURL, token, tokenKind)
		},
		resolve: gitlab.ResolveTarget,
	}
}

func (s *Service) Preflight(ctx context.Context, opts cli.Options) (StatusReport, error) {
	status := StatusReport{Status: StatusError, Stage: "preflight"}
	target, err := s.resolve(opts)
	if err != nil {
		status.Messages = []string{err.Error()}
		return status, err
	}
	status.ProjectID = target.ProjectID
	status.MergeRequestIID = target.MergeRequestIID
	status.BaseURL = target.BaseURL
	status.TokenKind = target.TokenKind
	status.TokenSource = target.TokenSource

	client, err := s.newClient(target.BaseURL, target.Token, target.TokenKind)
	if err != nil {
		status.Messages = []string{err.Error()}
		return status, err
	}
	if opts.PublishTarget == cli.PublishTargetNote {
		if _, err := client.ListMergeRequestNotes(ctx, target.ProjectID, target.MergeRequestIID); err != nil {
			err = describeReadFailure(err)
			status.Messages = []string{err.Error()}
			return status, err
		}
		if err := client.ProbeCreateMergeRequestNoteAccess(ctx, target.ProjectID, target.MergeRequestIID); err != nil {
			err = describeCreateFailure(err, target)
			status.Messages = []string{err.Error()}
			return status, err
		}
	} else {
		if _, err := client.GetMergeRequest(ctx, target.ProjectID, target.MergeRequestIID); err != nil {
			err = describeMergeRequestReadFailure(err)
			status.Messages = []string{err.Error()}
			return status, err
		}
		if err := client.ProbeUpdateMergeRequestDescriptionAccess(ctx, target.ProjectID, target.MergeRequestIID); err != nil {
			err = describeDescriptionUpdateFailure(err, target)
			status.Messages = []string{err.Error()}
			return status, err
		}
	}

	status.Status = StatusOK
	status.Messages = []string{"GitLab comment preflight passed."}
	return status, nil
}

func (s *Service) Post(ctx context.Context, opts cli.Options, reports []output.ClusterReport) (Result, error) {
	target, err := s.resolve(opts)
	if err != nil {
		return Result{}, err
	}
	client, err := s.newClient(target.BaseURL, target.Token, target.TokenKind)
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
	body, err := renderBody(reports, diff.Mode(opts.DiffMode), meta, renderOpts, opts.PublishTarget)
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
		body, err = renderBody(reports, diff.Mode(opts.DiffMode), meta, renderOpts, opts.PublishTarget)
		if err != nil {
			return Result{}, err
		}
	}

	if opts.PublishTarget != cli.PublishTargetNote {
		return s.postDescription(ctx, client, target, body)
	}
	return s.postNote(ctx, client, target, body)
}

func (s *Service) postNote(ctx context.Context, client NoteClient, target gitlab.Target, body string) (Result, error) {
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

func (s *Service) postDescription(ctx context.Context, client NoteClient, target gitlab.Target, body string) (Result, error) {
	mr, err := client.GetMergeRequest(ctx, target.ProjectID, target.MergeRequestIID)
	if err != nil {
		return Result{}, err
	}
	next, action := upsertDescriptionBlock(mr.Description, body)
	if normalizeNoteBody(next) == normalizeNoteBody(mr.Description) {
		return Result{
			Action:  "noop",
			Message: fmt.Sprintf("møbius MR description report on !%s is already up to date", target.MergeRequestIID),
		}, nil
	}
	if _, err := client.UpdateMergeRequestDescription(ctx, target.ProjectID, target.MergeRequestIID, next); err != nil {
		return Result{}, err
	}
	return Result{
		Action:  action,
		Message: fmt.Sprintf("Updated møbius MR description report on !%s", target.MergeRequestIID),
	}, nil
}

func renderBody(reports []output.ClusterReport, mode diff.Mode, meta output.NoteMetadata, opts output.NoteRenderOptions, target cli.PublishTarget) (string, error) {
	if target == cli.PublishTargetNote {
		return output.RenderCommentBodyWithOptions(reports, mode, meta, opts)
	}
	return output.RenderDescriptionBodyWithOptions(reports, mode, meta, opts)
}

func upsertDescriptionBlock(description, body string) (string, string) {
	block := descriptionStartMarker + "\n" + strings.TrimSpace(body) + "\n" + descriptionEndMarker
	start := strings.Index(description, descriptionStartMarker)
	end := strings.Index(description, descriptionEndMarker)
	if start >= 0 && end >= start {
		end += len(descriptionEndMarker)
		next := description[:start] + block + description[end:]
		return strings.TrimRight(next, "\n") + "\n", "updated"
	}
	if strings.TrimSpace(description) == "" {
		return block + "\n", "created"
	}
	return strings.TrimRight(description, "\n") + "\n\n" + block + "\n", "created"
}

func WriteStatusArtifact(outputDir string, status StatusReport) error {
	if outputDir == "" {
		return nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outputDir, "comment-preflight.json"), data, 0o644)
}

func describeMergeRequestReadFailure(err error) error {
	var apiErr *gitlab.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("could not reach the GitLab merge request API; check CI_API_V4_URL/CI_SERVER_URL, network access, and TLS settings: %w", err)
	}
	switch apiErr.StatusCode {
	case 401:
		return fmt.Errorf("GitLab rejected the resolved token while reading the merge request; check GITLAB_TOKEN/--gitlab-token and ensure it is valid for this GitLab instance")
	case 403:
		return fmt.Errorf("resolved GitLab token can reach the API but cannot read the merge request; use a token with API scope and project access")
	case 404:
		return fmt.Errorf("GitLab merge request API returned 404 while resolving the description target; check CI_PROJECT_ID, CI_MERGE_REQUEST_IID, and token visibility for the target project")
	default:
		return err
	}
}

func describeDescriptionUpdateFailure(err error, target gitlab.Target) error {
	var apiErr *gitlab.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("could not probe GitLab merge request description update; check network access and GitLab API availability: %w", err)
	}
	switch apiErr.StatusCode {
	case 401:
		return fmt.Errorf("resolved GitLab token from %s was rejected while testing merge request description updates; use GITLAB_TOKEN or --gitlab-token with a valid API token", target.TokenSource)
	case 403:
		if target.TokenKind == gitlab.TokenKindJob {
			return fmt.Errorf("resolved token from %s can read the merge request but cannot update its description; CI_JOB_TOKEN is often read-only, use GITLAB_TOKEN or --gitlab-token with API scope", target.TokenSource)
		}
		return fmt.Errorf("resolved GitLab token from %s can reach the merge request but lacks permission to update its description; use a project, group, or bot token with API scope", target.TokenSource)
	case 404:
		return fmt.Errorf("GitLab returned 404 while probing merge request description updates; check CI_PROJECT_ID, CI_MERGE_REQUEST_IID, GitLab visibility, and whether the token can see the target MR")
	default:
		return err
	}
}

func describeReadFailure(err error) error {
	var apiErr *gitlab.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("could not reach the GitLab merge request notes API; check CI_API_V4_URL/CI_SERVER_URL, network access, and TLS settings: %w", err)
	}
	switch apiErr.StatusCode {
	case 401:
		return fmt.Errorf("GitLab rejected the resolved token while reading merge request notes; check GITLAB_TOKEN/--gitlab-token and ensure it is valid for this GitLab instance")
	case 403:
		return fmt.Errorf("resolved GitLab token can reach the API but cannot read merge request notes; use a token with API scope and project access")
	case 404:
		return fmt.Errorf("GitLab merge request notes API returned 404 while resolving the comment target; check CI_PROJECT_ID, CI_MERGE_REQUEST_IID, and token visibility for the target project")
	default:
		return err
	}
}

func describeCreateFailure(err error, target gitlab.Target) error {
	var apiErr *gitlab.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("could not probe GitLab comment creation; check network access and GitLab API availability: %w", err)
	}
	switch apiErr.StatusCode {
	case 401:
		return fmt.Errorf("resolved GitLab token from %s was rejected while testing merge request note creation; use GITLAB_TOKEN or --gitlab-token with a valid API token", target.TokenSource)
	case 403:
		if target.TokenKind == gitlab.TokenKindJob {
			return fmt.Errorf("resolved token from %s can read the merge request but cannot create MR notes; CI_JOB_TOKEN is often read-only for notes, use GITLAB_TOKEN or --gitlab-token with API scope", target.TokenSource)
		}
		return fmt.Errorf("resolved GitLab token from %s can reach the merge request but lacks permission to create MR notes; use a project, group, or bot token with API scope", target.TokenSource)
	case 404:
		return fmt.Errorf("GitLab returned 404 while probing merge request note creation; check CI_PROJECT_ID, CI_MERGE_REQUEST_IID, GitLab visibility, and whether the token can see the target MR")
	default:
		return err
	}
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
