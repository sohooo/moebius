// Package report builds cluster diff reports from the current worktree and merge-base baseline.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"

	"mobius/internal/cli"
	"mobius/internal/config"
	"mobius/internal/diff"
	"mobius/internal/gitrepo"
	"mobius/internal/helmrender"
	"mobius/internal/output"
	"mobius/internal/resources"
	"mobius/internal/severity"
	"mobius/internal/validate"
)

func Build(opts cli.Options) ([]output.ClusterReport, string, error) {
	repo, err := gitrepo.Open(".")
	if err != nil {
		return nil, "", err
	}
	repoConfig, err := config.LoadRepoConfig(repo.Root())
	if err != nil {
		return nil, "", err
	}
	layout := repoConfig.Layout
	layout.ClustersDir = repoConfig.EffectiveClustersDir(opts.ClustersDir)
	head, err := repo.ResolveCommit("HEAD")
	if err != nil {
		return nil, "", err
	}
	baseRef, err := repo.ResolveCommit(opts.BaseRef)
	if err != nil {
		return nil, "", err
	}
	mergeBase, err := repo.MergeBase(head, baseRef)
	if err != nil {
		return nil, "", err
	}

	clusters, err := selectClusters(repo, layout, opts, mergeBase, head)
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
	for _, dir := range []string{currentOutput, baselineOutput, diffOutput} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, "", err
		}
	}

	var reports []output.ClusterReport
	for _, cluster := range clusters {
		currentExists := fileExists(config.AppsPath(repo.Root(), layout, cluster))
		baselineExists, err := repo.PathExistsAtCommit(mergeBase, filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster, layout.Apps.File)))
		if err != nil {
			return nil, "", err
		}
		if !currentExists && !baselineExists {
			return nil, "", fmt.Errorf("cluster %q does not exist in current worktree or at merge-base", cluster)
		}

		if err := prepareBaselineCluster(repo, mergeBase, layout, cluster, baselineRoot); err != nil {
			return nil, "", err
		}
		if err := renderCluster(repo.Root(), layout, cluster, currentOutput, renderer); err != nil {
			return nil, "", err
		}
		if err := renderCluster(baselineRoot, layout, cluster, baselineOutput, renderer); err != nil {
			return nil, "", err
		}

		report, err := compareCluster(cluster, baselineOutput, currentOutput, diffOutput, opts.ContextLines, opts.Validate)
		if err != nil {
			return nil, "", err
		}
		reports = append(reports, report)
	}

	return reports, outputDir, nil
}

func selectClusters(repo *gitrepo.Repo, layout config.LayoutConfig, opts cli.Options, mergeBase, head *object.Commit) ([]string, error) {
	switch {
	case opts.Cluster != "":
		return []string{opts.Cluster}, nil
	case opts.AllClusters:
		return repo.AllClusters(layout.ClustersDir)
	default:
		return repo.ChangedClusters(layout.ClustersDir, mergeBase, head)
	}
}

func prepareBaselineCluster(repo *gitrepo.Repo, mergeBase *object.Commit, layout config.LayoutConfig, cluster, baselineRoot string) error {
	clusterRel := filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster))
	exists, err := repo.PathExistsAtCommit(mergeBase, clusterRel)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	if err := repo.WriteDirAtCommit(mergeBase, clusterRel, baselineRoot); err != nil {
		return err
	}

	releases, err := config.LoadReleases(baselineRoot, layout, cluster)
	if err != nil {
		return err
	}
	for _, release := range releases {
		if strings.HasPrefix(release.Chart, "oci://") {
			if release.Version == "" {
				return fmt.Errorf("cluster %q baseline release %q uses OCI chart without version", cluster, release.Name)
			}
			continue
		}
		chartPrefix := filepath.ToSlash(release.Chart)
		if exists, err := repo.PathExistsAtCommit(mergeBase, chartPrefix); err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("cluster %q baseline release %q references missing chart path %q at merge-base", cluster, release.Name, release.Chart)
		}
		if err := repo.WriteDirAtCommit(mergeBase, chartPrefix, baselineRoot); err != nil {
			return err
		}
	}
	return nil
}

func renderCluster(root string, layout config.LayoutConfig, cluster, outputRoot string, renderer *helmrender.Renderer) error {
	appsPath := config.AppsPath(root, layout, cluster)
	if !fileExists(appsPath) {
		return nil
	}
	releases, err := config.LoadReleases(root, layout, cluster)
	if err != nil {
		return err
	}
	clusterDir := filepath.Join(outputRoot, cluster)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		return err
	}

	for _, release := range releases {
		overridePath := config.ResolveOverridePath(root, layout, cluster, release)
		if !fileExists(overridePath) {
			overridePath = ""
		}
		rendered, err := renderer.Render(root, release.Chart, release.Version, release.Name, release.Namespace, overridePath)
		if err != nil {
			return fmt.Errorf("render cluster %q release %q: %w", cluster, release.Name, err)
		}

		chartDir := filepath.Join(clusterDir, release.Name)
		resourceDir := filepath.Join(chartDir, "resources")
		if err := os.MkdirAll(resourceDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(chartDir, "namespace.txt"), []byte(release.Namespace+"\n"), 0o644); err != nil {
			return err
		}
		renderedPath := filepath.Join(chartDir, "rendered.yaml")
		if err := os.WriteFile(renderedPath, []byte(rendered), 0o644); err != nil {
			return err
		}
		if _, err := resources.SplitRendered(rendered, resourceDir); err != nil {
			return err
		}
	}
	return nil
}

func compareCluster(cluster, baselineOutput, currentOutput, diffOutput string, contextLines int, doValidate bool) (output.ClusterReport, error) {
	report := output.ClusterReport{Name: cluster}

	chartNames, err := unionDirs(filepath.Join(baselineOutput, cluster), filepath.Join(currentOutput, cluster))
	if err != nil {
		return report, err
	}

	for _, chartName := range chartNames {
		baselineChartDir := filepath.Join(baselineOutput, cluster, chartName)
		currentChartDir := filepath.Join(currentOutput, cluster, chartName)
		namespace := firstNonEmpty(readFirstLine(filepath.Join(currentChartDir, "namespace.txt")), readFirstLine(filepath.Join(baselineChartDir, "namespace.txt")))

		baselineResources, err := resources.LoadDir(filepath.Join(baselineChartDir, "resources"))
		if err != nil {
			return report, err
		}
		currentResources, err := resources.LoadDir(filepath.Join(currentChartDir, "resources"))
		if err != nil {
			return report, err
		}
		schemaResolver := validate.NewSchemaResolver(currentResources)
		duplicateCounts := resourceIdentityCounts(currentResources)
		resourceKeys := unionKeys(baselineResources, currentResources)
		if len(resourceKeys) == 0 {
			continue
		}

		chartReport := output.ChartReport{Name: chartName, Namespace: namespace}
		chartDiffDir := filepath.Join(diffOutput, cluster, chartName)
		if err := os.MkdirAll(chartDiffDir, 0o755); err != nil {
			return report, err
		}

		for _, resourceKey := range resourceKeys {
			oldResource, oldOK := baselineResources[resourceKey]
			newResource, newOK := currentResources[resourceKey]
			state := "changed"
			switch {
			case !oldOK:
				state = "added"
			case !newOK:
				state = "removed"
			}

			oldPath, newPath := oldResource.Path, newResource.Path
			oldValue, newValue := oldResource.Value, newResource.Value
			kind, name, namespace := newResource.Kind, newResource.Name, newResource.Namespace
			if !newOK {
				kind, name, namespace = oldResource.Kind, oldResource.Name, oldResource.Namespace
			}

			result, err := diff.Compare(oldPath, newPath, oldValue, newValue, contextLines)
			if err != nil {
				return report, err
			}
			if !result.HasChanges {
				continue
			}

			rawPath := filepath.Join(chartDiffDir, resourceKey+".diff")
			if strings.TrimSpace(result.RawDiff) != "" {
				if err := os.WriteFile(rawPath, []byte(result.RawDiff), 0o644); err != nil {
					return report, err
				}
			}

			semanticText, err := diff.RenderSemanticReport(result.Changes)
			if err != nil {
				return report, err
			}
			if strings.TrimSpace(semanticText) != "" {
				if err := os.WriteFile(filepath.Join(chartDiffDir, resourceKey+".semantic.txt"), []byte(semanticText), 0o644); err != nil {
					return report, err
				}
			}

			switch state {
			case "added":
				report.Added++
			case "removed":
				report.Removed++
			default:
				report.Changed++
			}

			assessment := severity.Assess(severity.Input{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
				State:     state,
				Changes:   result.Changes,
			})
			validationResult := validate.Result{Status: validate.StatusValid}
			if doValidate && newOK {
				validationResult = validate.Validate(validate.Input{
					Resource:   newResource,
					Siblings:   currentResources,
					Duplicates: duplicateCounts,
					Resolver:   schemaResolver,
				})
			}

			chartReport.Resources = append(chartReport.Resources, output.ResourceReport{
				State:      state,
				Kind:       kind,
				Name:       name,
				Namespace:  namespace,
				Result:     result,
				Semantic:   semanticText,
				Assessment: assessment,
				Validation: validationResult,
			})
		}

		if len(chartReport.Resources) > 0 {
			report.Charts = append(report.Charts, chartReport)
		}
	}

	return report, nil
}

func unionDirs(paths ...string) ([]string, error) {
	set := map[string]struct{}{}
	for _, path := range paths {
		entries, err := os.ReadDir(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				set[entry.Name()] = struct{}{}
			}
		}
	}
	return sortedSet(set), nil
}

func unionKeys(left, right map[string]resources.Resource) []string {
	set := map[string]struct{}{}
	for key := range left {
		set[key] = struct{}{}
	}
	for key := range right {
		set[key] = struct{}{}
	}
	return sortedSet(set)
}

func sortedSet(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func readFirstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resourceIdentityCounts(resourcesByKey map[string]resources.Resource) map[string]int {
	counts := map[string]int{}
	for _, resource := range resourcesByKey {
		counts[resource.Identity]++
	}
	return counts
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
