package report

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/config"
)

type releaseSelection struct {
	all   bool
	names map[string]struct{}
}

func allReleasesSelection() releaseSelection {
	return releaseSelection{all: true}
}

func namedReleaseSelection(names map[string]struct{}) releaseSelection {
	return releaseSelection{names: names}
}

func (s releaseSelection) includes(name string) bool {
	if s.all {
		return true
	}
	_, ok := s.names[name]
	return ok
}

func (s releaseSelection) empty() bool {
	return !s.all && len(s.names) == 0
}

type overrideFingerprint struct {
	Path     string
	Exists   bool
	Contents string
}

func planAffectedReleases(root, baselineRoot string, layout config.LayoutConfig, cluster string, currentExists, baselineExists bool, changedPaths []string, baselineReleases, currentReleases map[string]config.Release) (releaseSelection, error) {
	if currentExists != baselineExists {
		return allReleasesSelection(), nil
	}
	if configChanged(changedPaths) {
		return allReleasesSelection(), nil
	}

	names := map[string]struct{}{}
	for name := range baselineReleases {
		names[name] = struct{}{}
	}
	for name := range currentReleases {
		names[name] = struct{}{}
	}

	affected := map[string]struct{}{}
	clusterChangedPaths := pathsForCluster(layout, cluster, changedPaths)
	overridePaths := map[string]struct{}{}

	for name := range names {
		baselineRelease, baselineOK := baselineReleases[name]
		currentRelease, currentOK := currentReleases[name]
		if !baselineOK || !currentOK || !reflect.DeepEqual(baselineRelease, currentRelease) {
			affected[name] = struct{}{}
		}

		baselineOverride, err := overrideFingerprintForRelease(baselineRoot, layout, cluster, baselineRelease, baselineOK)
		if err != nil {
			return releaseSelection{}, err
		}
		currentOverride, err := overrideFingerprintForRelease(root, layout, cluster, currentRelease, currentOK)
		if err != nil {
			return releaseSelection{}, err
		}
		if baselineOverride.Exists {
			overridePaths[baselineOverride.Path] = struct{}{}
		}
		if currentOverride.Exists {
			overridePaths[currentOverride.Path] = struct{}{}
		}
		if baselineOverride != currentOverride {
			affected[name] = struct{}{}
		}

		if localChartChanged(changedPaths, baselineRelease, baselineOK) || localChartChanged(changedPaths, currentRelease, currentOK) {
			affected[name] = struct{}{}
		}
	}

	if hasUnmappedClusterChange(clusterChangedPaths, layout.Apps.Files, overridePaths) {
		return allReleasesSelection(), nil
	}
	return namedReleaseSelection(affected), nil
}

func configChanged(changedPaths []string) bool {
	for _, path := range changedPaths {
		if filepath.ToSlash(path) == "config.yaml" {
			return true
		}
	}
	return false
}

func pathsForCluster(layout config.LayoutConfig, cluster string, changedPaths []string) []string {
	prefix := strings.TrimSuffix(filepath.ToSlash(filepath.Join(layout.ClustersDir, cluster)), "/") + "/"
	var out []string
	for _, path := range changedPaths {
		path = filepath.ToSlash(path)
		if strings.HasPrefix(path, prefix) {
			out = append(out, strings.TrimPrefix(path, prefix))
		}
	}
	sort.Strings(out)
	return out
}

func overrideFingerprintForRelease(root string, layout config.LayoutConfig, cluster string, release config.Release, ok bool) (overrideFingerprint, error) {
	if !ok {
		return overrideFingerprint{}, nil
	}
	path := config.ResolveOverridePath(root, layout, cluster, release)
	clusterDir := config.ClusterDir(root, layout, cluster)
	rel, err := filepath.Rel(clusterDir, path)
	if err != nil {
		return overrideFingerprint{}, err
	}
	fp := overrideFingerprint{Path: filepath.ToSlash(rel)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fp, nil
		}
		return overrideFingerprint{}, err
	}
	fp.Exists = true
	fp.Contents = string(data)
	return fp, nil
}

func localChartChanged(changedPaths []string, release config.Release, ok bool) bool {
	if !ok || release.IsRemoteChart() || release.Chart == "" {
		return false
	}
	chartPath := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(release.Chart)), "/")
	if chartPath == "." || strings.HasPrefix(chartPath, "../") || filepath.IsAbs(release.Chart) {
		return false
	}
	for _, changedPath := range changedPaths {
		changedPath = filepath.ToSlash(changedPath)
		if changedPath == chartPath || strings.HasPrefix(changedPath, chartPath+"/") {
			return true
		}
	}
	return false
}

func hasUnmappedClusterChange(clusterPaths []string, appsFiles []string, overridePaths map[string]struct{}) bool {
	appsFileSet := map[string]struct{}{}
	for _, appsFile := range appsFiles {
		appsFileSet[filepath.ToSlash(appsFile)] = struct{}{}
	}
	for _, path := range clusterPaths {
		path = filepath.ToSlash(path)
		if _, ok := appsFileSet[path]; ok {
			continue
		}
		if _, ok := overridePaths[path]; ok {
			continue
		}
		return true
	}
	return false
}
