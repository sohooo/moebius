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
	repoConfig, _, err := config.LoadRepoConfigWithMetadata(repo.Root())
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
	_, baseRef, err := repo.ResolveBaseRef(opts.BaseRef)
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

	clusters, err := selectClusters(repo, layout, opts, mergeBase, head, changedPaths)
	if err != nil {
		return nil, "", err
	}
	if len(clusters) == 0 {
		return nil, "", nil
	}

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
		_ = writeArtifactIndex(outputDir, reports)
		_ = writeArtifactSummary(outputDir, reports)
	}()
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
		baselineReleases, err := loadReleasesIfPresent(baselineRoot, layout, cluster)
		if err != nil {
			return nil, "", err
		}
		currentReleases, err := loadReleasesIfPresent(repo.Root(), layout, cluster)
		if err != nil {
			return nil, "", err
		}
		selection, err := planAffectedReleases(repo.Root(), baselineRoot, layout, cluster, currentExists, baselineExists, changedPaths, baselineReleases, currentReleases)
		if err != nil {
			return nil, "", err
		}
		if selection.empty() {
			continue
		}
		if err := prepareBaselineCharts(repo, mergeBase, layout, cluster, baselineRoot, baselineReleases, selection); err != nil {
			return nil, "", err
		}
		if err := renderCluster(repo.Root(), layout, cluster, "current", currentOutput, selection, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, "", err
		}
		if err := renderCluster(baselineRoot, layout, cluster, "baseline", baselineOutput, selection, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, "", err
		}
		report, err := compareCluster(cluster, baselineOutput, currentOutput, diffOutput, opts.ContextLines, opts.Validate, diffIgnoreOptions(repoConfig), baselineReleases, currentReleases)
		if err != nil {
			return nil, "", err
		}
		if len(report.Charts) == 0 {
			continue
		}
		reports = append(reports, report)
	}

	return reports, outputDir, nil
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
	if !anyAppsFileExists(root, layout, cluster) {
		return map[string]config.Release{}, nil
	}
	releases, err := config.LoadReleases(root, layout, cluster)
	if err != nil {
		return nil, err
	}
	out := make(map[string]config.Release, len(releases))
	for _, release := range releases {
		out[release.Name] = release
	}
	return out, nil
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
