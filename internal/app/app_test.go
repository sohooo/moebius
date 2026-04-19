package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/comment"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/gitlab"
	"github.com/sohooo/moebius/internal/output"
)

func TestRunCommentFallsBackToDiffOnPreflightFailure(t *testing.T) {
	origParse := parseOptions
	origBuild := buildReports
	origNewService := newCommentService
	origPrint := printReports
	origInspect := inspectCurrentRepo
	defer func() {
		parseOptions = origParse
		buildReports = origBuild
		newCommentService = origNewService
		printReports = origPrint
		inspectCurrentRepo = origInspect
	}()

	outputDir := t.TempDir()
	parseOptions = func(args []string, stdout io.Writer) (cli.Options, error) {
		return cli.Options{
			Command:      cli.CommandComment,
			DiffMode:     cli.DiffModeSemantic,
			OutputFormat: cli.OutputFormatMarkdown,
			OutputDir:    outputDir,
		}, nil
	}
	buildReports = func(opts cli.Options) ([]output.ClusterReport, string, error) {
		return []output.ClusterReport{{
			Name:    "kube-bravo",
			Changed: 1,
			Charts: []output.ChartReport{{
				Name:      "hello-world",
				Namespace: "demo",
			}},
		}}, outputDir, nil
	}
	newCommentService = func() commentService {
		return fakeCommentService{
			preflightStatus: comment.StatusReport{
				Status:          comment.StatusError,
				Stage:           "preflight",
				ProjectID:       "1",
				MergeRequestIID: "7",
				BaseURL:         "https://gitlab.example/api/v4",
				TokenKind:       gitlab.TokenKindJob,
				TokenSource:     "CI_JOB_TOKEN",
				Messages:        []string{"resolved token can read the merge request but cannot create MR notes; CI_JOB_TOKEN is often read-only for notes, use GITLAB_TOKEN or --gitlab-token with API scope"},
			},
			preflightErr: errors.New("preflight failed"),
		}
	}
	printReports = func(w io.Writer, reports []output.ClusterReport, mode diff.Mode, format cli.OutputFormat) error {
		_, err := io.WriteString(w, "fallback diff output\n")
		return err
	}

	var stdout bytes.Buffer
	err := run([]string{"comment"}, &stdout)
	if err == nil {
		t.Fatal("expected run error")
	}
	if !strings.Contains(stdout.String(), "møbius comment failed.") {
		t.Fatalf("expected failure header, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Artifacts written to") {
		t.Fatalf("expected artifact path in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "fallback diff output") {
		t.Fatalf("expected fallback diff output, got %s", stdout.String())
	}
	data, readErr := os.ReadFile(filepath.Join(outputDir, "comment-preflight.json"))
	if readErr != nil {
		t.Fatalf("expected comment-preflight.json: %v", readErr)
	}
	if !strings.Contains(string(data), `"status": "error"`) {
		t.Fatalf("unexpected status artifact: %s", string(data))
	}
}

func TestRunDiffNoChangesPrintsResolvedContext(t *testing.T) {
	origParse := parseOptions
	origBuild := buildReports
	origInspect := inspectCurrentRepo
	defer func() {
		parseOptions = origParse
		buildReports = origBuild
		inspectCurrentRepo = origInspect
	}()

	parseOptions = func(args []string, stdout io.Writer) (cli.Options, error) {
		return cli.Options{
			Command: cli.CommandDiff,
		}, nil
	}
	buildReports = func(opts cli.Options) ([]output.ClusterReport, string, error) {
		return nil, "", nil
	}
	inspectCurrentRepo = func(opts cli.Options) (repoContext, error) {
		return repoContext{
			BaseRefName: "main",
			EffectiveLayout: config.LayoutConfig{
				ClustersDir: "clusters",
			},
		}, nil
	}

	var stdout bytes.Buffer
	if err := run([]string{"diff"}, &stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	text := stdout.String()
	for _, needle := range []string{
		"No affected clusters.",
		"Effective clusters dir: clusters",
		"Base ref: main",
		"mobius clusters",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %q in output, got %s", needle, text)
		}
	}
}

type fakeCommentService struct {
	preflightStatus comment.StatusReport
	preflightErr    error
	postResult      comment.Result
	postErr         error
}

func (f fakeCommentService) Preflight(ctx context.Context, opts cli.Options) (comment.StatusReport, error) {
	return f.preflightStatus, f.preflightErr
}

func (f fakeCommentService) Post(ctx context.Context, opts cli.Options, reports []output.ClusterReport) (comment.Result, error) {
	return f.postResult, f.postErr
}
