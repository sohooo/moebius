package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/gitrepo"
)

func selectClusters(repo *gitrepo.Repo, layout config.LayoutConfig, opts cli.Options, mergeBase, head *object.Commit, changedPaths []string) ([]string, error) {
	switch {
	case opts.Cluster != "":
		available, err := availableClusters(repo, layout, mergeBase)
		if err != nil {
			return nil, err
		}
		if !slicesContains(available, opts.Cluster) {
			if len(available) == 0 {
				return nil, fmt.Errorf("cluster %q does not exist in the effective layout under %q (apps files: %s)", opts.Cluster, layout.ClustersDir, config.AppsFilesSummary(layout))
			}
			return nil, fmt.Errorf("cluster %q does not exist in the effective layout under %q (apps files: %s); available clusters: %s", opts.Cluster, layout.ClustersDir, config.AppsFilesSummary(layout), strings.Join(available, ", "))
		}
		return []string{opts.Cluster}, nil
	case opts.AllClusters:
		return repo.AllClustersForAppsFiles(layout.ClustersDir, layout.Apps.Files)
	default:
		clusters, err := repo.ChangedClusters(layout.ClustersDir, mergeBase, head)
		if err != nil {
			return nil, err
		}
		if hasNonClusterChange(layout, changedPaths) {
			available, err := availableClusters(repo, layout, mergeBase)
			if err != nil {
				return nil, err
			}
			set := map[string]struct{}{}
			for _, cluster := range clusters {
				set[cluster] = struct{}{}
			}
			for _, cluster := range available {
				set[cluster] = struct{}{}
			}
			clusters = make([]string, 0, len(set))
			for cluster := range set {
				clusters = append(clusters, cluster)
			}
			sort.Strings(clusters)
		}
		return clusters, nil
	}
}

func hasNonClusterChange(layout config.LayoutConfig, changedPaths []string) bool {
	prefix := strings.TrimSuffix(filepath.ToSlash(layout.ClustersDir), "/") + "/"
	for _, path := range changedPaths {
		if !strings.HasPrefix(filepath.ToSlash(path), prefix) {
			return true
		}
	}
	return false
}

func availableClusters(repo *gitrepo.Repo, layout config.LayoutConfig, mergeBase *object.Commit) ([]string, error) {
	current, err := repo.AllClustersForAppsFiles(layout.ClustersDir, layout.Apps.Files)
	if err != nil {
		return nil, err
	}
	baseline, err := repo.AllClustersAtCommitForAppsFiles(mergeBase, layout.ClustersDir, layout.Apps.Files)
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, cluster := range current {
		set[cluster] = struct{}{}
	}
	for _, cluster := range baseline {
		set[cluster] = struct{}{}
	}
	clusters := make([]string, 0, len(set))
	for cluster := range set {
		clusters = append(clusters, cluster)
	}
	sort.Strings(clusters)
	return clusters, nil
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
