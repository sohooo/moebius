package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/output"
)

const artifactIndexFilename = "index.md"
const artifactSummaryFilename = "summary.json"

func writeArtifactMessage(dir, state, cluster, release string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s--%s--%s.txt", state, cluster, release)
	return os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func writeArtifactIndex(outputDir string, reports []output.ClusterReport) error {
	if outputDir == "" {
		return nil
	}
	var b strings.Builder
	fmt.Fprintln(&b, "# møbius Artifacts")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Artifact layout:")
	fmt.Fprintln(&b, "- `current/`: current rendered manifests and split resources")
	fmt.Fprintln(&b, "- `baseline/`: merge-base rendered manifests and split resources")
	fmt.Fprintln(&b, "- `diff/`: raw and semantic per-resource diffs")
	fmt.Fprintln(&b, "- `errors/`: render failures persisted even in hard-fail mode")
	fmt.Fprintln(&b, "- `warnings/`: non-fatal render warnings such as permissive duplicate-key parsing")
	fmt.Fprintln(&b, "- `run-summary.md`: effective configuration and release selection decisions")
	fmt.Fprintln(&b, "- `run-summary.json`: machine-readable run summary")
	fmt.Fprintln(&b)

	errors := listArtifactFiles(filepath.Join(outputDir, "errors"))
	warnings := listArtifactFiles(filepath.Join(outputDir, "warnings"))
	fmt.Fprintf(&b, "## Summary\n\n- Clusters with report output: %d\n- Error artifacts: %d\n- Warning artifacts: %d\n\n", len(reports), len(errors), len(warnings))
	if len(reports) > 0 {
		fmt.Fprintln(&b, "## Cluster Reports")
		fmt.Fprintln(&b)
		for _, report := range reports {
			fmt.Fprintf(&b, "- `%s`: %d chart(s), added %d, removed %d, changed %d\n", report.Name, len(report.Charts), report.Added, report.Removed, report.Changed)
		}
		fmt.Fprintln(&b)
	}
	if len(errors) > 0 {
		fmt.Fprintln(&b, "## Error Artifacts")
		fmt.Fprintln(&b)
		for _, name := range errors {
			fmt.Fprintf(&b, "- `errors/%s`\n", name)
		}
		fmt.Fprintln(&b)
	}
	if len(warnings) > 0 {
		fmt.Fprintln(&b, "## Warning Artifacts")
		fmt.Fprintln(&b)
		for _, name := range warnings {
			fmt.Fprintf(&b, "- `warnings/%s`\n", name)
		}
		fmt.Fprintln(&b)
	}
	return os.WriteFile(filepath.Join(outputDir, artifactIndexFilename), []byte(b.String()), 0o644)
}

func listArtifactFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

type artifactSummary struct {
	Clusters       int                    `json:"clusters"`
	Charts         int                    `json:"charts"`
	Added          int                    `json:"added"`
	Removed        int                    `json:"removed"`
	Changed        int                    `json:"changed"`
	ErrorArtifacts []string               `json:"error_artifacts,omitempty"`
	Warnings       []string               `json:"warning_artifacts,omitempty"`
	Reports        []clusterArtifactEntry `json:"reports,omitempty"`
}

type clusterArtifactEntry struct {
	Name    string `json:"name"`
	Charts  int    `json:"charts"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Changed int    `json:"changed"`
}

func writeArtifactSummary(outputDir string, reports []output.ClusterReport) error {
	if outputDir == "" {
		return nil
	}
	summary := artifactSummary{
		ErrorArtifacts: listArtifactFiles(filepath.Join(outputDir, "errors")),
		Warnings:       listArtifactFiles(filepath.Join(outputDir, "warnings")),
	}
	for _, report := range reports {
		summary.Clusters++
		summary.Charts += len(report.Charts)
		summary.Added += report.Added
		summary.Removed += report.Removed
		summary.Changed += report.Changed
		summary.Reports = append(summary.Reports, clusterArtifactEntry{
			Name:    report.Name,
			Charts:  len(report.Charts),
			Added:   report.Added,
			Removed: report.Removed,
			Changed: report.Changed,
		})
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outputDir, artifactSummaryFilename), data, 0o644)
}
