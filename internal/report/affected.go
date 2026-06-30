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

type affectedPlan struct {
	releaseSelection
	Reasons        map[string][]string
	FallbackReason string
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
	plan, err := planAffectedReleaseDetails(root, baselineRoot, layout, cluster, currentExists, baselineExists, changedPaths, baselineReleases, currentReleases)
	return plan.releaseSelection, err
}

func planAffectedReleaseDetails(root, baselineRoot string, layout config.LayoutConfig, cluster string, currentExists, baselineExists bool, changedPaths []string, baselineReleases, currentReleases map[string]config.Release) (affectedPlan, error) {
	names := map[string]struct{}{}
	for name := range baselineReleases {
		names[name] = struct{}{}
	}
	for name := range currentReleases {
		names[name] = struct{}{}
	}
	reasons := map[string][]string{}
	if currentExists != baselineExists {
		reason := "cluster_added"
		if baselineExists {
			reason = "cluster_removed"
		}
		for name := range names {
			reasons[name] = append(reasons[name], reason)
		}
		return affectedPlan{releaseSelection: allReleasesSelection(), Reasons: reasons, FallbackReason: reason}, nil
	}
	if configChanged(changedPaths) {
		for name := range names {
			reasons[name] = append(reasons[name], "config_changed_full_cluster_fallback")
		}
		return affectedPlan{releaseSelection: allReleasesSelection(), Reasons: reasons, FallbackReason: "config_changed_full_cluster_fallback"}, nil
	}

	affected := map[string]struct{}{}
	clusterChangedPaths := pathsForCluster(layout, cluster, changedPaths)
	overridePaths := map[string]struct{}{}

	for name := range names {
		baselineRelease, baselineOK := baselineReleases[name]
		currentRelease, currentOK := currentReleases[name]
		switch {
		case !baselineOK:
			affected[name] = struct{}{}
			reasons[name] = append(reasons[name], "release_added")
		case !currentOK:
			affected[name] = struct{}{}
			reasons[name] = append(reasons[name], "release_removed")
		case !reflect.DeepEqual(baselineRelease, currentRelease):
			affected[name] = struct{}{}
			reasons[name] = append(reasons[name], "release_attributes_changed")
		}

		baselineOverride, err := overrideFingerprintForRelease(baselineRoot, layout, cluster, baselineRelease, baselineOK)
		if err != nil {
			return affectedPlan{}, err
		}
		currentOverride, err := overrideFingerprintForRelease(root, layout, cluster, currentRelease, currentOK)
		if err != nil {
			return affectedPlan{}, err
		}
		if baselineOverride.Exists {
			overridePaths[baselineOverride.Path] = struct{}{}
		}
		if currentOverride.Exists {
			overridePaths[currentOverride.Path] = struct{}{}
		}
		if baselineOverride != currentOverride {
			affected[name] = struct{}{}
			reasons[name] = append(reasons[name], overrideChangeReason(baselineOverride, currentOverride))
		}

		if localChartChanged(changedPaths, baselineRelease, baselineOK) || localChartChanged(changedPaths, currentRelease, currentOK) {
			affected[name] = struct{}{}
			reasons[name] = append(reasons[name], "local_chart_changed")
		}
	}

	if hasUnmappedClusterChange(clusterChangedPaths, layout.Apps.Files, overridePaths) {
		for name := range names {
			reasons[name] = append(reasons[name], "unmapped_cluster_change_full_cluster_fallback")
		}
		return affectedPlan{releaseSelection: allReleasesSelection(), Reasons: normalizeReasonMap(names, reasons), FallbackReason: "unmapped_cluster_change_full_cluster_fallback"}, nil
	}
	return affectedPlan{releaseSelection: namedReleaseSelection(affected), Reasons: normalizeReasonMap(names, reasons)}, nil
}

func normalizeReasonMap(names map[string]struct{}, reasons map[string][]string) map[string][]string {
	for name := range names {
		if len(reasons[name]) == 0 {
			reasons[name] = []string{"not_affected"}
		}
		reasons[name] = uniqueStrings(reasons[name])
	}
	return reasons
}

func overrideChangeReason(baseline, current overrideFingerprint) string {
	switch {
	case !baseline.Exists && current.Exists:
		return "override_added"
	case baseline.Exists && !current.Exists:
		return "override_removed"
	case baseline.Path != current.Path:
		return "override_path_switched"
	default:
		return "override_changed"
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
