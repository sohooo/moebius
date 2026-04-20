package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

func TestRenderReports_Markdown(t *testing.T) {
	report := sampleClusterReport()

	got, err := RenderReports([]ClusterReport{report}, diff.ModeSemantic, cli.OutputFormatMarkdown)
	if err != nil {
		t.Fatalf("RenderReports returned error: %v", err)
	}

	want := readGolden(t, "markdown_report.golden")
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("unexpected markdown output:\n%s", got)
	}
}

func TestRenderCommentBody_NoChanges(t *testing.T) {
	body, err := RenderCommentBody(nil, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	want := readGolden(t, "comment_no_changes.golden")
	if strings.TrimSpace(body) != strings.TrimSpace(want) {
		t.Fatalf("unexpected comment body:\n%s", body)
	}
}

func TestRenderCommentBody_UsesCollapsibleChartSections(t *testing.T) {
	body, err := RenderCommentBody([]ClusterReport{sampleClusterReport()}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef", BaseRef: "master", DiffMode: "semantic", GeneratedAt: "2026-04-05T12:00:00Z"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	want := readGolden(t, "comment_report.golden")
	if strings.TrimSpace(body) != strings.TrimSpace(want) {
		t.Fatalf("unexpected comment body:\n%s", body)
	}
}

func TestRenderCommentBody_SummaryMode(t *testing.T) {
	body, err := RenderCommentBodyWithOptions([]ClusterReport{sampleClusterReport()}, diff.ModeSemantic, NoteMetadata{
		CommitSHA:   "deadbeef",
		BaseRef:     "master",
		DiffMode:    "semantic",
		GeneratedAt: "2026-04-05T12:00:00Z",
	}, NoteRenderOptions{Mode: cli.CommentModeSummary, Status: "changes detected"})
	if err != nil {
		t.Fatalf("RenderCommentBodyWithOptions returned error: %v", err)
	}

	want := readGolden(t, "comment_summary_report.golden")
	if strings.TrimSpace(body) != strings.TrimSpace(want) {
		t.Fatalf("unexpected summary comment body:\n%s", body)
	}
}

func TestRenderCommentBody_IncludesRenderWarnings(t *testing.T) {
	report := ClusterReport{
		Name:    "kube-bravo",
		Added:   0,
		Removed: 0,
		Changed: 0,
		Charts: []ChartReport{
			{
				Name:          "argocd",
				Namespace:     "argocd",
				RenderWarning: `cluster "kube-bravo" release "argocd" chart "oci://internal.oci.repo/helm-int/argo-cd" produced invalid current rendered YAML`,
			},
		},
	}

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "warnings detected") {
		t.Fatalf("expected warnings status in body, got %s", body)
	}
	if !strings.Contains(body, "| Severity | Cluster | Resource | Finding |") {
		t.Fatalf("expected highlights table in body, got %s", body)
	}
	if !strings.Contains(body, "render warning: cluster \"kube-bravo\" release \"argocd\"") {
		t.Fatalf("expected render warning highlight in body, got %s", body)
	}
	if !strings.Contains(body, "[`Chart/argocd`](#chart-kube-bravo-argocd)") {
		t.Fatalf("expected render warning chart link in body, got %s", body)
	}
	if !strings.Contains(body, "Render warnings:** 1 skipped release(s)") {
		t.Fatalf("expected render warning summary in body, got %s", body)
	}
	if !strings.Contains(body, "> [!important]") {
		t.Fatalf("expected important alert in body, got %s", body)
	}
}

func TestRenderCommentBody_LinksHighlightsAndShowsVersionChanges(t *testing.T) {
	report := sampleClusterReport()
	report.Charts[0].HasRemoteSource = true
	report.Charts[0].BaselineTargetRevision = "10.3.0"
	report.Charts[0].CurrentTargetRevision = "12.0.2"

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "[`ClusterRole/hello-world`](#resource-kube-bravo-clusterrole-hello-world)") {
		t.Fatalf("expected linked highlight resource in body, got %s", body)
	}
	if !strings.Contains(body, "version 10.3.0 → 12.0.2") {
		t.Fatalf("expected chart version change in body, got %s", body)
	}
}

func TestRenderCommentBody_UsesUniqueResourceAnchorsAcrossClusters(t *testing.T) {
	first := sampleClusterReport()
	second := sampleClusterReport()
	second.Name = "kube-charlie"

	body, err := RenderCommentBody([]ClusterReport{first, second}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, `id="resource-kube-bravo-deployment-hello-world"`) {
		t.Fatalf("expected kube-bravo resource anchor in body, got %s", body)
	}
	if !strings.Contains(body, `id="resource-kube-charlie-deployment-hello-world"`) {
		t.Fatalf("expected kube-charlie resource anchor in body, got %s", body)
	}
}

func sampleClusterReport() ClusterReport {
	change := diff.Change{
		State: "changed",
		Path: []diff.Segment{
			{Key: "spec"},
			{Key: "replicas"},
		},
		Old: 2,
		New: 3,
	}
	result := diff.Result{
		HasChanges: true,
		Changes:    []diff.Change{change},
		RawDiff: `--- old
+++ new
@@ -1,3 +1,3 @@
 spec:
-  replicas: 2
+  replicas: 3
`,
	}

	return ClusterReport{
		Name:    "kube-bravo",
		Added:   0,
		Removed: 0,
		Changed: 2,
		Charts: []ChartReport{
			{
				Name:      "hello-world",
				Namespace: "demo",
				Resources: []ResourceReport{
					{
						State:      "changed",
						Kind:       "Deployment",
						Name:       "hello-world",
						Namespace:  "demo",
						Result:     result,
						Assessment: severity.Assess(severity.Input{Kind: "Deployment", Name: "hello-world", Namespace: "demo", State: "changed", Changes: result.Changes}),
						Validation: validate.Result{
							Status:       validate.StatusValid,
							Coverage:     validate.CoverageValidated,
							SchemaSource: validate.SchemaSourceEmbedded,
						},
					},
					{
						State:     "changed",
						Kind:      "ClusterRole",
						Name:      "hello-world",
						Namespace: "",
						Result: diff.Result{
							HasChanges: true,
							Changes: []diff.Change{{
								State: "changed",
								Path:  []diff.Segment{{Key: "rules"}},
								Old:   []interface{}{"get"},
								New:   []interface{}{"get", "list"},
							}},
						},
						Assessment: severity.Assess(severity.Input{
							Kind:    "ClusterRole",
							Name:    "hello-world",
							State:   "changed",
							Changes: []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "rules"}}, Old: []interface{}{"get"}, New: []interface{}{"get", "list"}}},
						}),
						Validation: validate.Result{
							Status:       validate.StatusValid,
							Coverage:     validate.CoverageUnvalidated,
							SchemaSource: validate.SchemaSourceNone,
						},
					},
				},
			},
		},
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}
