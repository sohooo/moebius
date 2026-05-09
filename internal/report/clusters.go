package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/gitrepo"
)

func selectClusters(repo *gitrepo.Repo, layout config.LayoutConfig, opts cli.Options, mergeBase, head *object.Commit) ([]string, error) {
	switch {
	case opts.Cluster != "":
		available, err := availableClusters(repo, layout, mergeBase)
		if err != nil {
			return nil, err
		}
		if !slicesContains(available, opts.Cluster) {
			if len(available) == 0 {
				return nil, fmt.Errorf("cluster %q does not exist in the effective layout under %q", opts.Cluster, layout.ClustersDir)
			}
			return nil, fmt.Errorf("cluster %q does not exist in the effective layout under %q; available clusters: %s", opts.Cluster, layout.ClustersDir, strings.Join(available, ", "))
		}
		return []string{opts.Cluster}, nil
	case opts.AllClusters:
		return repo.AllClusters(layout.ClustersDir, layout.Apps.File)
	default:
		return repo.ChangedClusters(layout.ClustersDir, mergeBase, head)
	}
}

func availableClusters(repo *gitrepo.Repo, layout config.LayoutConfig, mergeBase *object.Commit) ([]string, error) {
	current, err := repo.AllClusters(layout.ClustersDir, layout.Apps.File)
	if err != nil {
		return nil, err
	}
	baseline, err := repo.AllClustersAtCommit(mergeBase, layout.ClustersDir, layout.Apps.File)
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
