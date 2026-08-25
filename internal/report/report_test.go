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
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/helmrender"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
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

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, diff.IgnoreOptions{}, map[string]config.Release{}, map[string]config.Release{})
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

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, diff.IgnoreOptions{}, map[string]config.Release{}, map[string]config.Release{})
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

func TestCompareCluster_SuppressesIgnoredOnlyMetadataChanges(t *testing.T) {
	root := t.TempDir()
	baselineOutput := filepath.Join(root, "baseline")
	currentOutput := filepath.Join(root, "current")
	diffOutput := filepath.Join(root, "diff")

	writeRenderedResource(t, filepath.Join(baselineOutput, "kube-bravo", "outline", "resources", "apps_v1_Deployment_demo_outline.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: outline
  namespace: demo
  labels:
    app.kubernetes.io/version: 1.8.0
    helm.sh/chart: outline-0.8.0
spec:
  template:
    metadata:
      annotations:
        checksum/config: old
`)
	writeRenderedResource(t, filepath.Join(currentOutput, "kube-bravo", "outline", "resources", "apps_v1_Deployment_demo_outline.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: outline
  namespace: demo
  labels:
    app.kubernetes.io/version: 1.8.1
    helm.sh/chart: outline-0.9.0
spec:
  template:
    metadata:
      annotations:
        checksum/config: new
`)

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, diff.IgnoreOptions{UseDefaults: true}, map[string]config.Release{}, map[string]config.Release{})
	if err != nil {
		t.Fatalf("compareCluster returned error: %v", err)
	}
	if len(report.Charts) != 0 {
		t.Fatalf("expected ignored-only chart to be omitted, got %#v", report.Charts)
	}
	if report.Changed != 0 {
		t.Fatalf("expected no changed resources, got %d", report.Changed)
	}
	if _, err := os.Stat(filepath.Join(diffOutput, "kube-bravo", "outline", "apps_v1_Deployment_demo_outline.yaml.diff")); err == nil {
		t.Fatalf("ignored-only raw diff artifact should not be written")
	}
}

func TestCompareCluster_KeepsMixedResourceAfterFilteringIgnoredChanges(t *testing.T) {
	root := t.TempDir()
	baselineOutput := filepath.Join(root, "baseline")
	currentOutput := filepath.Join(root, "current")
	diffOutput := filepath.Join(root, "diff")

	writeRenderedResource(t, filepath.Join(baselineOutput, "kube-bravo", "outline", "resources", "apps_v1_Deployment_demo_outline.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: outline
  namespace: demo
  labels:
    app.kubernetes.io/version: 1.8.0
spec:
  replicas: 2
`)
	writeRenderedResource(t, filepath.Join(currentOutput, "kube-bravo", "outline", "resources", "apps_v1_Deployment_demo_outline.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: outline
  namespace: demo
  labels:
    app.kubernetes.io/version: 1.8.1
spec:
  replicas: 3
`)

	report, err := compareCluster("kube-bravo", baselineOutput, currentOutput, diffOutput, 3, false, diff.IgnoreOptions{UseDefaults: true}, map[string]config.Release{}, map[string]config.Release{})
	if err != nil {
		t.Fatalf("compareCluster returned error: %v", err)
	}
	if report.Changed != 1 {
		t.Fatalf("expected one changed resource, got %d", report.Changed)
	}
	if len(report.Charts) != 1 || len(report.Charts[0].Resources) != 1 {
		t.Fatalf("expected one chart with one resource, got %#v", report.Charts)
	}
	resource := report.Charts[0].Resources[0]
	if got := changeSummaries(resource.Result.Changes); strings.Join(got, "\n") != "changed spec.replicas" {
		t.Fatalf("unexpected filtered changes: %v", got)
	}
	if strings.Contains(resource.Semantic, "app.kubernetes.io/version") {
		t.Fatalf("ignored version label leaked into semantic output:\n%s", resource.Semantic)
	}
	if resource.Assessment.Level != "high" {
		t.Fatalf("expected severity to be based on replica change, got %#v", resource.Assessment)
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
	writeReportTestFile(t, filepath.Join(clusterDir, "overrides", "common.yaml"), "message: common\n")
	writeReportTestFile(t, filepath.Join(clusterDir, "overrides", "default", "dev-app.yaml"), "message: dev\n")

	err := renderCluster(root, layout, "kube-bravo", "current", outputRoot, allReleasesSelection(), helmrender.New(cacheDir), cli.RenderErrorModeFail, cli.DuplicateKeyModeError)
	if err != nil {
		t.Fatalf("renderCluster returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "kube-bravo", "prod-app", "rendered.yaml")); err != nil {
		t.Fatalf("expected prod-app rendered output: %v", err)
	}
	prodRendered, err := os.ReadFile(filepath.Join(outputRoot, "kube-bravo", "prod-app", "rendered.yaml"))
	if err != nil {
		t.Fatalf("expected prod-app rendered output: %v", err)
	}
	if !strings.Contains(string(prodRendered), `message: "common"`) {
		t.Fatalf("expected prod-app common override values, got:\n%s", string(prodRendered))
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
	warning, err := os.ReadFile(filepath.Join(outputRoot, "kube-bravo", "prod-app", renderNoticeFilename))
	if err != nil {
		t.Fatalf("expected duplicate release warning: %v", err)
	}
	if !strings.Contains(string(warning), `release "prod-app" is defined in both apps.yaml and apps-dev.yaml`) {
		t.Fatalf("unexpected duplicate warning:\n%s", string(warning))
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

	err := renderCluster(root, layout, "kube-bravo", "current", outputRoot, allReleasesSelection(), helmrender.New(cacheDir), cli.RenderErrorModeWarnSkipRelease, cli.DuplicateKeyModeError)
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
		artifactHTMLFilename,
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

func TestBuildPrunesUnaffectedReleases(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: outline
  namespace: demo
  project: default
  chart: charts/hello-world
- name: keycloak
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/outline.yaml"), "message: outline-base\n")
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/keycloak.yaml"), "message: keycloak-base\n")
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/outline.yaml"), "message: outline-current\n")
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
	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    outputDir,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got, want := clusterNames(reports), "kube-bravo"; got != want {
		t.Fatalf("unexpected cluster reports %q want %q", got, want)
	}
	if len(reports[0].Charts) != 1 || reports[0].Charts[0].Name != "outline" {
		t.Fatalf("expected only outline chart in report, got %#v", reports[0].Charts)
	}
	for _, rel := range []string{
		"current/kube-bravo/outline/rendered.yaml",
		"baseline/kube-bravo/outline/rendered.yaml",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, rel)); err != nil {
			t.Fatalf("expected affected artifact %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"current/kube-bravo/keycloak",
		"baseline/kube-bravo/keycloak",
		"diff/kube-bravo/keycloak",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected unaffected artifact %s, err=%v", rel, err)
		}
	}
	summaryText := readReportTestFile(t, filepath.Join(outputDir, runSummaryMarkdownFilename))
	if !strings.Contains(summaryText, "| `keycloak` | `apps.yaml` | `skipped` | `not_affected` | `not_rendered` | `not_reported` |") {
		t.Fatalf("expected skipped keycloak in run summary, got:\n%s", summaryText)
	}
}

func TestBuildDefaultAppsDevOverrideChangeIsReported(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps-dev.yaml"), `- name: dev-app
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/dev-app.yaml"), "message: base\n")
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/dev-app.yaml"), "message: current\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	outputDir := filepath.Join(root, "artifacts")
	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    outputDir,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got, want := clusterNames(reports), "kube-bravo"; got != want {
		t.Fatalf("unexpected cluster reports %q want %q", got, want)
	}
	if len(reports[0].Charts) != 1 || reports[0].Charts[0].Name != "dev-app" {
		t.Fatalf("expected dev-app chart report, got %#v", reports[0].Charts)
	}
	if reports[0].Changed != 1 {
		t.Fatalf("expected one changed resource, got %#v", reports[0])
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current/kube-bravo/dev-app/rendered.yaml")); err != nil {
		t.Fatalf("expected current dev-app artifact: %v", err)
	}
	summaryText := readReportTestFile(t, filepath.Join(outputDir, runSummaryMarkdownFilename))
	for _, needle := range []string{
		"Apps files: `apps.yaml,apps-dev.yaml`",
		"| `dev-app` | `apps-dev.yaml` | `selected` | `override_changed` | `rendered` | `produced_changes` |",
	} {
		if !strings.Contains(summaryText, needle) {
			t.Fatalf("expected run summary to contain %q, got:\n%s", needle, summaryText)
		}
	}
	var runSummary runSummary
	if err := json.Unmarshal([]byte(readReportTestFile(t, filepath.Join(outputDir, runSummaryJSONFilename))), &runSummary); err != nil {
		t.Fatalf("unmarshal run summary: %v", err)
	}
	if len(runSummary.Layout.AppsFiles) < 2 || runSummary.Layout.AppsFiles[1] != "apps-dev.yaml" {
		t.Fatalf("expected apps-dev.yaml in run summary, got %#v", runSummary.Layout.AppsFiles)
	}
}

func TestBuildCommonOverrideChangeAffectsAllClusterReleases(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeReportTestFile(t, filepath.Join(root, "charts/cluster-aware/Chart.yaml"), "apiVersion: v2\nname: cluster-aware\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts/cluster-aware/templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  owner: {{ .Values.cluster.owner | quote }}
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: outline
  namespace: demo
  project: default
  chart: charts/cluster-aware
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps-dev.yaml"), `- name: keycloak
  namespace: demo
  project: default
  chart: charts/cluster-aware
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/common.yaml"), "cluster:\n  owner: platform\n")
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/common.yaml"), "cluster:\n  owner: product\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	outputDir := filepath.Join(root, "artifacts")
	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    outputDir,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got, want := clusterNames(reports), "kube-bravo"; got != want {
		t.Fatalf("unexpected cluster reports %q want %q", got, want)
	}
	if got, want := chartNames(reports[0].Charts), "keycloak,outline"; got != want {
		t.Fatalf("unexpected chart reports %q want %q", got, want)
	}
	summaryText := readReportTestFile(t, filepath.Join(outputDir, runSummaryMarkdownFilename))
	for _, needle := range []string{
		"Common override path: `overrides/common.yaml`",
		"| `outline` | `apps.yaml` | `selected` | `common_override_changed` | `rendered` | `produced_changes` |",
		"| `keycloak` | `apps-dev.yaml` | `selected` | `common_override_changed` | `rendered` | `produced_changes` |",
		"_No warnings or full-cluster fallbacks recorded._",
	} {
		if !strings.Contains(summaryText, needle) {
			t.Fatalf("expected run summary to contain %q, got:\n%s", needle, summaryText)
		}
	}
	var runSummary runSummary
	if err := json.Unmarshal([]byte(readReportTestFile(t, filepath.Join(outputDir, runSummaryJSONFilename))), &runSummary); err != nil {
		t.Fatalf("unmarshal run summary: %v", err)
	}
	if runSummary.Layout.CommonOverridePath != "overrides/common.yaml" {
		t.Fatalf("expected common override path in JSON summary, got %#v", runSummary.Layout)
	}
}

func TestBuildDoesNotRenderUnaffectedBrokenRelease(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeReportTestFile(t, filepath.Join(root, "charts/broken/Chart.yaml"), "apiVersion: v2\nname: broken\nversion: 0.1.0\n")
	writeReportTestFile(t, filepath.Join(root, "charts/broken/templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ required "name required" .Values.name }}
`)
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: broken
  namespace: demo
  project: default
  chart: charts/broken
- name: ok
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/ok.yaml"), "message: base\n")
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/ok.yaml"), "message: current\n")
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
	reports, _, err := Build(cli.Options{
		BaseRef:         "main",
		OutputDir:       outputDir,
		ContextLines:    1,
		RenderErrorMode: cli.RenderErrorModeFail,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(reports) != 1 || len(reports[0].Charts) != 1 || reports[0].Charts[0].Name != "ok" {
		t.Fatalf("expected only ok chart in report, got %#v", reports)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current/kube-bravo/broken")); !os.IsNotExist(err) {
		t.Fatalf("unexpected broken chart artifact, err=%v", err)
	}
}

func TestBuildLocalChartChangeAffectsReferencingReleases(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: outline
  namespace: demo
  project: default
  chart: charts/hello-world
- name: keycloak
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "charts/hello-world/templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  message: {{ .Values.message | quote }}
  chartChange: "true"
`)
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

	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    filepath.Join(root, "artifacts"),
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got, want := clusterNames(reports), "kube-bravo"; got != want {
		t.Fatalf("unexpected cluster reports %q want %q", got, want)
	}
	if got, want := chartNames(reports[0].Charts), "keycloak,outline"; got != want {
		t.Fatalf("unexpected chart reports %q want %q", got, want)
	}
}

func TestBuildChartModeReportsValuesChange(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeRootChart(t, root, "message: base\n", `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  message: {{ .Values.message | quote }}
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "values.yaml"), "message: current\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	outputDir := filepath.Join(root, "artifacts")
	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    outputDir,
		ContextLines: 1,
		Namespace:    "demo",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got, want := clusterNames(reports), "chart"; got != want {
		t.Fatalf("unexpected cluster reports %q want %q", got, want)
	}
	if len(reports[0].Charts) != 1 || reports[0].Charts[0].Name != "app" {
		t.Fatalf("expected app chart report, got %#v", reports[0].Charts)
	}
	if reports[0].Changed != 1 {
		t.Fatalf("expected one changed resource, got %#v", reports[0])
	}
	for _, rel := range []string{
		"current/chart/app/rendered.yaml",
		"baseline/chart/app/rendered.yaml",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, rel)); err != nil {
			t.Fatalf("expected chart artifact %s: %v", rel, err)
		}
	}
	summaryText := readReportTestFile(t, filepath.Join(outputDir, runSummaryMarkdownFilename))
	for _, needle := range []string{
		"Mode: `chart_repository`",
		"| `app` | `Chart.yaml` | `selected` | `local_chart_changed` | `rendered` | `produced_changes` |",
	} {
		if !strings.Contains(summaryText, needle) {
			t.Fatalf("expected chart run summary to contain %q, got:\n%s", needle, summaryText)
		}
	}
}

func TestBuildChartModeMissingDefaultValuesIsAllowed(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeRootChart(t, root, "", `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)

	writeReportTestFile(t, filepath.Join(root, "templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
data:
  changed: "true"
`)
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	reports, _, err := Build(cli.Options{
		BaseRef:      "main",
		OutputDir:    filepath.Join(root, "artifacts"),
		ContextLines: 1,
		Namespace:    "default",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(reports) != 1 || reports[0].Changed != 1 {
		t.Fatalf("expected chart report without values.yaml, got %#v", reports)
	}
}

func TestBuildChartModeExplicitMissingValuesErrors(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeRootChart(t, root, "", `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)
	writeReportTestFile(t, filepath.Join(root, "templates/configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
data:
  changed: "true"
`)
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	_, _, err = Build(cli.Options{
		BaseRef:     "main",
		OutputDir:   filepath.Join(root, "artifacts"),
		ChartPath:   ".",
		ValuesFiles: []string{"values-ci.yaml"},
		Namespace:   "default",
	})
	if err == nil || !strings.Contains(err.Error(), `values file "values-ci.yaml" does not exist`) {
		t.Fatalf("expected missing explicit values file error, got %v", err)
	}
}

func TestBuildChartModeUnchangedRenderProducesNoReport(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeRootChart(t, root, "", `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)
	writeReportTestFile(t, filepath.Join(root, "README.md"), "docs only\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	reports, _, err := Build(cli.Options{
		BaseRef:   "main",
		OutputDir: filepath.Join(root, "artifacts"),
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected no report for unchanged render, got %#v", reports)
	}
}

func TestBuildChartModeAmbiguousWithClusterLayout(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	writeRootChart(t, root, "", `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`)
	writeTinyChart(t, root)
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/apps.yaml"), `- name: app
  namespace: demo
  project: default
  chart: charts/hello-world
`)
	mainHash := commitReportRepo(t, repo, "main")
	setReportRepoMain(t, repo, mainHash)
	writeReportTestFile(t, filepath.Join(root, "README.md"), "docs only\n")
	_ = commitReportRepo(t, repo, "feature")

	oldWD := chdirReportTest(t, root)
	defer restoreReportTestWD(t, oldWD)

	_, _, err = Build(cli.Options{
		BaseRef:   "main",
		OutputDir: filepath.Join(root, "artifacts"),
		Namespace: "default",
	})
	if err == nil || !strings.Contains(err.Error(), "both a root Chart.yaml and discoverable clusters") {
		t.Fatalf("expected ambiguous chart/cluster error, got %v", err)
	}
}

func writeRenderedResource(t *testing.T, path, body string) {
	t.Helper()
	writeReportTestFile(t, path, body)
	chartDir := filepath.Dir(filepath.Dir(path))
	if err := os.WriteFile(filepath.Join(chartDir, "namespace.txt"), []byte("demo\n"), 0o644); err != nil {
		t.Fatalf("write namespace: %v", err)
	}
}

func changeSummaries(changes []diff.Change) []string {
	out := make([]string, 0, len(changes))
	for _, change := range changes {
		out = append(out, change.State+" "+diff.PathString(change.Path))
	}
	return out
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

func readReportTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
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

func writeRootChart(t *testing.T, root, values, template string) {
	t.Helper()
	writeReportTestFile(t, filepath.Join(root, "Chart.yaml"), "apiVersion: v2\nname: app\nversion: 0.1.0\n")
	if values != "" {
		writeReportTestFile(t, filepath.Join(root, "values.yaml"), values)
	}
	writeReportTestFile(t, filepath.Join(root, "templates/configmap.yaml"), template)
}

func chdirReportTest(t *testing.T, root string) string {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	return oldWD
}

func restoreReportTestWD(t *testing.T, oldWD string) {
	t.Helper()
	if err := os.Chdir(oldWD); err != nil {
		t.Fatalf("restore cwd: %v", err)
	}
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

func chartNames(charts []output.ChartReport) string {
	names := make([]string, 0, len(charts))
	for _, chart := range charts {
		names = append(names, chart.Name)
	}
	return strings.Join(names, ",")
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
		"[`report.html`](report.html)",
		"`report.html`: full offline HTML report",
		"## Error Artifacts",
		"`run-summary.md`: effective configuration and release selection decisions",
		"`run-summary.json`: machine-readable run summary",
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

func TestWriteArtifactHTML_EmbedsSearchableFullReportAndDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := writeArtifactMessage(filepath.Join(root, "errors"), "current", "kube-bravo", "otel-stack", []string{"render <failed>"}); err != nil {
		t.Fatalf("write error artifact: %v", err)
	}
	reports := []output.ClusterReport{{
		Name:    "kube-bravo",
		Changed: 1,
		Charts: []output.ChartReport{{
			Name:      "otel-<stack>",
			Namespace: "monitoring",
			Resources: []output.ResourceReport{{
				State:     "changed",
				Kind:      "Deployment",
				Name:      "collector",
				Namespace: "monitoring",
				Result: diff.Result{
					Changes: []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "spec"}, {Key: "replicas"}}, Old: 1, New: 2}},
					RawDiff: "--- baseline.yaml\n+++ current.yaml\n@@ -1 +1 @@\n-replicas: 1\n+replicas: 2\n",
				},
				Assessment: severity.Assessment{
					Level: severity.LevelHigh,
					Findings: []severity.Finding{{
						Level: severity.LevelHigh, Category: "capacity", Reason: "replicas changed 1 -> 2", Path: "spec.replicas",
					}},
				},
				Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated, SchemaSource: validate.SchemaSourceEmbedded},
			}},
		}},
	}}
	summary := &runSummary{Mode: "cluster_repository", BaseRef: "main", HeadSHA: "head", MergeBaseSHA: "base", ChangedPathsCount: 2}

	if err := writeArtifactHTML(root, reports, summary); err != nil {
		t.Fatalf("writeArtifactHTML returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, artifactHTMLFilename))
	if err != nil {
		t.Fatalf("read HTML report: %v", err)
	}
	text := string(data)
	for _, needle := range []string{
		"<!doctype html>",
		"local artifact report",
		"Search cluster, chart, resource, path, or finding",
		"<summary><span class=\"resource-id\">",
		"data-severity=\"high\"",
		"replicas changed 1 -&gt; 2",
		"spec.replicas",
		"Semantic diff",
		"Raw unified diff",
		"replicas: 1",
		"errors/current--kube-bravo--otel-stack.txt",
		"render &lt;failed&gt;",
		"otel-&lt;stack&gt;",
		"Self-contained report generated by møbius",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected HTML report to contain %q, got:\n%s", needle, text)
		}
	}
	if strings.Contains(text, "otel-<stack>") || strings.Contains(text, "render <failed>") {
		t.Fatalf("expected report data to be HTML escaped:\n%s", text)
	}
	if strings.Contains(text, "https://") || strings.Contains(text, "http://") {
		t.Fatalf("expected a self-contained report without external assets:\n%s", text)
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
	if got["html_report"].(string) != artifactHTMLFilename {
		t.Fatalf("unexpected HTML report entry: %v", got["html_report"])
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
