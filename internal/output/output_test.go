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
