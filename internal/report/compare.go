package report

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/resources"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

func compareCluster(cluster, baselineOutput, currentOutput, diffOutput string, contextLines int, doValidate bool, baselineReleases map[string]config.Release, currentReleases map[string]config.Release) (output.ClusterReport, error) {
	report := output.ClusterReport{Name: cluster}

	chartNames, err := unionDirs(filepath.Join(baselineOutput, cluster), filepath.Join(currentOutput, cluster))
	if err != nil {
		return report, err
	}

	for _, chartName := range chartNames {
		baselineChartDir := filepath.Join(baselineOutput, cluster, chartName)
		currentChartDir := filepath.Join(currentOutput, cluster, chartName)
		namespace := firstNonEmpty(readFirstLine(filepath.Join(currentChartDir, "namespace.txt")), readFirstLine(filepath.Join(baselineChartDir, "namespace.txt")))
		renderWarning := joinNonEmpty(
			strings.TrimSpace(readFirstLine(filepath.Join(currentChartDir, renderWarningFilename))),
			strings.TrimSpace(readFirstLine(filepath.Join(baselineChartDir, renderWarningFilename))),
		)
		renderNotices := append(readLines(filepath.Join(currentChartDir, renderNoticeFilename)), readLines(filepath.Join(baselineChartDir, renderNoticeFilename))...)

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
		if len(resourceKeys) == 0 && renderWarning == "" && len(renderNotices) == 0 {
			continue
		}

		baselineRelease := baselineReleases[chartName]
		currentRelease := currentReleases[chartName]
		chartReport := output.ChartReport{
			Name:                   chartName,
			Namespace:              namespace,
			RenderWarning:          renderWarning,
			Warnings:               renderNotices,
			BaselineTargetRevision: baselineRelease.TargetRevision,
			CurrentTargetRevision:  currentRelease.TargetRevision,
			HasRemoteSource:        baselineRelease.IsRemoteChart() || currentRelease.IsRemoteChart(),
		}
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

		if len(chartReport.Resources) > 0 || chartReport.RenderWarning != "" || len(chartReport.Warnings) > 0 {
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

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(part))
	}
	return strings.Join(out, " | ")
}

func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
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
