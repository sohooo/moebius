package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohooo/moebius/internal/config"
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
