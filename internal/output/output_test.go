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
	if strings.Contains(body, "| Severity | Cluster | Resource | Finding |") {
		t.Fatalf("expected no global highlights table in body, got %s", body)
	}
	if strings.Contains(body, "Highlights by cluster") || strings.Contains(body, "| Severity | Resource | Finding |") {
		t.Fatalf("expected no highlights section in body, got %s", body)
	}
	if !strings.Contains(body, "Analysis is partial.") {
		t.Fatalf("expected partial analysis summary in body, got %s", body)
	}
	if strings.Index(body, "Analysis is partial.") > strings.Index(body, "**Navigation**") {
		t.Fatalf("expected partial analysis warning above navigation, got %s", body)
	}
	if !strings.Contains(body, "1 release(s) skipped due to other render warnings.") {
		t.Fatalf("expected generic skipped release summary in body, got %s", body)
	}
	if !strings.Contains(body, "Other render warnings:** 1 skipped release(s)") {
		t.Fatalf("expected generic render warning summary in body, got %s", body)
	}
	if !strings.Contains(body, "> [!important]") {
		t.Fatalf("expected important alert in body, got %s", body)
	}
	if !strings.Contains(body, "| **Summary** | render skipped · highest severity 🔵 info · analysis partial |") {
		t.Fatalf("expected chart summary table in body, got %s", body)
	}
	if !strings.Contains(body, "_Report compares merge-base and current MR state | validation: clean | commit: `deadbeef`._") {
		t.Fatalf("expected clean validation metadata in footer, got %s", body)
	}
}

func TestRenderCommentBody_DistinguishesMissingChartVersions(t *testing.T) {
	report := ClusterReport{
		Name: "kube-bravo",
		Charts: []ChartReport{
			{
				Name:          "argocd",
				Namespace:     "argocd",
				RenderWarning: `cluster "kube-bravo" release "argocd" chart "oci://internal.oci.repo/helm-int/argo-cd" requested chart version "1.2.3" is unavailable: manifest unknown`,
			},
			{
				Name:          "broken",
				Namespace:     "default",
				RenderWarning: `cluster "kube-bravo" release "broken" chart "oci://internal.oci.repo/helm-int/broken" produced invalid current rendered YAML`,
			},
		},
	}

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	for _, needle := range []string{
		"1 release(s) skipped because the requested chart version is unavailable.",
		"**Missing chart versions:** 1 skipped release(s)",
		"1 release(s) skipped due to other render warnings.",
		"**Other render warnings:** 1 skipped release(s)",
		"> Render warning: requested version 1.2.3 unavailable",
		"| **Summary** | requested version 1.2.3 unavailable · highest severity 🔵 info · analysis partial |",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected body to contain %q, got %s", needle, body)
		}
	}
}

func TestRenderCommentBody_ShowsVersionChanges(t *testing.T) {
	report := sampleClusterReport()
	report.Charts[0].HasRemoteSource = true
	report.Charts[0].BaselineTargetRevision = "10.3.0"
	report.Charts[0].CurrentTargetRevision = "12.0.2"

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "[`ClusterRole/hello-world`](#resource-kube-bravo-clusterrole-hello-world)") {
		t.Fatalf("expected linked chart change resource in body, got %s", body)
	}
	if !strings.Contains(body, "version 10.3.0 → 12.0.2") {
		t.Fatalf("expected chart version change in body, got %s", body)
	}
	if !strings.Contains(body, "| **Summary** | version 10.3.0 → 12.0.2 · 2 resources affected · highest severity 🔴 critical |") {
		t.Fatalf("expected chart summary table with version change in body, got %s", body)
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

func TestRenderDescriptionBody_UsesEmojiHeadingsAndStableLinks(t *testing.T) {
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
	if !strings.Contains(body, "[kube-bravo](#user-content-computer-kube-bravo)") {
		t.Fatalf("expected emoji cluster navigation link:\n%s", body)
	}
	if !strings.Contains(body, "[`Deployment/hello-world`](#user-content-kube-bravo-hello-world-demodeployment-hello-world)") {
		t.Fatalf("expected resource highlight link:\n%s", body)
	}
	if !strings.Contains(body, "## :computer: kube-bravo") {
		t.Fatalf("expected emoji cluster heading:\n%s", body)
	}
	if !strings.Contains(body, "### :package: kube-bravo hello-world") {
		t.Fatalf("expected emoji chart heading:\n%s", body)
	}
	if !strings.Contains(body, "#### `kube-bravo` · hello-world · demo/Deployment hello-world") {
		t.Fatalf("expected resource heading without mobius prefix:\n%s", body)
	}
	if strings.Contains(body, "mobius cluster") || strings.Contains(body, "mobius chart") || strings.Contains(body, "mobius resource") {
		t.Fatalf("description body must not contain old mobius heading prefixes:\n%s", body)
	}
	if strings.Contains(body, "Charts with changes:") {
		t.Fatalf("description body must not contain redundant chart count line:\n%s", body)
	}
	if strings.Contains(body, "**Resource:**") {
		t.Fatalf("description body must not contain redundant resource line:\n%s", body)
	}
	if !strings.Contains(body, "- changed · severity 🟠 high") {
		t.Fatalf("expected resource metadata bullet with severity badge:\n%s", body)
	}
	if !strings.Contains(body, "- changed · severity 🟠 high · validation: validated via embedded · [up](#user-content-package-kube-bravo-hello-world)") {
		t.Fatalf("expected resource metadata bullet with chart backlink:\n%s", body)
	}
	if strings.Contains(body, "#møbius") || strings.Contains(body, "## møbius") || strings.Contains(body, "### møbius") || strings.Contains(body, "#### møbius") {
		t.Fatalf("actionable links and heading targets must use ASCII mobius:\n%s", body)
	}
	if !strings.Contains(body, "| **Summary** | 2 resources affected · highest severity 🔴 critical |") {
		t.Fatalf("expected bold summary label with severity badge:\n%s", body)
	}
	if strings.Contains(body, "**Severity:**") || strings.Contains(body, "**Change fingerprint:**") || strings.Contains(body, "**Validation:**") || strings.Contains(body, "> [!caution]") {
		t.Fatalf("expected compact top-level summary without duplicate severity/fingerprint/validation/caution text:\n%s", body)
	}
	if !strings.Contains(body, "| **Change mix** | +0 · -0 · ~2 |") {
		t.Fatalf("expected chart change mix:\n%s", body)
	}
	if !strings.Contains(body, "| **Surface** | security · workload |") {
		t.Fatalf("expected chart surface summary:\n%s", body)
	}
	if !strings.Contains(body, "| **Severity** | 🔴 critical 1 · 🟠 high 1 |") {
		t.Fatalf("expected severity summary badges:\n%s", body)
	}
	if !strings.Contains(body, "[`ClusterRole/hello-world`](#user-content-kube-bravo-hello-world-clusterrole-hello-world)") {
		t.Fatalf("expected empty namespace resource link to preserve GitLab heading slug:\n%s", body)
	}
	if strings.Contains(body, "#user-content-kube-bravo-hello-world-none-clusterrole-hello-world") {
		t.Fatalf("empty namespace resource links must not inject none:\n%s", body)
	}
	if !strings.Contains(body, "**Changes**") {
		t.Fatalf("expected chart changes section:\n%s", body)
	}
	if !strings.Contains(body, "- 🔴 [`ClusterRole/hello-world`](#user-content-kube-bravo-hello-world-clusterrole-hello-world) · RBAC rules changed at `rules`") {
		t.Fatalf("expected linked chart changes with severity icon only:\n%s", body)
	}
	if strings.Contains(body, "**critical**") || strings.Contains(body, "**high**") {
		t.Fatalf("chart changes must not duplicate severity words:\n%s", body)
	}
}

func TestRenderCommentBody_ClassifiesPlatformSurfaces(t *testing.T) {
	report := ClusterReport{
		Name:    "kube-bravo",
		Changed: 4,
		Charts: []ChartReport{{
			Name:      "platform",
			Namespace: "platform",
			Resources: []ResourceReport{
				{
					State: "changed",
					Kind:  "Database",
					Name:  "app",
					Assessment: severity.Assessment{
						Level: severity.LevelMedium,
						Findings: []severity.Finding{{
							Level:    severity.LevelMedium,
							Category: "platform",
							Reason:   "CloudNativePG Database changed",
						}},
					},
					Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated},
				},
				{
					State:      "changed",
					Kind:       "ApplicationSet",
					Name:       "apps",
					Assessment: severity.Assessment{Level: severity.LevelInfo},
					Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated},
				},
				{
					State:      "changed",
					Kind:       "VaultAuth",
					Name:       "auth",
					Assessment: severity.Assessment{Level: severity.LevelCritical},
					Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated},
				},
				{
					State:      "changed",
					Kind:       "Widget",
					Name:       "custom",
					Assessment: severity.Assessment{Level: severity.LevelLow},
					Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated},
				},
			},
		}},
	}

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "| **Surface** | security · database · ci/cd · custom |") {
		t.Fatalf("expected ordered platform surface taxonomy:\n%s", body)
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
	firstAnchor := "#user-content-kube-bravo-hello-world-demodeployment-hello-world"
	secondAnchor := "#user-content-kube-bravo-other-chart-otherdeployment-hello-world"
	if !strings.Contains(body, firstAnchor) || !strings.Contains(body, secondAnchor) {
		t.Fatalf("expected resource anchors to include chart and namespace:\n%s", body)
	}
}

func TestRenderDescriptionBody_IgnoresDotsInGitLabResourceAnchors(t *testing.T) {
	report := ClusterReport{
		Name:    "kube-bravo",
		Changed: 1,
		Charts: []ChartReport{{
			Name:      "spawn",
			Namespace: "default",
			Resources: []ResourceReport{{
				State: "changed",
				Kind:  "CustomResourceDefinition",
				Name:  "clustermetadata.core.example.com",
				Assessment: severity.Assessment{
					Level: severity.LevelCritical,
					Findings: []severity.Finding{{
						Level:  severity.LevelCritical,
						Reason: "CustomResourceDefinition changed",
					}},
				},
				Validation: validate.Result{Status: validate.StatusValid, Coverage: validate.CoverageValidated},
			}},
		}},
	}

	body, err := RenderDescriptionBodyWithOptions([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{}, NoteRenderOptions{
		Mode:   cli.CommentModeFull,
		Status: "changes detected",
	})
	if err != nil {
		t.Fatalf("RenderDescriptionBodyWithOptions returned error: %v", err)
	}
	want := "#user-content-kube-bravo-spawn-customresourcedefinition-clustermetadatacoreexamplecom"
	if !strings.Contains(body, want) {
		t.Fatalf("expected dotted CRD anchor to ignore dots:\n%s", body)
	}
	if strings.Contains(body, "clustermetadata-core-example-com") {
		t.Fatalf("dotted CRD anchor must not convert dots to dashes:\n%s", body)
	}
	if !strings.Contains(body, "#### `kube-bravo` · spawn · /CustomResourceDefinition clustermetadata.core.example.com") {
		t.Fatalf("expected readable CRD resource heading:\n%s", body)
	}
}

func TestRenderCommentBody_ChartChangesListsAllResources(t *testing.T) {
	report := ClusterReport{
		Name:    "kube-bravo",
		Changed: 6,
		Charts: []ChartReport{{
			Name:      "many",
			Namespace: "demo",
			Resources: []ResourceReport{
				{State: "changed", Kind: "ConfigMap", Name: "one", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
				{State: "changed", Kind: "ConfigMap", Name: "two", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
				{State: "changed", Kind: "ConfigMap", Name: "three", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
				{State: "changed", Kind: "ConfigMap", Name: "four", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
				{State: "changed", Kind: "ConfigMap", Name: "five", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
				{State: "changed", Kind: "ConfigMap", Name: "six", Assessment: severity.Assessment{Level: severity.LevelLow}, Validation: validate.Result{Status: validate.StatusValid}},
			},
		}},
	}

	body, err := RenderCommentBody([]ClusterReport{report}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if !strings.Contains(body, "**Changes**") {
		t.Fatalf("expected chart changes section:\n%s", body)
	}
	if got := strings.Count(body, "🟢 [`ConfigMap/"); got != 6 {
		t.Fatalf("expected all 6 resources in chart changes list, got %d:\n%s", got, body)
	}
	if !strings.Contains(body, "- changed · severity 🟢 low · [up](#chart-kube-bravo-many)") {
		t.Fatalf("expected note-mode chart backlink:\n%s", body)
	}
}

func TestRenderCommentBody_RendersClusterDetailsAfterNavigationWithoutOuterFold(t *testing.T) {
	body, err := RenderCommentBody([]ClusterReport{sampleClusterReport()}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	navigation := strings.Index(body, "**Navigation**")
	clusterHeading := strings.Index(body, "## Cluster `kube-bravo`")
	chartFold := strings.Index(body, "<summary>Chart `hello-world`")
	if navigation == -1 || clusterHeading == -1 || chartFold == -1 {
		t.Fatalf("expected navigation, cluster heading, and chart fold:\n%s", body)
	}
	if strings.Contains(body, "Cluster Details ·") {
		t.Fatalf("expected no outer cluster details fold:\n%s", body)
	}
	if strings.Contains(body, "**Highlights by cluster**") {
		t.Fatalf("expected no highlights section:\n%s", body)
	}
	if !(navigation < clusterHeading && clusterHeading < chartFold) {
		t.Fatalf("expected navigation before cluster body and chart fold:\n%s", body)
	}
}

func TestRenderCommentBody_DoesNotRenderHighlights(t *testing.T) {
	first := sampleClusterReport()
	second := sampleClusterReport()
	second.Name = "kube-charlie"
	third := sampleClusterReport()
	third.Name = "kube-delta"

	body, err := RenderCommentBody([]ClusterReport{first, second, third}, diff.ModeSemantic, NoteMetadata{CommitSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}

	if strings.Contains(body, "**Highlights**\n\n| Severity | Cluster | Resource | Finding |") {
		t.Fatalf("expected no global highlights table:\n%s", body)
	}
	if strings.Contains(body, "_Additional highlights are grouped by cluster below._") {
		t.Fatalf("expected no grouped highlights note:\n%s", body)
	}
	if strings.Contains(body, "Highlights by cluster") || strings.Contains(body, "highlights</summary>") {
		t.Fatalf("expected no grouped highlights:\n%s", body)
	}
}

func TestRenderCommentBody_FooterUsesCompactMetadataLinks(t *testing.T) {
	body, err := RenderCommentBody([]ClusterReport{sampleClusterReport()}, diff.ModeSemantic, NoteMetadata{
		PipelineURL: "https://gitlab.example/pipelines/123",
		JobURL:      "https://gitlab.example/jobs/456",
		CommitSHA:   "deadbeef",
		BaseRef:     "master",
		DiffMode:    "semantic",
		GeneratedAt: "2026-04-05T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("RenderCommentBody returned error: %v", err)
	}
	if strings.Contains(body, "Pipeline: https://") || strings.Contains(body, "Job: https://") {
		t.Fatalf("expected no raw top metadata URLs:\n%s", body)
	}
	for _, needle := range []string{
		"[pipeline](https://gitlab.example/pipelines/123)",
		"[job](https://gitlab.example/jobs/456)",
		"commit: `deadbeef`",
		"base ref: `master`",
		"diff mode: `semantic`",
		"generated: `2026-04-05T12:00:00Z`",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected compact footer metadata %q:\n%s", needle, body)
		}
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
	if !strings.Contains(body, "> [!warning]\n> duplicate key \"prometheus.io/scrape\" accepted with last-wins behavior") {
		t.Fatalf("expected duplicate-key warning above table, got %s", body)
	}
	if !strings.Contains(body, "| **Summary** | 1 resource affected · highest severity 🟠 high · analysis partial |") {
		t.Fatalf("expected chart summary table with partial analysis in body, got %s", body)
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
