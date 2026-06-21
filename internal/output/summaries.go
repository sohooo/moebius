package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

func chartKinds(chart ChartReport) []string {
	if chart.RenderWarning != "" {
		return []string{"<unavailable>"}
	}
	set := map[string]struct{}{}
	for _, resource := range chart.Resources {
		set[resource.Kind] = struct{}{}
	}
	kinds := make([]string, 0, len(set))
	for kind := range set {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func onlyValueTweaks(chart ChartReport) bool {
	if chart.RenderWarning != "" {
		return false
	}
	for _, resource := range chart.Resources {
		if resource.State != "changed" {
			return false
		}
	}
	return true
}

func collectChartResourceChanges(cluster string, chart ChartReport, target renderTarget, linkResources bool) []string {
	if chart.RenderWarning != "" {
		return nil
	}
	var out []string
	for _, resource := range chart.Resources {
		line := primaryResourceHighlight(resource)
		if line == "" {
			line = resource.State
		}
		label := fmt.Sprintf("`%s/%s`", resource.Kind, resource.Name)
		if linkResources {
			label = fmt.Sprintf("[%s](#%s)", label, resourceLinkAnchor(cluster, chart.Name, resource, target))
		}
		out = append(out, fmt.Sprintf("%s %s · %s", severityIcon(resource.Assessment.Level), label, line))
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func topFindings(resource ResourceReport, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, finding := range resource.Assessment.Findings {
		out = append(out, finding.Reason)
		if len(out) >= limit {
			break
		}
	}
	return dedupeStrings(out)
}

func topValidationFindings(resource ResourceReport, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, finding := range resource.Validation.Findings {
		line := finding.Message
		if finding.Path != "" {
			line = fmt.Sprintf("%s (%s)", line, finding.Path)
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return dedupeStrings(out)
}

func chartSeverity(chart ChartReport) severity.Level {
	if chart.RenderWarning != "" {
		return severity.LevelInfo
	}
	level := severity.LevelInfo
	for _, resource := range chart.Resources {
		if severity.Rank(resource.Assessment.Level) > severity.Rank(level) {
			level = resource.Assessment.Level
		}
	}
	return level
}

func chartSeverityCounts(chart ChartReport) map[severity.Level]int {
	if chart.RenderWarning != "" {
		return map[severity.Level]int{}
	}
	counts := map[severity.Level]int{}
	for _, resource := range chart.Resources {
		counts[resource.Assessment.Level]++
	}
	return counts
}

func clusterSeverityCounts(report ClusterReport) map[severity.Level]int {
	counts := map[severity.Level]int{}
	for _, chart := range report.Charts {
		for level, count := range chartSeverityCounts(chart) {
			counts[level] += count
		}
	}
	return counts
}

func formatSeveritySummary(counts map[severity.Level]int) string {
	order := []severity.Level{
		severity.LevelCritical,
		severity.LevelHigh,
		severity.LevelMedium,
		severity.LevelLow,
		severity.LevelInfo,
	}
	var parts []string
	for _, level := range order {
		if counts[level] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", level, counts[level]))
	}
	return strings.Join(parts, ", ")
}

func chartValidationCounts(chart ChartReport) (errors int, warnings int, unvalidated int) {
	if chart.RenderWarning != "" {
		return 0, 0, 0
	}
	for _, resource := range chart.Resources {
		switch resource.Validation.Status {
		case validate.StatusError:
			errors++
		case validate.StatusWarning:
			warnings++
		}
		if resource.Validation.Coverage == validate.CoverageUnvalidated {
			unvalidated++
		}
	}
	return errors, warnings, unvalidated
}

func validationSuffix(result validate.Result) string {
	if result.Status == "" || result.Status == validate.StatusValid {
		return ""
	}
	return fmt.Sprintf(", validation: %s", result.Status)
}

func validationCoverageLine(result validate.Result) string {
	switch result.Coverage {
	case validate.CoverageValidated:
		if result.SchemaSource == validate.SchemaSourceNone || result.SchemaSource == "" {
			return "validated"
		}
		return fmt.Sprintf("validated via %s", result.SchemaSource)
	case validate.CoverageUnvalidated:
		return "unvalidated (no schema available)"
	default:
		return ""
	}
}

func resourceMetadataLine(cluster, chart string, resource ResourceReport, target renderTarget) string {
	parts := []string{
		resource.State,
		fmt.Sprintf("severity %s", severityBadge(resource.Assessment.Level)),
	}
	if detail := validationCoverageLine(resource.Validation); detail != "" {
		parts = append(parts, "validation: "+detail)
	} else if resource.Validation.Status != "" && resource.Validation.Status != validate.StatusValid {
		parts = append(parts, "validation: "+string(resource.Validation.Status))
	}
	parts = append(parts, fmt.Sprintf("[up](#%s)", chartLinkAnchor(cluster, chart, target)))
	return strings.Join(parts, " · ")
}

func validateStatusRank(status validate.Status) int {
	switch status {
	case validate.StatusError:
		return 3
	case validate.StatusWarning:
		return 2
	default:
		return 1
	}
}

func isMissingVersionRenderWarning(warning string) bool {
	_, ok := missingVersionFromRenderWarning(warning)
	return ok
}

func missingVersionFromRenderWarning(warning string) (string, bool) {
	const marker = `requested chart version "`
	start := strings.Index(warning, marker)
	if start == -1 {
		return "", false
	}
	start += len(marker)
	end := strings.Index(warning[start:], `"`)
	if end == -1 {
		return "", false
	}
	version := strings.TrimSpace(warning[start : start+end])
	if version == "" {
		return "", false
	}
	return version, true
}

func chartRenderWarningSummary(warning string) string {
	if version, ok := missingVersionFromRenderWarning(warning); ok {
		return fmt.Sprintf("requested version %s unavailable", version)
	}
	return warning
}

func chartVersionSuffix(chart ChartReport) string {
	if versionChange := chartVersionChange(chart); versionChange != "" {
		return " · version " + versionChange
	}
	return ""
}

func chartVersionChange(chart ChartReport) string {
	if !chart.HasRemoteSource {
		return ""
	}
	if chart.BaselineTargetRevision == "" || chart.CurrentTargetRevision == "" || chart.BaselineTargetRevision == chart.CurrentTargetRevision {
		return ""
	}
	return fmt.Sprintf("%s → %s", chart.BaselineTargetRevision, chart.CurrentTargetRevision)
}

func renderChartSignalTable(b *strings.Builder, chart ChartReport, added, removed, changed int) {
	fmt.Fprintln(b, "| Signal | Details |")
	fmt.Fprintln(b, "| --- | --- |")
	fmt.Fprintf(b, "| **Summary** | %s |\n", escapeTable(chartSummaryDetails(chart, added, removed, changed)))
	if kinds := formatChartKinds(chart); kinds != "" {
		fmt.Fprintf(b, "| **Kinds** | %s |\n", escapeTable(kinds))
	}
	if chart.RenderWarning == "" {
		fmt.Fprintf(b, "| **Change mix** | %s |\n", escapeTable(formatChangeMix(added, removed, changed)))
		if surfaces := formatChartSurfaces(chart); surfaces != "" {
			fmt.Fprintf(b, "| **Surface** | %s |\n", escapeTable(surfaces))
		}
	}
	if onlyValueTweaks(chart) {
		fmt.Fprintln(b, "| **Scope** | value-level tweaks only |")
	}
	if summary := formatSeveritySummaryWithBadges(chartSeverityCounts(chart)); summary != "" {
		fmt.Fprintf(b, "| **Severity** | %s |\n", escapeTable(summary))
	}
	errors, warnings, unvalidated := chartValidationCounts(chart)
	if errors > 0 || warnings > 0 || unvalidated > 0 {
		fmt.Fprintf(b, "| **Validation** | %d errors · %d warnings · %d unvalidated |\n", errors, warnings, unvalidated)
	}
	if gap := validationGapLine(chart); gap != "" {
		fmt.Fprintf(b, "| **Validation gaps** | %s |\n", escapeTable(gap))
	}
	fmt.Fprintln(b)
}

func chartSummaryLine(chart ChartReport, added, removed, changed int) string {
	parts := []string{fmt.Sprintf("Chart `%s`", chart.Name)}
	if versionChange := chartVersionChange(chart); versionChange != "" {
		parts = append(parts, "version "+versionChange)
	}
	parts = append(parts,
		fmt.Sprintf("namespace `%s`", emptyToNone(chart.Namespace)),
		fmt.Sprintf("severity %s", severityIcon(chartSeverity(chart))),
		formatChangeMix(added, removed, changed),
	)
	return strings.Join(parts, " · ")
}

func chartSummaryDetails(chart ChartReport, added, removed, changed int) string {
	var parts []string
	if versionChange := chartVersionChange(chart); versionChange != "" {
		parts = append(parts, "version "+versionChange)
	}
	total := added + removed + changed
	if chart.RenderWarning != "" {
		if version, ok := missingVersionFromRenderWarning(chart.RenderWarning); ok {
			parts = append(parts, fmt.Sprintf("requested version %s unavailable", version))
		} else {
			parts = append(parts, "render skipped")
		}
	} else if total > 0 {
		if total == 1 {
			parts = append(parts, "1 resource affected")
		} else {
			parts = append(parts, fmt.Sprintf("%d resources affected", total))
		}
	}
	parts = append(parts, "highest severity "+severityBadge(chartSeverity(chart)))
	if chart.RenderWarning != "" || len(chart.Warnings) > 0 {
		parts = append(parts, "analysis partial")
	}
	return strings.Join(parts, " · ")
}

func formatChartKinds(chart ChartReport) string {
	kinds := chartKinds(chart)
	if len(kinds) == 0 {
		return ""
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, fmt.Sprintf("`%s`", kind))
	}
	return strings.Join(out, ", ")
}

func formatSeveritySummaryWithBadges(counts map[severity.Level]int) string {
	order := []severity.Level{
		severity.LevelCritical,
		severity.LevelHigh,
		severity.LevelMedium,
		severity.LevelLow,
		severity.LevelInfo,
	}
	var parts []string
	for _, level := range order {
		if counts[level] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", severityBadge(level), counts[level]))
	}
	return strings.Join(parts, " · ")
}

func formatSeveritySummaryIcons(counts map[severity.Level]int) string {
	order := []severity.Level{
		severity.LevelCritical,
		severity.LevelHigh,
		severity.LevelMedium,
		severity.LevelLow,
		severity.LevelInfo,
	}
	var parts []string
	for _, level := range order {
		if counts[level] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", severityIcon(level), counts[level]))
	}
	return strings.Join(parts, " · ")
}

func formatChangeFingerprint(added, removed, changed int, severityCounts map[severity.Level]int, schemaGaps int) string {
	if added == 0 && removed == 0 && changed == 0 && len(severityCounts) == 0 && schemaGaps == 0 {
		return ""
	}
	parts := []string{formatChangeMix(added, removed, changed)}
	if summary := formatSeveritySummaryWithBadges(severityCounts); summary != "" {
		parts = append(parts, summary)
	}
	if schemaGaps > 0 {
		parts = append(parts, fmt.Sprintf("schema gaps %d", schemaGaps))
	}
	return strings.Join(parts, " · ")
}

func formatChangeMix(added, removed, changed int) string {
	return fmt.Sprintf("+%d · -%d · ~%d", added, removed, changed)
}

func severityIcon(level severity.Level) string {
	return strings.Fields(severityBadge(level))[0]
}

func severityBadge(level severity.Level) string {
	switch level {
	case severity.LevelCritical:
		return "🔴 critical"
	case severity.LevelHigh:
		return "🟠 high"
	case severity.LevelMedium:
		return "🟡 medium"
	case severity.LevelLow:
		return "🟢 low"
	default:
		return "🔵 info"
	}
}

func escapeTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func primaryResourceHighlight(resource ResourceReport) string {
	if len(resource.Validation.Findings) > 0 && resource.Validation.Status != validate.StatusValid {
		line := resource.Validation.Findings[0].Message
		if resource.Validation.Findings[0].Path != "" {
			line = fmt.Sprintf("%s (%s)", line, resource.Validation.Findings[0].Path)
		}
		return fmt.Sprintf("validation %s: %s", resource.Validation.Status, line)
	}
	if len(resource.Assessment.Findings) > 0 {
		return resource.Assessment.Findings[0].Reason
	}
	return ""
}

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}
