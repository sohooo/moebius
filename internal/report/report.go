// Package report builds cluster diff reports from the current worktree and merge-base baseline.
package report

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/gitrepo"
	"github.com/sohooo/moebius/internal/helmrender"
	"github.com/sohooo/moebius/internal/output"
)

const renderWarningFilename = "render-warning.txt"
const renderNoticeFilename = "render-notices.txt"

func Build(opts cli.Options) ([]output.ClusterReport, string, error) {
	repo, err := gitrepo.Open(".")
	if err != nil {
		return nil, "", err
	}
	repoConfig, configMeta, err := config.LoadRepoConfigWithMetadata(repo.Root())
	if err != nil {
		return nil, "", err
	}
	layout := repoConfig.Layout
	layout.ClustersDir = repoConfig.EffectiveClustersDir(opts.ClustersDir)
	if len(opts.AppsFiles) > 0 {
		layout.Apps.Files = opts.AppsFiles
	}
	head, err := repo.ResolveCommit("HEAD")
	if err != nil {
		return nil, "", err
	}
	baseRefName, baseRef, err := repo.ResolveBaseRef(opts.BaseRef)
	if err != nil {
		return nil, "", err
	}
	mergeBase, err := repo.MergeBase(head, baseRef)
	if err != nil {
		return nil, "", err
	}
	changedPaths, err := repo.ChangedPaths(mergeBase, head)
	if err != nil {
		return nil, "", err
	}

	chartMode, err := shouldUseChartMode(repo, layout, opts, mergeBase)
	if err != nil {
		return nil, "", err
	}
	var clusters []string
	if !chartMode {
		clusters, err = selectClusters(repo, layout, opts, mergeBase, head, changedPaths)
		if err != nil {
			return nil, "", err
		}
		if len(clusters) == 0 {
			return nil, "", nil
		}
	}
	summary := newRunSummary(opts, repoConfig, configMeta, layout, modeName(chartMode), baseRefName, head.Hash.String(), mergeBase.Hash.String(), changedPaths, clusters)

	outputDir := opts.OutputDir
	cleanupOutput := false
	if outputDir == "" {
		outputDir, err = os.MkdirTemp("", "mobius-output-")
		if err != nil {
			return nil, "", err
		}
		cleanupOutput = true
	}
	tempRoot, err := os.MkdirTemp("", "mobius-work-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(tempRoot)
	if cleanupOutput {
		defer os.RemoveAll(outputDir)
	}

	cacheDir := filepath.Join(tempRoot, "helm-cache")
	baselineRoot := filepath.Join(tempRoot, "baseline-tree")
	for _, dir := range []string{cacheDir, baselineRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, "", err
		}
	}

	renderer := helmrender.New(cacheDir)
	currentOutput := filepath.Join(outputDir, "current")
	baselineOutput := filepath.Join(outputDir, "baseline")
	diffOutput := filepath.Join(outputDir, "diff")
	errorsOutput := filepath.Join(outputDir, "errors")
	warningsOutput := filepath.Join(outputDir, "warnings")
	for _, dir := range []string{currentOutput, baselineOutput, diffOutput, errorsOutput, warningsOutput} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, "", err
		}
	}

	var reports []output.ClusterReport
	defer func() {
		_ = writeRunSummaryArtifacts(outputDir, summary)
		if !cleanupOutput {
			_ = writeArtifactHTML(outputDir, reports, summary)
		}
		_ = writeArtifactIndex(outputDir, reports)
		_ = writeArtifactSummary(outputDir, reports)
	}()
	if chartMode {
		reports, err = buildChartModeReport(repo, repoConfig, opts, mergeBase, changedPaths, baselineRoot, currentOutput, baselineOutput, diffOutput, renderer, summary)
		if err != nil {
			return nil, "", err
		}
		return reports, outputDir, nil
	}
	for _, cluster := range clusters {
		currentExists := anyAppsFileExists(repo.Root(), layout, cluster)
		baselineExists, err := anyAppsFileExistsAtCommit(repo, mergeBase, layout, cluster)
		if err != nil {
			return nil, "", err
		}
		if !currentExists && !baselineExists {
			return nil, "", fmt.Errorf("cluster %q does not exist in current worktree or at merge-base", cluster)
		}

		if err := prepareBaselineClusterFiles(repo, mergeBase, layout, cluster, baselineRoot); err != nil {
			return nil, "", err
		}
		clusterSummary := runSummaryCluster{
			Name:              cluster,
			Status:            "considered",
			CurrentAppsFiles:  appsFilesExisting(repo.Root(), layout, cluster),
			BaselineAppsFiles: nil,
		}
		clusterSummary.BaselineAppsFiles, err = appsFilesExistingAtCommit(repo, mergeBase, layout, cluster)
		if err != nil {
			return nil, "", err
		}

		baselineReleases, baselineSources, _, err := loadReleaseInfoIfPresent(baselineRoot, layout, cluster)
		if err != nil {
			return nil, "", err
		}
		currentReleases, currentSources, currentWarnings, err := loadReleaseInfoIfPresent(repo.Root(), layout, cluster)
		if err != nil {
			return nil, "", err
		}
		for _, warning := range currentWarnings {
			clusterSummary.Warnings = append(clusterSummary.Warnings, warning.Message)
		}
		selection, err := planAffectedReleaseDetails(repo.Root(), baselineRoot, layout, cluster, currentExists, baselineExists, changedPaths, baselineReleases, currentReleases)
		if err != nil {
			return nil, "", err
		}
		clusterSummary.FallbackReason = selection.FallbackReason
		if selection.empty() {
			clusterSummary.Status = "skipped"
			clusterSummary.Releases = summarizeReleases(baselineReleases, currentReleases, baselineSources, currentSources, currentWarnings, selection, nil, baselineOutput, currentOutput, cluster)
			summary.addCluster(clusterSummary)
			continue
		}
		if err := prepareBaselineCharts(repo, mergeBase, layout, cluster, baselineRoot, baselineReleases, selection.releaseSelection); err != nil {
			return nil, "", err
		}
		if err := renderCluster(repo.Root(), layout, cluster, "current", currentOutput, selection.releaseSelection, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, "", err
		}
		if err := renderCluster(baselineRoot, layout, cluster, "baseline", baselineOutput, selection.releaseSelection, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, "", err
		}
		report, err := compareCluster(cluster, baselineOutput, currentOutput, diffOutput, opts.ContextLines, opts.Validate, diffIgnoreOptions(repoConfig), baselineReleases, currentReleases)
		if err != nil {
			return nil, "", err
		}
		if len(report.Charts) == 0 {
			clusterSummary.Status = "no_effective_changes"
			clusterSummary.Releases = summarizeReleases(baselineReleases, currentReleases, baselineSources, currentSources, currentWarnings, selection, nil, baselineOutput, currentOutput, cluster)
			summary.addCluster(clusterSummary)
			continue
		}
		clusterSummary.Status = "reported"
		clusterSummary.Releases = summarizeReleases(baselineReleases, currentReleases, baselineSources, currentSources, currentWarnings, selection, releaseReportResults(report), baselineOutput, currentOutput, cluster)
		summary.addCluster(clusterSummary)
		reports = append(reports, report)
	}

	return reports, outputDir, nil
}

func modeName(chartMode bool) string {
	if chartMode {
		return "chart_repository"
	}
	return "cluster_repository"
}

func diffIgnoreOptions(cfg config.RepoConfig) diff.IgnoreOptions {
	out := diff.IgnoreOptions{
		UseDefaults: cfg.Diff.Ignore.Defaults,
	}
	for _, rule := range cfg.Diff.Ignore.Metadata {
		out.Metadata = append(out.Metadata, diff.MetadataIgnoreRule{
			Locations:   rule.Locations,
			Labels:      rule.Labels,
			Annotations: rule.Annotations,
		})
	}
	return out
}

func loadReleasesIfPresent(root string, layout config.LayoutConfig, cluster string) (map[string]config.Release, error) {
	releases, _, _, err := loadReleaseInfoIfPresent(root, layout, cluster)
	return releases, err
}

func loadReleaseInfoIfPresent(root string, layout config.LayoutConfig, cluster string) (map[string]config.Release, map[string]string, []config.ReleaseWarning, error) {
	if !anyAppsFileExists(root, layout, cluster) {
		return map[string]config.Release{}, map[string]string{}, nil, nil
	}
	metadata, warnings, err := config.LoadReleaseMetadataWithWarnings(root, layout, cluster)
	if err != nil {
		return nil, nil, nil, err
	}
	out := make(map[string]config.Release, len(metadata))
	sources := make(map[string]string, len(metadata))
	for _, item := range metadata {
		out[item.Release.Name] = item.Release
		sources[item.Release.Name] = item.SourceFile
	}
	return out, sources, warnings, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func anyAppsFileExists(root string, layout config.LayoutConfig, cluster string) bool {
	for _, path := range config.AppsPaths(root, layout, cluster) {
		if fileExists(path) {
			return true
		}
	}
	return false
}

func appsFilesExisting(root string, layout config.LayoutConfig, cluster string) []string {
	var out []string
	for _, appsFile := range layout.Apps.Files {
		if fileExists(filepath.Join(config.ClusterDir(root, layout, cluster), filepath.FromSlash(appsFile))) {
			out = append(out, appsFile)
		}
	}
	return out
}

func anyAppsFileExistsAtCommit(repo *gitrepo.Repo, commit *object.Commit, layout config.LayoutConfig, cluster string) (bool, error) {
	for _, appsFile := range layout.Apps.Files {
		relPath := filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster, appsFile))
		exists, err := repo.PathExistsAtCommit(commit, relPath)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func appsFilesExistingAtCommit(repo *gitrepo.Repo, commit *object.Commit, layout config.LayoutConfig, cluster string) ([]string, error) {
	var out []string
	for _, appsFile := range layout.Apps.Files {
		relPath := filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster, appsFile))
		exists, err := repo.PathExistsAtCommit(commit, relPath)
		if err != nil {
			return nil, err
		}
		if exists {
			out = append(out, appsFile)
		}
	}
	return out, nil
}
