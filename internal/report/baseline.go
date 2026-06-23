package report

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/gitrepo"
)

func prepareBaselineClusterFiles(repo *gitrepo.Repo, mergeBase *object.Commit, layout config.LayoutConfig, cluster, baselineRoot string) error {
	clusterRel := filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster))
	exists, err := repo.PathExistsAtCommit(mergeBase, clusterRel)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	return repo.WriteDirAtCommit(mergeBase, clusterRel, baselineRoot)
}

func prepareBaselineCharts(repo *gitrepo.Repo, mergeBase *object.Commit, layout config.LayoutConfig, cluster, baselineRoot string, releases map[string]config.Release, selection releaseSelection) error {
	for _, release := range releases {
		if !selection.includes(release.Name) {
			continue
		}
		if release.IsRemoteChart() {
			if release.TargetRevision == "" {
				return fmt.Errorf("cluster %q baseline release %q uses remote chart without targetRevision", cluster, release.Name)
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
