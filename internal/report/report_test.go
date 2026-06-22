package report

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/helmrender"
	"github.com/sohooo/moebius/internal/output"
)

func TestCompareCluster_IncludesChartWithWarningsOnly(t *testing.T) {
	root := t.TempDir()
	baselineOutput := filepath.Join(root, "baseline")
	currentOutput := filepath.Join(root, "current")
	diffOutput := filepath.Join(root, "diff")
	currentChartDir := filepath.Join(currentOutput, "kube-bravo", "otel-stack")

	if err := os.MkdirAll(currentChartDir, 0o755); err != nil {
		t.Fatalf("mkdir current chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentChartDir, "namespace.txt"), []byte("monitoring\n"), 0o644); err != nil {
		t.Fatalf("write namespace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentChartDir, renderNoticeFilename), []byte("duplicate key kept last value\n"), 0o644); err != nil {
		t.Fatalf("write notices: %v", err)
	}

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, map[string]config.Release{}, map[string]config.Release{})
	if err != nil {
		t.Fatalf("compareCluster returned error: %v", err)
	}
	if len(report.Charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(report.Charts))
	}
	if report.Charts[0].Name != "otel-stack" {
		t.Fatalf("unexpected chart name %q", report.Charts[0].Name)
	}
	if len(report.Charts[0].Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(report.Charts[0].Warnings))
	}
}

func TestCompareCluster_IncludesChartWithMissingVersionWarningOnly(t *testing.T) {
	root := t.TempDir()
	baselineOutput := filepath.Join(root, "baseline")
	currentOutput := filepath.Join(root, "current")
	diffOutput := filepath.Join(root, "diff")
	currentChartDir := filepath.Join(currentOutput, "kube-bravo", "argocd")

	if err := os.MkdirAll(currentChartDir, 0o755); err != nil {
		t.Fatalf("mkdir current chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentChartDir, "namespace.txt"), []byte("argocd\n"), 0o644); err != nil {
		t.Fatalf("write namespace: %v", err)
	}
	warning := `cluster "kube-bravo" release "argocd" chart "oci://internal.oci.repo/helm-int/argo-cd" requested chart version "1.2.3" is unavailable: manifest unknown`
	if err := os.WriteFile(filepath.Join(currentChartDir, renderWarningFilename), []byte(warning+"\n"), 0o644); err != nil {
		t.Fatalf("write render warning: %v", err)
	}

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, map[string]config.Release{}, map[string]config.Release{})
	if err != nil {
		t.Fatalf("compareCluster returned error: %v", err)
	}
	if len(report.Charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(report.Charts))
	}
	if report.Charts[0].RenderWarning != warning {
		t.Fatalf("unexpected render warning %q", report.Charts[0].RenderWarning)
	}
}

func TestMissingVersionRenderWarning_FormatsIdentity(t *testing.T) {
	release := config.Release{
		Name:           "argocd",
		RepoURL:        "oci://internal.oci.repo/helm-int",
		Chart:          "argo-cd",
		TargetRevision: "1.2.3",
	}
	err := &helmrender.MissingVersionError{
		ChartRef:        "oci://internal.oci.repo/helm-int/argo-cd",
		RepoURL:         release.RepoURL,
		TargetRevision:  release.TargetRevision,
		UnderlyingError: errors.New("manifest unknown"),
	}

	got, ok := missingVersionRenderWarning("kube-bravo", release, release.ChartReference(), err)
	if !ok {
		t.Fatal("expected missing version warning")
	}
	for _, needle := range []string{
		`cluster "kube-bravo" release "argocd"`,
		`chart "oci://internal.oci.repo/helm-int/argo-cd"`,
		`requested chart version "1.2.3" is unavailable`,
		`repo "oci://internal.oci.repo/helm-int"`,
		`manifest unknown`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected warning to contain %q, got %q", needle, got)
		}
	}
}

func TestRenderClusterLoadsMultipleAppsFilesAndSharedOverrides(t *testing.T) {
	root := t.TempDir()
	layout := config.Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := config.ClusterDir(root, layout, "kube-bravo")
	outputRoot := filepath.Join(root, "out")
	cacheDir := filepath.Join(root, "cache")

	writeReportTestFile(t, filepath.Join(root, "charts", "hello-world", "Chart.yaml"), "apiVersion: v2\nname: hello-world\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts", "hello-world", "values.yaml"), "message: default\n")
	writeReportTestFile(t, filepath.Join(root, "charts", "hello-world", "templates", "configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  message: {{ .Values.message | quote }}
`)
	writeReportTestFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: prod-app
  namespace: prod
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), `- name: prod-app
  namespace: ignored
  project: default
  chart: charts/ignored
- name: dev-app
  namespace: dev
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(clusterDir, "overrides", "default", "dev-app.yaml"), "message: dev\n")

	err := renderCluster(root, layout, "kube-bravo", "current", outputRoot, helmrender.New(cacheDir), cli.RenderErrorModeFail, cli.DuplicateKeyModeError)
	if err != nil {
		t.Fatalf("renderCluster returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "kube-bravo", "prod-app", "rendered.yaml")); err != nil {
		t.Fatalf("expected prod-app rendered output: %v", err)
	}
	devRendered, err := os.ReadFile(filepath.Join(outputRoot, "kube-bravo", "dev-app", "rendered.yaml"))
	if err != nil {
		t.Fatalf("expected dev-app rendered output: %v", err)
	}
	if !strings.Contains(string(devRendered), `message: "dev"`) {
		t.Fatalf("expected dev-app override values, got:\n%s", string(devRendered))
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "kube-bravo", "ignored", "rendered.yaml")); err == nil {
		t.Fatalf("duplicate release from secondary apps file should not render")
	}
}

func TestRenderClusterWarnSkipReleaseSkipsRenderFailure(t *testing.T) {
	root := t.TempDir()
	layout := config.Default().Layout
	clusterDir := config.ClusterDir(root, layout, "kube-bravo")
	outputRoot := filepath.Join(root, "out", "current")
	cacheDir := filepath.Join(root, "cache")

	writeReportTestFile(t, filepath.Join(root, "charts", "broken", "Chart.yaml"), "apiVersion: v2\nname: broken\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts", "broken", "templates", "configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ required "name required" .Values.name }}
`)
	writeReportTestFile(t, filepath.Join(root, "charts", "ok", "Chart.yaml"), "apiVersion: v2\nname: ok\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts", "ok", "templates", "configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`)
	writeReportTestFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: broken
  namespace: demo
  project: default
  chart: charts/broken
- name: ok
  namespace: demo
  project: default
  chart: charts/ok
`)

	err := renderCluster(root, layout, "kube-bravo", "current", outputRoot, helmrender.New(cacheDir), cli.RenderErrorModeWarnSkipRelease, cli.DuplicateKeyModeError)
	if err != nil {
		t.Fatalf("renderCluster returned error: %v", err)
	}
	warningPath := filepath.Join(outputRoot, "kube-bravo", "broken", renderWarningFilename)
	warning, err := os.ReadFile(warningPath)
	if err != nil {
		t.Fatalf("expected skipped release warning: %v", err)
	}
	for _, needle := range []string{
		`cluster "kube-bravo" release "broken"`,
		`chart "charts/broken" failed to render current manifests`,
		`name required`,
	} {
		if !strings.Contains(string(warning), needle) {
			t.Fatalf("expected warning to contain %q, got:\n%s", needle, string(warning))
		}
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "kube-bravo", "ok", "rendered.yaml")); err != nil {
		t.Fatalf("expected ok release rendered output: %v", err)
	}
}

func TestBuildRendersMergeBaseCurrentAndSelectedClusters(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: app
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-baseline/apps-dev.yaml"), `- name: old
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/app.yaml"), "message: base\n")
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/app.yaml"), "message: current\n")
	if err := os.RemoveAll(filepath.Join(root, "clusters/kube-baseline")); err != nil {
		t.Fatalf("remove baseline-only cluster: %v", err)
	}
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-current/apps-dev.yaml"), `- name: new
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-current/overrides/default/new.yaml"), "message: added\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	outputDir := filepath.Join(root, "artifacts")
	reports, gotOutputDir, err := Build(cli.Options{
		BaseRef:       "main",
		AppsFiles:     []string{"apps.yaml", "apps-dev.yaml"},
		OutputDir:     outputDir,
		ContextLines:  1,
		Validate:      true,
		DiffMode:      cli.DiffModeSemantic,
		OutputFormat:  cli.OutputFormatMarkdown,
		CommentMode:   cli.CommentModeFull,
		PublishTarget: cli.PublishTargetDescription,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if gotOutputDir != outputDir {
		t.Fatalf("unexpected output dir %q want %q", gotOutputDir, outputDir)
	}
	if got, want := clusterNames(reports), "kube-baseline,kube-bravo,kube-current"; got != want {
		t.Fatalf("unexpected changed cluster reports %q want %q", got, want)
	}
	baseline := findReport(t, reports, "kube-baseline")
	if baseline.Removed == 0 {
		t.Fatalf("expected kube-baseline removed resources, got %#v", baseline)
	}
	bravo := findReport(t, reports, "kube-bravo")
	if bravo.Changed == 0 {
		t.Fatalf("expected kube-bravo changed resources, got %#v", bravo)
	}
	current := findReport(t, reports, "kube-current")
	if current.Added == 0 {
		t.Fatalf("expected kube-current added resources, got %#v", current)
	}
	for _, rel := range []string{
		"current/kube-bravo/app/rendered.yaml",
		"baseline/kube-bravo/app/rendered.yaml",
		"current/kube-current/new/rendered.yaml",
		artifactIndexFilename,
		artifactSummaryFilename,
	} {
		if _, err := os.Stat(filepath.Join(outputDir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}

	allReports, _, err := Build(cli.Options{
		BaseRef:      "main",
		AppsFiles:    []string{"apps.yaml", "apps-dev.yaml"},
		AllClusters:  true,
		OutputDir:    filepath.Join(root, "all-artifacts"),
		ContextLines: 1,
		Validate:     false,
	})
	if err != nil {
		t.Fatalf("Build all clusters returned error: %v", err)
	}
	if got, want := clusterNames(allReports), "kube-bravo,kube-current"; got != want {
		t.Fatalf("unexpected all-cluster reports %q want %q", got, want)
	}

	_, _, err = Build(cli.Options{
		BaseRef:   "main",
		Cluster:   "missing",
		OutputDir: filepath.Join(root, "missing-artifacts"),
	})
	if err == nil || !strings.Contains(err.Error(), `cluster "missing" does not exist`) {
		t.Fatalf("expected selected cluster error, got %v", err)
	}
}

func writeReportTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func writeTinyChart(t *testing.T, root string) {
	t.Helper()
	writeReportTestFile(t, filepath.Join(root, "charts/hello-world/Chart.yaml"), "apiVersion: v2\nname: hello-world\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts/hello-world/values.yaml"), "message: default\n")
	writeReportTestFile(t, filepath.Join(root, "charts/hello-world/templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  message: {{ .Values.message | quote }}
`)
}

func commitReportRepo(t *testing.T, repo *git.Repository, message string) plumbing.Hash {
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

func setReportRepoMain(t *testing.T, repo *git.Repository, hash plumbing.Hash) {
	t.Helper()
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), hash)); err != nil {
		t.Fatalf("set main ref: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/remotes/origin/main"), hash)); err != nil {
		t.Fatalf("set origin/main ref: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), plumbing.ReferenceName("refs/remotes/origin/main"))); err != nil {
		t.Fatalf("set origin/HEAD ref: %v", err)
	}
	_ = repo.CreateBranch(&gitconfig.Branch{Name: "main"})
}

func clusterNames(reports []output.ClusterReport) string {
	names := make([]string, 0, len(reports))
	for _, report := range reports {
		names = append(names, report.Name)
	}
	return strings.Join(names, ",")
}

func findReport(t *testing.T, reports []output.ClusterReport, name string) output.ClusterReport {
	t.Helper()
	for _, report := range reports {
		if report.Name == name {
			return report
		}
	}
	t.Fatalf("report %q not found in %#v", name, reports)
	return output.ClusterReport{}
}

func TestWriteArtifactIndex_IncludesErrorAndWarningArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := writeArtifactMessage(filepath.Join(root, "errors"), "current", "kube-bravo", "otel-stack", []string{"render failed"}); err != nil {
		t.Fatalf("write error artifact: %v", err)
	}
	if err := writeArtifactMessage(filepath.Join(root, "warnings"), "current", "kube-bravo", "otel-stack", []string{"duplicate key kept last value"}); err != nil {
		t.Fatalf("write warning artifact: %v", err)
	}

	reports := []output.ClusterReport{{
		Name:    "kube-bravo",
		Added:   1,
		Removed: 0,
		Changed: 2,
		Charts: []output.ChartReport{{
			Name:      "otel-stack",
			Namespace: "monitoring",
		}},
	}}

	if err := writeArtifactIndex(root, reports); err != nil {
		t.Fatalf("writeArtifactIndex returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, artifactIndexFilename))
	if err != nil {
		t.Fatalf("read artifact index: %v", err)
	}
	text := string(data)
	for _, needle := range []string{
		"# møbius Artifacts",
		"## Error Artifacts",
		"warnings/current--kube-bravo--otel-stack.txt",
		"errors/current--kube-bravo--otel-stack.txt",
		"`kube-bravo`: 1 chart(s), added 1, removed 0, changed 2",
	} {
		if strings.Contains(text, needle) {
			continue
		}
		t.Fatalf("expected artifact index to contain %q, got:\n%s", needle, text)
	}
}

func TestWriteArtifactSummary_IncludesCountsAndArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := writeArtifactMessage(filepath.Join(root, "errors"), "current", "kube-bravo", "otel-stack", []string{"render failed"}); err != nil {
		t.Fatalf("write error artifact: %v", err)
	}
	if err := writeArtifactMessage(filepath.Join(root, "warnings"), "baseline", "kube-bravo", "otel-stack", []string{"duplicate key kept last value"}); err != nil {
		t.Fatalf("write warning artifact: %v", err)
	}

	reports := []output.ClusterReport{{
		Name:    "kube-bravo",
		Added:   1,
		Removed: 2,
		Changed: 3,
		Charts: []output.ChartReport{
			{Name: "otel-stack"},
			{Name: "argocd"},
		},
	}}

	if err := writeArtifactSummary(root, reports); err != nil {
		t.Fatalf("writeArtifactSummary returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, artifactSummaryFilename))
	if err != nil {
		t.Fatalf("read artifact summary: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal artifact summary: %v", err)
	}

	if got["clusters"].(float64) != 1 {
		t.Fatalf("expected clusters=1, got %v", got["clusters"])
	}
	if got["charts"].(float64) != 2 {
		t.Fatalf("expected charts=2, got %v", got["charts"])
	}
	if got["added"].(float64) != 1 || got["removed"].(float64) != 2 || got["changed"].(float64) != 3 {
		t.Fatalf("unexpected change counts: %v", got)
	}
	errorArtifacts := got["error_artifacts"].([]interface{})
	if len(errorArtifacts) != 1 || errorArtifacts[0].(string) != "current--kube-bravo--otel-stack.txt" {
		t.Fatalf("unexpected error artifacts: %v", errorArtifacts)
	}
	warningArtifacts := got["warning_artifacts"].([]interface{})
	if len(warningArtifacts) != 1 || warningArtifacts[0].(string) != "baseline--kube-bravo--otel-stack.txt" {
		t.Fatalf("unexpected warning artifacts: %v", warningArtifacts)
	}
}
