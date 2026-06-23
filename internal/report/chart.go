package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
	"gopkg.in/yaml.v3"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/gitrepo"
	"github.com/sohooo/moebius/internal/helmrender"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/resources"
)

const chartModeCluster = "chart"

type chartMetadata struct {
	Name string `yaml:"name"`
}

func shouldUseChartMode(repo *gitrepo.Repo, layout config.LayoutConfig, opts cli.Options, mergeBase *object.Commit) (bool, error) {
	if opts.ChartPath != "" {
		return true, nil
	}
	if opts.Cluster != "" || opts.AllClusters || opts.ClustersDir != "" || len(opts.AppsFiles) > 0 {
		return false, nil
	}
	if !fileExists(filepath.Join(repo.Root(), "Chart.yaml")) {
		return false, nil
	}
	currentClusters, err := repo.AllClustersForAppsFiles(layout.ClustersDir, layout.Apps.Files)
	if err != nil {
		return false, err
	}
	baselineClusters, err := repo.AllClustersAtCommitForAppsFiles(mergeBase, layout.ClustersDir, layout.Apps.Files)
	if err != nil {
		return false, err
	}
	if len(currentClusters) == 0 && len(baselineClusters) == 0 {
		return true, nil
	}
	return false, fmt.Errorf("repository contains both a root Chart.yaml and discoverable clusters; use --chart-path for chart mode or --cluster/--all-clusters for cluster mode")
}

func buildChartModeReport(repo *gitrepo.Repo, repoConfig config.RepoConfig, opts cli.Options, mergeBase *object.Commit, changedPaths []string, baselineRoot, currentOutput, baselineOutput, diffOutput string, renderer *helmrender.Renderer) ([]output.ClusterReport, error) {
	chartPath := opts.ChartPath
	if chartPath == "" {
		chartPath = "."
	}
	chartPath = filepath.ToSlash(filepath.Clean(chartPath))
	currentExists := fileExists(filepath.Join(repo.Root(), filepath.FromSlash(chartPath), "Chart.yaml"))
	baselineChartYAML := filepath.ToSlash(filepath.Join(chartPath, "Chart.yaml"))
	if chartPath == "." {
		baselineChartYAML = "Chart.yaml"
	}
	baselineExists, err := repo.PathExistsAtCommit(mergeBase, baselineChartYAML)
	if err != nil {
		return nil, err
	}
	if !currentExists && !baselineExists {
		return nil, fmt.Errorf("chart %q does not exist in current worktree or at merge-base", chartPath)
	}
	if !chartPathChanged(chartPath, changedPaths) && currentExists == baselineExists {
		return nil, nil
	}
	if baselineExists {
		if err := repo.WriteDirAtCommit(mergeBase, chartPath, baselineRoot); err != nil {
			return nil, err
		}
	}

	releaseName, err := chartReleaseName(repo.Root(), baselineRoot, chartPath, opts.ReleaseName, currentExists)
	if err != nil {
		return nil, err
	}
	currentRelease := config.Release{Name: releaseName, Namespace: opts.Namespace, Chart: chartPath}
	baselineRelease := currentRelease
	currentReleases := map[string]config.Release{}
	baselineReleases := map[string]config.Release{}
	if currentExists {
		currentReleases[releaseName] = currentRelease
		valuesFiles, err := chartValuesFiles(repo.Root(), chartPath, opts)
		if err != nil {
			return nil, err
		}
		if err := renderChartModeRelease(repo.Root(), chartPath, chartModeCluster, releaseName, opts.Namespace, "current", currentOutput, valuesFiles, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, err
		}
	}
	if baselineExists {
		baselineReleases[releaseName] = baselineRelease
		valuesFiles, err := chartValuesFiles(baselineRoot, chartPath, opts)
		if err != nil {
			return nil, err
		}
		if err := renderChartModeRelease(baselineRoot, chartPath, chartModeCluster, releaseName, opts.Namespace, "baseline", baselineOutput, valuesFiles, renderer, opts.RenderErrorMode, opts.DuplicateKeyMode); err != nil {
			return nil, err
		}
	}

	report, err := compareCluster(chartModeCluster, baselineOutput, currentOutput, diffOutput, opts.ContextLines, opts.Validate, diffIgnoreOptions(repoConfig), baselineReleases, currentReleases)
	if err != nil {
		return nil, err
	}
	if len(report.Charts) == 0 {
		return nil, nil
	}
	return []output.ClusterReport{report}, nil
}

func chartReleaseName(root, baselineRoot, chartPath, override string, preferCurrent bool) (string, error) {
	if override != "" {
		return override, nil
	}
	roots := []string{root, baselineRoot}
	if !preferCurrent {
		roots = []string{baselineRoot, root}
	}
	for _, candidateRoot := range roots {
		name, err := readChartName(filepath.Join(candidateRoot, filepath.FromSlash(chartPath), "Chart.yaml"))
		if err == nil && name != "" {
			return name, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("chart %q does not define a name in Chart.yaml; set --release-name", chartPath)
}

func readChartName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var meta chartMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return strings.TrimSpace(meta.Name), nil
}

func chartValuesFiles(root, chartPath string, opts cli.Options) ([]string, error) {
	chartRoot := filepath.Join(root, filepath.FromSlash(chartPath))
	if len(opts.ValuesFiles) == 0 {
		path := filepath.Join(chartRoot, "values.yaml")
		if fileExists(path) {
			return []string{path}, nil
		}
		return nil, nil
	}
	out := make([]string, 0, len(opts.ValuesFiles))
	for _, valueFile := range opts.ValuesFiles {
		path := filepath.Join(chartRoot, filepath.FromSlash(valueFile))
		if !fileExists(path) {
			return nil, fmt.Errorf("values file %q does not exist under chart %q", valueFile, chartPath)
		}
		out = append(out, path)
	}
	return out, nil
}

func chartPathChanged(chartPath string, changedPaths []string) bool {
	if chartPath == "." {
		return len(changedPaths) > 0
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(chartPath), "/") + "/"
	for _, path := range changedPaths {
		path = filepath.ToSlash(path)
		if path == chartPath || strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func renderChartModeRelease(root, chartPath, cluster, releaseName, namespace, state, outputRoot string, valuesFiles []string, renderer *helmrender.Renderer, mode cli.RenderErrorMode, duplicateKeyMode cli.DuplicateKeyMode) error {
	clusterDir := filepath.Join(outputRoot, cluster)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		return err
	}
	chartRef := filepath.ToSlash(chartPath)
	rendered, err := renderer.RenderWithValuesFiles(root, chartRef, "", "", releaseName, namespace, valuesFiles)
	if err != nil {
		release := config.Release{Name: releaseName, Namespace: namespace, Chart: chartPath}
		warning := renderFailureWarning(cluster, release, chartRef, state, err)
		if writeErr := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "warnings"), state, cluster, releaseName, []string{warning}); writeErr != nil {
			return writeErr
		}
		if mode == cli.RenderErrorModeWarnSkipRelease {
			return writeSkippedReleaseWarning(clusterDir, release, warning)
		}
		return fmt.Errorf("render chart %q release %q: %w", chartPath, releaseName, err)
	}

	chartDir := filepath.Join(clusterDir, releaseName)
	resourceDir := filepath.Join(chartDir, "resources")
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(chartDir, "namespace.txt"), []byte(namespace+"\n"), 0o644); err != nil {
		return err
	}
	renderedPath := filepath.Join(chartDir, "rendered.yaml")
	if err := os.WriteFile(renderedPath, []byte(rendered), 0o644); err != nil {
		return err
	}
	_, notices, err := resources.SplitRendered(rendered, resourceDir, resources.SplitOptions{
		DuplicateKeyMode: optsDuplicateMode(duplicateKeyMode),
	})
	if err != nil {
		message := fmt.Sprintf("chart %q release %q produced invalid %s rendered YAML (rendered manifest: %s): %v", chartPath, releaseName, state, renderedPath, err)
		if writeErr := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "errors"), state, cluster, releaseName, []string{message}); writeErr != nil {
			return writeErr
		}
		if mode == cli.RenderErrorModeWarnSkipRelease {
			return os.WriteFile(filepath.Join(chartDir, renderWarningFilename), []byte(message+"\n"), 0o644)
		}
		return fmt.Errorf("%s", message)
	}
	if len(notices) > 0 {
		lines := make([]string, 0, len(notices))
		for _, notice := range notices {
			lines = append(lines, fmt.Sprintf("chart %q release %q %s render warning: %s", chartPath, releaseName, state, notice))
		}
		if err := os.WriteFile(filepath.Join(chartDir, renderNoticeFilename), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return err
		}
		if err := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "warnings"), state, cluster, releaseName, lines); err != nil {
			return err
		}
	}
	return nil
}
