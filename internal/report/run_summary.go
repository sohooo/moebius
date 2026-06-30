package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/output"
)

const runSummaryMarkdownFilename = "run-summary.md"
const runSummaryJSONFilename = "run-summary.json"

type runSummary struct {
	Mode              string               `json:"mode"`
	BaseRef           string               `json:"base_ref"`
	HeadSHA           string               `json:"head_sha"`
	MergeBaseSHA      string               `json:"merge_base_sha"`
	ConfigSources     string               `json:"config_sources"`
	Layout            runSummaryLayout     `json:"layout"`
	DiffIgnore        runSummaryDiffIgnore `json:"diff_ignore"`
	Options           runSummaryOptions    `json:"options"`
	ChangedPathsCount int                  `json:"changed_paths_count"`
	SelectedClusters  []string             `json:"selected_clusters,omitempty"`
	Clusters          []runSummaryCluster  `json:"clusters,omitempty"`
}

type runSummaryLayout struct {
	ClustersDir  string   `json:"clusters_dir"`
	AppsFiles    []string `json:"apps_files"`
	OverridePath string   `json:"override_path"`
	FallbackPath string   `json:"fallback_path,omitempty"`
}

type runSummaryDiffIgnore struct {
	Defaults      bool `json:"defaults"`
	MetadataRules int  `json:"metadata_rules"`
}

type runSummaryOptions struct {
	Cluster          string   `json:"cluster,omitempty"`
	AllClusters      bool     `json:"all_clusters"`
	ChartPath        string   `json:"chart_path,omitempty"`
	ValuesFiles      []string `json:"values_files,omitempty"`
	ReleaseName      string   `json:"release_name,omitempty"`
	Namespace        string   `json:"namespace,omitempty"`
	RenderErrorMode  string   `json:"render_error_mode"`
	DuplicateKeyMode string   `json:"duplicate_key_mode"`
	Validate         bool     `json:"validate"`
	ContextLines     int      `json:"context_lines"`
	CommentMode      string   `json:"comment_mode,omitempty"`
}

type runSummaryCluster struct {
	Name              string              `json:"name"`
	Status            string              `json:"status"`
	CurrentAppsFiles  []string            `json:"current_apps_files,omitempty"`
	BaselineAppsFiles []string            `json:"baseline_apps_files,omitempty"`
	FallbackReason    string              `json:"fallback_reason,omitempty"`
	Warnings          []string            `json:"warnings,omitempty"`
	Releases          []runSummaryRelease `json:"releases,omitempty"`
}

type runSummaryRelease struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace,omitempty"`
	SourceFile       string   `json:"source_file,omitempty"`
	BaselineSource   string   `json:"baseline_source_file,omitempty"`
	CurrentExists    bool     `json:"current_exists"`
	BaselineExists   bool     `json:"baseline_exists"`
	Decision         string   `json:"decision"`
	Reasons          []string `json:"reasons,omitempty"`
	RenderResult     string   `json:"render_result"`
	ReportResult     string   `json:"report_result"`
	DuplicateWarning string   `json:"duplicate_warning,omitempty"`
}

func newRunSummary(opts cli.Options, cfg config.RepoConfig, meta config.LoadMetadata, layout config.LayoutConfig, mode, baseRef, headSHA, mergeBaseSHA string, changedPaths []string, selectedClusters []string) *runSummary {
	return &runSummary{
		Mode:              mode,
		BaseRef:           baseRef,
		HeadSHA:           headSHA,
		MergeBaseSHA:      mergeBaseSHA,
		ConfigSources:     meta.SourceSummary(),
		ChangedPathsCount: len(changedPaths),
		SelectedClusters:  append([]string(nil), selectedClusters...),
		Layout: runSummaryLayout{
			ClustersDir:  layout.ClustersDir,
			AppsFiles:    append([]string(nil), layout.Apps.Files...),
			OverridePath: layout.Overrides.Path,
			FallbackPath: layout.Overrides.FallbackPath,
		},
		DiffIgnore: runSummaryDiffIgnore{
			Defaults:      cfg.Diff.Ignore.Defaults,
			MetadataRules: len(cfg.Diff.Ignore.Metadata),
		},
		Options: runSummaryOptions{
			Cluster:          opts.Cluster,
			AllClusters:      opts.AllClusters,
			ChartPath:        opts.ChartPath,
			ValuesFiles:      append([]string(nil), opts.ValuesFiles...),
			ReleaseName:      opts.ReleaseName,
			Namespace:        opts.Namespace,
			RenderErrorMode:  string(opts.RenderErrorMode),
			DuplicateKeyMode: string(opts.DuplicateKeyMode),
			Validate:         opts.Validate,
			ContextLines:     opts.ContextLines,
			CommentMode:      string(opts.CommentMode),
		},
	}
}

func (s *runSummary) addCluster(cluster runSummaryCluster) {
	sort.Slice(cluster.Releases, func(i, j int) bool {
		return cluster.Releases[i].Name < cluster.Releases[j].Name
	})
	s.Clusters = append(s.Clusters, cluster)
}

func writeRunSummaryArtifacts(outputDir string, summary *runSummary) error {
	if outputDir == "" || summary == nil {
		return nil
	}
	if err := writeRunSummaryJSON(outputDir, summary); err != nil {
		return err
	}
	return writeRunSummaryMarkdown(outputDir, summary)
}

func writeRunSummaryJSON(outputDir string, summary *runSummary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outputDir, runSummaryJSONFilename), data, 0o644)
}

func writeRunSummaryMarkdown(outputDir string, summary *runSummary) error {
	var b strings.Builder
	fmt.Fprintln(&b, "# møbius Run Summary")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Effective Configuration")
	fmt.Fprintf(&b, "- Mode: `%s`\n", summary.Mode)
	fmt.Fprintf(&b, "- Config sources: `%s`\n", summary.ConfigSources)
	fmt.Fprintf(&b, "- Base ref: `%s`\n", summary.BaseRef)
	fmt.Fprintf(&b, "- Merge-base: `%s`\n", summary.MergeBaseSHA)
	fmt.Fprintf(&b, "- Head: `%s`\n", summary.HeadSHA)
	fmt.Fprintf(&b, "- Clusters dir: `%s`\n", summary.Layout.ClustersDir)
	fmt.Fprintf(&b, "- Apps files: `%s`\n", strings.Join(summary.Layout.AppsFiles, ","))
	fmt.Fprintf(&b, "- Override path: `%s`\n", summary.Layout.OverridePath)
	if summary.Layout.FallbackPath != "" {
		fmt.Fprintf(&b, "- Fallback override path: `%s`\n", summary.Layout.FallbackPath)
	}
	fmt.Fprintf(&b, "- Diff ignore defaults: `%t`\n", summary.DiffIgnore.Defaults)
	fmt.Fprintf(&b, "- Diff ignore metadata rules: `%d`\n", summary.DiffIgnore.MetadataRules)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Selected Inputs")
	fmt.Fprintf(&b, "- Changed paths: `%d`\n", summary.ChangedPathsCount)
	if len(summary.SelectedClusters) > 0 {
		fmt.Fprintf(&b, "- Selected clusters: `%s`\n", strings.Join(summary.SelectedClusters, ","))
	}
	if summary.Options.ChartPath != "" {
		fmt.Fprintf(&b, "- Chart path: `%s`\n", summary.Options.ChartPath)
	}
	if len(summary.Options.ValuesFiles) > 0 {
		fmt.Fprintf(&b, "- Values files: `%s`\n", strings.Join(summary.Options.ValuesFiles, ","))
	}
	fmt.Fprintf(&b, "- Render error mode: `%s`\n", summary.Options.RenderErrorMode)
	fmt.Fprintf(&b, "- Duplicate key mode: `%s`\n", summary.Options.DuplicateKeyMode)
	fmt.Fprintf(&b, "- Validation: `%t`\n", summary.Options.Validate)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Clusters")
	if len(summary.Clusters) == 0 {
		fmt.Fprintln(&b, "_No clusters or chart inputs were processed._")
	} else {
		for _, cluster := range summary.Clusters {
			fmt.Fprintf(&b, "### `%s`\n\n", cluster.Name)
			fmt.Fprintf(&b, "- Status: `%s`\n", cluster.Status)
			if len(cluster.CurrentAppsFiles) > 0 {
				fmt.Fprintf(&b, "- Current apps files: `%s`\n", strings.Join(cluster.CurrentAppsFiles, ","))
			}
			if len(cluster.BaselineAppsFiles) > 0 {
				fmt.Fprintf(&b, "- Baseline apps files: `%s`\n", strings.Join(cluster.BaselineAppsFiles, ","))
			}
			if cluster.FallbackReason != "" {
				fmt.Fprintf(&b, "- Fallback: `%s`\n", cluster.FallbackReason)
			}
			for _, warning := range cluster.Warnings {
				fmt.Fprintf(&b, "- Warning: %s\n", warning)
			}
			fmt.Fprintln(&b)
			renderRunSummaryReleaseTable(&b, cluster.Releases)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Warnings And Fallbacks")
	found := false
	for _, cluster := range summary.Clusters {
		if cluster.FallbackReason != "" {
			found = true
			fmt.Fprintf(&b, "- `%s`: `%s`\n", cluster.Name, cluster.FallbackReason)
		}
		for _, warning := range cluster.Warnings {
			found = true
			fmt.Fprintf(&b, "- `%s`: %s\n", cluster.Name, warning)
		}
	}
	if !found {
		fmt.Fprintln(&b, "_No warnings or full-cluster fallbacks recorded._")
	}
	return os.WriteFile(filepath.Join(outputDir, runSummaryMarkdownFilename), []byte(b.String()), 0o644)
}

func renderRunSummaryReleaseTable(b *strings.Builder, releases []runSummaryRelease) {
	if len(releases) == 0 {
		fmt.Fprintln(b, "_No releases recorded._")
		fmt.Fprintln(b)
		return
	}
	fmt.Fprintln(b, "| Release | Source | Decision | Reasons | Render | Report |")
	fmt.Fprintln(b, "| --- | --- | --- | --- | --- | --- |")
	for _, release := range releases {
		source := release.SourceFile
		if source == "" {
			source = release.BaselineSource
		}
		fmt.Fprintf(b, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			release.Name,
			source,
			release.Decision,
			strings.Join(release.Reasons, ","),
			release.RenderResult,
			release.ReportResult,
		)
	}
	fmt.Fprintln(b)
}

func releaseReportResults(report output.ClusterReport) map[string]string {
	out := map[string]string{}
	for _, chart := range report.Charts {
		switch {
		case len(chart.Resources) > 0:
			out[chart.Name] = "produced_changes"
		case chart.RenderWarning != "" || len(chart.Warnings) > 0:
			out[chart.Name] = "warning_only"
		default:
			out[chart.Name] = "no_effective_changes"
		}
	}
	return out
}

func summarizeReleases(baselineReleases, currentReleases map[string]config.Release, baselineSources, currentSources map[string]string, warnings []config.ReleaseWarning, selection affectedPlan, reportResults map[string]string, baselineOutput, currentOutput, cluster string) []runSummaryRelease {
	names := map[string]struct{}{}
	for name := range baselineReleases {
		names[name] = struct{}{}
	}
	for name := range currentReleases {
		names[name] = struct{}{}
	}
	warningsByRelease := map[string]string{}
	for _, warning := range warnings {
		warningsByRelease[warning.ReleaseName] = warning.Message
		names[warning.ReleaseName] = struct{}{}
	}
	var out []runSummaryRelease
	for name := range names {
		currentRelease, currentOK := currentReleases[name]
		baselineRelease, baselineOK := baselineReleases[name]
		release := currentRelease
		if !currentOK {
			release = baselineRelease
		}
		decision := "skipped"
		if selection.includes(name) {
			decision = "selected"
		}
		reportResult := "not_reported"
		if value := reportResults[name]; value != "" {
			reportResult = value
		} else if selection.includes(name) {
			reportResult = "no_effective_changes_after_diff_ignore"
		}
		out = append(out, runSummaryRelease{
			Name:             name,
			Namespace:        release.Namespace,
			SourceFile:       currentSources[name],
			BaselineSource:   baselineSources[name],
			CurrentExists:    currentOK,
			BaselineExists:   baselineOK,
			Decision:         decision,
			Reasons:          selection.Reasons[name],
			RenderResult:     releaseRenderResult(baselineOutput, currentOutput, cluster, name, selection.includes(name)),
			ReportResult:     reportResult,
			DuplicateWarning: warningsByRelease[name],
		})
	}
	return out
}

func releaseRenderResult(baselineOutput, currentOutput, cluster, release string, selected bool) string {
	if !selected {
		return "not_rendered"
	}
	for _, root := range []string{currentOutput, baselineOutput} {
		chartDir := filepath.Join(root, cluster, release)
		if fileExists(filepath.Join(chartDir, renderWarningFilename)) {
			return "warn_skipped"
		}
		if fileExists(filepath.Join(chartDir, "rendered.yaml")) {
			return "rendered"
		}
	}
	return "not_rendered"
}
