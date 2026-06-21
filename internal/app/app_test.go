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
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/buildinfo"
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

func TestRunVersionPrintsBuildInfo(t *testing.T) {
	origParse := parseOptions
	defer func() { parseOptions = origParse }()

	parseOptions = func(args []string, stdout io.Writer) (cli.Options, error) {
		return cli.Options{Command: cli.CommandVersion}, nil
	}

	var stdout bytes.Buffer
	if err := run([]string{"version"}, &stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if got, want := stdout.String(), buildinfo.String(); got != want {
		t.Fatalf("unexpected version output %q want %q", got, want)
	}
}

func TestRunCommentReportsPostFailureAndWritesStatus(t *testing.T) {
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
		return cli.Options{Command: cli.CommandComment, DiffMode: cli.DiffModeSemantic, OutputDir: outputDir}, nil
	}
	buildReports = func(opts cli.Options) ([]output.ClusterReport, string, error) {
		return []output.ClusterReport{{Name: "kube-bravo"}}, outputDir, nil
	}
	newCommentService = func() commentService {
		return fakeCommentService{
			preflightStatus: comment.StatusReport{Status: comment.StatusOK, Stage: "preflight"},
			postErr:         errors.New("post failed"),
		}
	}
	printReports = func(w io.Writer, reports []output.ClusterReport, mode diff.Mode, format cli.OutputFormat) error {
		_, err := io.WriteString(w, "fallback diff output\n")
		return err
	}
	inspectCurrentRepo = func(opts cli.Options) (repoContext, error) {
		return repoContext{}, errors.New("not inspected")
	}

	var stdout bytes.Buffer
	err := run([]string{"comment"}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "post failed") {
		t.Fatalf("expected post failure, got %v", err)
	}
	if !strings.Contains(stdout.String(), "møbius comment failed.") || !strings.Contains(stdout.String(), "fallback diff output") {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	data, readErr := os.ReadFile(filepath.Join(outputDir, "comment-preflight.json"))
	if readErr != nil {
		t.Fatalf("expected status artifact: %v", readErr)
	}
	if !strings.Contains(string(data), `"stage": "post"`) || !strings.Contains(string(data), "post failed") {
		t.Fatalf("unexpected status artifact: %s", string(data))
	}
}

func TestRunClustersShowsCurrentBaselineAndChangedFlags(t *testing.T) {
	clearGitLabEnv(t)
	root, cleanup := tempMobiusRepo(t)
	defer cleanup()

	var stdout bytes.Buffer
	err := run([]string{"clusters", "--base-ref", "main", "--apps-files", "apps.yaml,apps-dev.yaml"}, &stdout)
	if err != nil {
		t.Fatalf("run clusters returned error: %v", err)
	}
	text := stdout.String()
	for _, needle := range []string{
		"Clusters under clusters (apps files: apps.yaml,apps-dev.yaml, base ref: main)",
		"- kube-baseline: current=no baseline=yes changed=yes",
		"- kube-bravo: current=yes baseline=yes changed=yes",
		"- kube-current: current=yes baseline=no changed=yes",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %q in clusters output from %s:\n%s", needle, root, text)
		}
	}
}

func TestRunDoctorSuccessSkipsGitLabChecksWithoutContext(t *testing.T) {
	clearGitLabEnv(t)
	_, cleanup := tempMobiusRepo(t)
	defer cleanup()

	var stdout bytes.Buffer
	err := run([]string{"doctor", "--base-ref", "main", "--apps-files", "apps.yaml,apps-dev.yaml"}, &stdout)
	if err != nil {
		t.Fatalf("run doctor returned error: %v\n%s", err, stdout.String())
	}
	for _, needle := range []string{
		"[ok] git repository found:",
		"[ok] effective layout: clusters_dir=clusters apps_files=apps.yaml,apps-dev.yaml",
		"[warn] GitLab comment checks skipped",
	} {
		if !strings.Contains(stdout.String(), needle) {
			t.Fatalf("expected %q in doctor output:\n%s", needle, stdout.String())
		}
	}
}

func TestRunDoctorReportsMissingSelectedCluster(t *testing.T) {
	clearGitLabEnv(t)
	_, cleanup := tempMobiusRepo(t)
	defer cleanup()

	var stdout bytes.Buffer
	err := run([]string{"doctor", "--base-ref", "main", "--cluster", "missing"}, &stdout)
	if err == nil {
		t.Fatal("expected doctor error for missing cluster")
	}
	if !strings.Contains(stdout.String(), `[error] cluster "missing" does not exist`) {
		t.Fatalf("unexpected doctor output:\n%s", stdout.String())
	}
}

func TestRunDoctorReportsGitLabPreflightFailure(t *testing.T) {
	origChecker := newCommentChecker
	defer func() { newCommentChecker = origChecker }()
	t.Setenv("GITLAB_TOKEN", "token")
	_, cleanup := tempMobiusRepo(t)
	defer cleanup()

	newCommentChecker = func() commentService {
		return fakeCommentService{
			preflightStatus: comment.StatusReport{Messages: []string{"cannot update merge request description"}},
			preflightErr:    errors.New("preflight failed"),
		}
	}

	var stdout bytes.Buffer
	err := run([]string{"doctor", "--base-ref", "main"}, &stdout)
	if err == nil {
		t.Fatal("expected doctor error")
	}
	if !strings.Contains(stdout.String(), "[error] cannot update merge request description") {
		t.Fatalf("unexpected doctor output:\n%s", stdout.String())
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

func tempMobiusRepo(t *testing.T) (string, func()) {
	t.Helper()
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeAppFile(t, root, "clusters/kube-bravo/apps.yaml", "- name: app\n  namespace: demo\n  chart: charts/app\n")
	writeAppFile(t, root, "clusters/kube-baseline/apps-dev.yaml", "- name: old\n  namespace: demo\n  chart: charts/app\n")
	writeAppFile(t, root, "charts/app/Chart.yaml", "apiVersion: v2\nname: app\nversion: 0.1.0\n")
	mainHash := commitAll(t, repo, "main")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash)); err != nil {
		t.Fatalf("set main ref: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), plumbing.ReferenceName("refs/remotes/origin/main"))); err != nil {
		t.Fatalf("set origin/HEAD: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/remotes/origin/main"), mainHash)); err != nil {
		t.Fatalf("set origin/main: %v", err)
	}
	_ = repo.CreateBranch(&gitconfig.Branch{Name: "main"})

	writeAppFile(t, root, "clusters/kube-bravo/apps.yaml", "- name: app\n  namespace: demo\n  chart: charts/app\n  values: changed\n")
	if err := os.RemoveAll(filepath.Join(root, "clusters/kube-baseline")); err != nil {
		t.Fatalf("remove baseline cluster: %v", err)
	}
	writeAppFile(t, root, "clusters/kube-current/apps-dev.yaml", "- name: new\n  namespace: demo\n  chart: charts/app\n")
	_ = commitAll(t, repo, "feature")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	return root, func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}
}

func writeAppFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func commitAll(t *testing.T, repo *git.Repository, message string) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatalf("AddGlob: %v", err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		All:    true,
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit %q: %v", message, err)
	}
	return hash
}

func clearGitLabEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CI_PROJECT_ID",
		"CI_MERGE_REQUEST_IID",
		"CI_API_V4_URL",
		"CI_SERVER_URL",
		"GITLAB_TOKEN",
		"GITLAB_PRIVATE_TOKEN",
		"GITLAB_API_TOKEN",
		"CI_JOB_TOKEN",
	} {
		t.Setenv(key, "")
	}
}
