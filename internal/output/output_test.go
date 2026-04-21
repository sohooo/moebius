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
	if !strings.Contains(body, "analysis partial: render warning skipped detailed diff") {
		t.Fatalf("expected render warning highlight in body, got %s", body)
	}
	if !strings.Contains(body, "[`Chart/argocd`](#chart-kube-bravo-argocd)") {
		t.Fatalf("expected render warning chart link in body, got %s", body)
	}
	if !strings.Contains(body, "Analysis is partial.") {
		t.Fatalf("expected partial analysis summary in body, got %s", body)
	}
	if !strings.Contains(body, "1 release(s) skipped due to render warnings.") {
		t.Fatalf("expected skipped release summary in body, got %s", body)
	}
	if !strings.Contains(body, "Render warnings:** 1 skipped release(s)") {
		t.Fatalf("expected render warning summary in body, got %s", body)
	}
	if !strings.Contains(body, "> [!important]") {
		t.Fatalf("expected important alert in body, got %s", body)
	}
	if !strings.Contains(body, "- Summary: render skipped · highest severity info · analysis partial") {
		t.Fatalf("expected chart summary bullet in body, got %s", body)
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
	if !strings.Contains(body, "[`Chart/hello-world`](#chart-kube-bravo-hello-world) | version upgrade: 10.3.0 → 12.0.2") {
		t.Fatalf("expected linked version-upgrade highlight in body, got %s", body)
	}
	if !strings.Contains(body, "version 10.3.0 → 12.0.2") {
		t.Fatalf("expected chart version change in body, got %s", body)
	}
	if !strings.Contains(body, "- Summary: version 10.3.0 → 12.0.2 · 2 resources affected · highest severity critical") {
		t.Fatalf("expected chart summary bullet with version change in body, got %s", body)
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

func TestRenderDescriptionBody_UsesMobiusHeadingsAndLinks(t *testing.T) {
	body, err := RenderDescriptionBodyWithOptions([]ClusterReport{sampleClusterReport()}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"}, NoteRenderOptions{
		Mode:   cli.CommentModeFull,
		Status: "changes detected",
	})
	if err != nil {
		t.Fatalf("RenderDescriptionBodyWithOptions returned error: %v", err)
	}
	if strings.Contains(body, `<a id=`) {
		t.Fatalf("description body must not contain custom anchor tags:\n%s", body)
	}
	if !strings.Contains(body, "[kube-bravo](#user-content-mobius-cluster-kube-bravo)") {
		t.Fatalf("expected mobius cluster navigation link:\n%s", body)
	}
	if !strings.Contains(body, "[`Deployment/hello-world`](#user-content-mobius-resource-kube-bravo-hello-world-demo-deployment-hello-world)") {
		t.Fatalf("expected mobius resource highlight link:\n%s", body)
	}
	if !strings.Contains(body, "## mobius cluster kube-bravo") {
		t.Fatalf("expected mobius cluster heading:\n%s", body)
	}
	if !strings.Contains(body, "### mobius chart kube-bravo hello-world") {
		t.Fatalf("expected mobius chart heading:\n%s", body)
	}
	if !strings.Contains(body, "#### mobius resource kube-bravo hello-world demo Deployment hello-world") {
		t.Fatalf("expected mobius resource heading:\n%s", body)
	}
	if strings.Contains(body, "#møbius") || strings.Contains(body, "## møbius") || strings.Contains(body, "### møbius") || strings.Contains(body, "#### møbius") {
		t.Fatalf("actionable links and heading targets must use ASCII mobius:\n%s", body)
	}
}

func TestRenderDescriptionBody_ResourceLinksIncludeChartAndNamespace(t *testing.T) {
	report := sampleClusterReport()
	second := report.Charts[0]
	second.Name = "other-chart"
	second.Namespace = "other"
	second.Resources = append([]ResourceReport(nil), second.Resources...)
	second.Resources[0].Namespace = "other"
	report.Charts = append(report.Charts, second)

	body, err := RenderDescriptionBodyWithOptions([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{}, NoteRenderOptions{
		Mode:   cli.CommentModeFull,
		Status: "changes detected",
	})
	if err != nil {
		t.Fatalf("RenderDescriptionBodyWithOptions returned error: %v", err)
	}
	firstAnchor := "#user-content-mobius-resource-kube-bravo-hello-world-demo-deployment-hello-world"
	secondAnchor := "#user-content-mobius-resource-kube-bravo-other-chart-other-deployment-hello-world"
	if !strings.Contains(body, firstAnchor) || !strings.Contains(body, secondAnchor) {
		t.Fatalf("expected resource anchors to include chart and namespace:\n%s", body)
	}
}

func TestSortReportsForComment_PrioritizesRemovedBeforeChangedBeforeAdded(t *testing.T) {
	reports := []ClusterReport{{
		Name: "kube-bravo",
		Charts: []ChartReport{{
			Name: "hello-world",
			Resources: []ResourceReport{
				{Kind: "ConfigMap", Name: "added", State: "added", Assessment: severity.Assessment{Level: severity.LevelMedium}, Validation: validate.Result{Status: validate.StatusValid}},
				{Kind: "ConfigMap", Name: "changed", State: "changed", Assessment: severity.Assessment{Level: severity.LevelMedium}, Validation: validate.Result{Status: validate.StatusValid}},
				{Kind: "ConfigMap", Name: "removed", State: "removed", Assessment: severity.Assessment{Level: severity.LevelMedium}, Validation: validate.Result{Status: validate.StatusValid}},
			},
		}},
	}}

	sortReportsForComment(reports)

	got := []string{
		reports[0].Charts[0].Resources[0].Name,
		reports[0].Charts[0].Resources[1].Name,
		reports[0].Charts[0].Resources[2].Name,
	}
	want := []string{"removed", "changed", "added"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected resource order: got %v want %v", got, want)
	}
}

func TestRenderCommentBody_IncludesPermissivePartialAnalysisWarning(t *testing.T) {
	report := ClusterReport{
		Name:    "kube-bravo",
		Added:   0,
		Removed: 0,
		Changed: 1,
		Charts: []ChartReport{
			{
				Name:      "hello-world",
				Namespace: "demo",
				Warnings: []string{
					`duplicate key "prometheus.io/scrape" accepted with last-wins behavior`,
					`duplicate key "prometheus.io/port" accepted with last-wins behavior`,
				},
				Resources: sampleClusterReport().Charts[0].Resources[:1],
			},
		},
	}

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "Analysis is partial.") {
		t.Fatalf("expected partial analysis summary in body, got %s", body)
	}
	if !strings.Contains(body, "duplicate YAML keys accepted with last-wins behavior: 2 override(s).") {
		t.Fatalf("expected last-wins summary in body, got %s", body)
	}
	if !strings.Contains(body, "- Summary: 1 resource affected · highest severity high · analysis partial") {
		t.Fatalf("expected chart summary bullet with partial analysis in body, got %s", body)
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
