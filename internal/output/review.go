package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

const maxAttentionItems = 5

type attentionItem struct {
	Cluster          string
	Chart            string
	Kind             string
	Name             string
	Namespace        string
	Level            severity.Level
	ValidationStatus validate.Status
	Surface          string
	Reason           string
	IsRenderWarning  bool
}

type reviewSignals struct {
	HasCriticalHigh   bool
	HasSecurity       bool
	HasNetworking     bool
	HasStorage        bool
	HasDatabase       bool
	HasValidationGaps bool
}

func renderReviewFocus(b *strings.Builder, reports []ClusterReport, target renderTarget) {
	stats := collectReportStats(reports)
	severityCounts := map[severity.Level]int{}
	surfaceSet := map[string]struct{}{}
	for _, report := range reports {
		for level, count := range clusterSeverityCounts(report) {
			severityCounts[level] += count
		}
		for _, chart := range report.Charts {
			for _, resource := range chart.Resources {
				for _, surface := range resourceSurfaces(resource) {
					surfaceSet[surface] = struct{}{}
				}
			}
		}
	}

	fmt.Fprintln(b, "**Review Focus**")
	fmt.Fprintln(b)
	if summary := formatSeveritySummaryIcons(severityCounts); summary != "" {
		fmt.Fprintf(b, "- Severity: %s\n", summary)
	}
	if surfaces := formatSurfaceSet(surfaceSet); surfaces != "" {
		fmt.Fprintf(b, "- Surfaces: %s\n", surfaces)
	}
	fmt.Fprintf(b, "- Changes: %s\n", formatChangeMix(stats.added, stats.removed, stats.changed))
	fmt.Fprintf(b, "- Validation: %d errors · %d warnings · %d unvalidated\n", stats.validationErrors, stats.validationWarnings, stats.unvalidatedResources)
	if stats.renderWarnings > 0 || stats.renderNotices > 0 {
		fmt.Fprintf(b, "- Analysis gaps: %d render warnings · %d render notices\n", stats.renderWarnings, stats.renderNotices)
	}

	items := collectAttentionItems(reports)
	if len(items) > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b, "**Attention Required**")
		for _, item := range limitAttentionItems(items, maxAttentionItems) {
			fmt.Fprintf(b, "- %s\n", formatAttentionItem(item, target))
		}
		if len(items) > maxAttentionItems {
			fmt.Fprintf(b, "- %d additional attention item(s) in chart details\n", len(items)-maxAttentionItems)
		}
	}
	fmt.Fprintln(b)
}

func renderReviewChecklist(b *strings.Builder, reports []ClusterReport) {
	signals := collectReviewSignals(reports)
	items := reviewChecklistItems(signals)
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(b, "**Review Checklist**")
	for _, item := range items {
		fmt.Fprintf(b, "- [ ] %s\n", item)
	}
	fmt.Fprintln(b)
}

func collectAttentionItems(reports []ClusterReport) []attentionItem {
	var items []attentionItem
	for _, report := range reports {
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				items = append(items, attentionItem{
					Cluster:         report.Name,
					Chart:           chart.Name,
					Level:           severity.LevelInfo,
					Surface:         "render",
					Reason:          "render warning: " + chartRenderWarningSummary(chart.RenderWarning),
					IsRenderWarning: true,
				})
				continue
			}
			for _, resource := range chart.Resources {
				reason := attentionReason(resource)
				if reason == "" {
					continue
				}
				items = append(items, attentionItem{
					Cluster:          report.Name,
					Chart:            chart.Name,
					Kind:             resource.Kind,
					Name:             resource.Name,
					Namespace:        resource.Namespace,
					Level:            resource.Assessment.Level,
					ValidationStatus: resource.Validation.Status,
					Surface:          primarySurface(resource),
					Reason:           reason,
				})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if severity.Rank(left.Level) != severity.Rank(right.Level) {
			return severity.Rank(left.Level) > severity.Rank(right.Level)
		}
		if validateStatusRank(left.ValidationStatus) != validateStatusRank(right.ValidationStatus) {
			return validateStatusRank(left.ValidationStatus) > validateStatusRank(right.ValidationStatus)
		}
		if left.Surface != right.Surface {
			return left.Surface < right.Surface
		}
		if left.Cluster != right.Cluster {
			return left.Cluster < right.Cluster
		}
		if left.Chart != right.Chart {
			return left.Chart < right.Chart
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Kind < right.Kind
	})
	return items
}

func attentionReason(resource ResourceReport) string {
	if resource.Validation.Status == validate.StatusError || resource.Validation.Status == validate.StatusWarning {
		return primaryResourceHighlight(resource)
	}
	if resource.Validation.Coverage == validate.CoverageUnvalidated && severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) {
		return "unvalidated high-severity resource: " + primaryResourceHighlight(resource)
	}
	if severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) {
		return primaryResourceHighlight(resource)
	}
	if resource.Namespace == "" && resource.State == "removed" {
		return "cluster-scoped resource removed"
	}
	for _, finding := range resource.Assessment.Findings {
		reason := strings.ToLower(finding.Reason)
		switch {
		case strings.Contains(reason, "rbac"):
			return finding.Reason
		case strings.Contains(reason, "webhook"):
			return finding.Reason
		case strings.Contains(reason, "storage"):
			return finding.Reason
		case strings.Contains(reason, "loadbalancer") || strings.Contains(reason, "ingress"):
			return finding.Reason
		}
	}
	return ""
}

func formatAttentionItem(item attentionItem, target renderTarget) string {
	if item.IsRenderWarning {
		return fmt.Sprintf("%s `%s` · [`%s`](#%s) · %s · %s", severityIcon(item.Level), item.Cluster, item.Chart, chartLinkAnchor(item.Cluster, item.Chart, target), item.Surface, item.Reason)
	}
	label := fmt.Sprintf("`%s/%s`", item.Kind, item.Name)
	resource := ResourceReport{Namespace: item.Namespace, Kind: item.Kind, Name: item.Name}
	link := fmt.Sprintf("[%s](#%s)", label, resourceLinkAnchor(item.Cluster, item.Chart, resource, target))
	return fmt.Sprintf("%s `%s` · `%s` · %s · %s · %s", severityIcon(item.Level), item.Cluster, item.Chart, link, emptyToNone(item.Surface), item.Reason)
}

func limitAttentionItems(items []attentionItem, limit int) []attentionItem {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func renderChartReviewHints(b *strings.Builder, chart ChartReport) {
	hints := chartReviewHints(chart)
	if len(hints) == 0 {
		return
	}
	fmt.Fprintln(b, "**Review Hints**")
	for _, hint := range hints {
		fmt.Fprintf(b, "- %s\n", hint)
	}
	fmt.Fprintln(b)
}

func chartReviewHints(chart ChartReport) []string {
	if chart.RenderWarning != "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, resource := range chart.Resources {
		for _, hint := range resourceReviewHints(resource) {
			set[hint] = struct{}{}
		}
	}
	order := []string{
		"Check whether new permissions are intentionally scoped.",
		"Check webhook failure policy, service reachability, and admission impact.",
		"Check external reachability, hostnames, and TLS changes.",
		"Check persistence, reclaim policy, backup, and migration impact.",
		"Check database availability, backup, and migration impact.",
		"Check image source, tag, and rollout expectations.",
		"Manually review resources without schema coverage.",
	}
	var hints []string
	for _, hint := range order {
		if _, ok := set[hint]; ok {
			hints = append(hints, hint)
		}
	}
	return hints
}

func resourceReviewHints(resource ResourceReport) []string {
	var hints []string
	if resource.Validation.Coverage == validate.CoverageUnvalidated && severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) {
		hints = append(hints, "Manually review resources without schema coverage.")
	}
	if resource.Kind == "ClusterRole" || resource.Kind == "ClusterRoleBinding" || resource.Kind == "Role" || resource.Kind == "RoleBinding" {
		hints = append(hints, "Check whether new permissions are intentionally scoped.")
	}
	if resource.Kind == "MutatingWebhookConfiguration" || resource.Kind == "ValidatingWebhookConfiguration" {
		hints = append(hints, "Check webhook failure policy, service reachability, and admission impact.")
	}
	for _, finding := range resource.Assessment.Findings {
		reason := strings.ToLower(finding.Reason)
		category := finding.Category
		switch {
		case strings.Contains(reason, "rbac"):
			hints = append(hints, "Check whether new permissions are intentionally scoped.")
		case strings.Contains(reason, "webhook"):
			hints = append(hints, "Check webhook failure policy, service reachability, and admission impact.")
		case strings.Contains(reason, "ingress") || strings.Contains(reason, "loadbalancer") || category == "network":
			hints = append(hints, "Check external reachability, hostnames, and TLS changes.")
		case strings.Contains(reason, "storage") || category == "storage":
			hints = append(hints, "Check persistence, reclaim policy, backup, and migration impact.")
		case strings.Contains(reason, "cloudnativepg") || category == "database":
			hints = append(hints, "Check database availability, backup, and migration impact.")
		case strings.Contains(reason, "image changed") || strings.Contains(reason, "image registry changed"):
			hints = append(hints, "Check image source, tag, and rollout expectations.")
		}
	}
	return dedupeStrings(hints)
}

func validationGapLine(chart ChartReport) string {
	if chart.RenderWarning != "" {
		return ""
	}
	var total, criticalHigh int
	for _, resource := range chart.Resources {
		if resource.Validation.Coverage != validate.CoverageUnvalidated {
			continue
		}
		total++
		if severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) {
			criticalHigh++
		}
	}
	if total == 0 {
		return ""
	}
	if criticalHigh > 0 {
		return fmt.Sprintf("%d resource(s) without schema coverage; %d critical/high need manual review", total, criticalHigh)
	}
	return fmt.Sprintf("%d resource(s) without schema coverage", total)
}

func collectReviewSignals(reports []ClusterReport) reviewSignals {
	var signals reviewSignals
	for _, report := range reports {
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				signals.HasValidationGaps = true
			}
			for _, resource := range chart.Resources {
				if severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) {
					signals.HasCriticalHigh = true
				}
				if resource.Validation.Coverage == validate.CoverageUnvalidated || resource.Validation.Status == validate.StatusError || resource.Validation.Status == validate.StatusWarning {
					signals.HasValidationGaps = true
				}
				for _, surface := range resourceSurfaces(resource) {
					switch surface {
					case "security":
						signals.HasSecurity = true
					case "networking":
						signals.HasNetworking = true
					case "storage":
						signals.HasStorage = true
					case "database":
						signals.HasDatabase = true
					}
				}
			}
		}
	}
	return signals
}

func reviewChecklistItems(signals reviewSignals) []string {
	var items []string
	if signals.HasCriticalHigh || signals.HasSecurity {
		items = append(items, "Critical/high security changes reviewed")
	}
	if signals.HasValidationGaps {
		items = append(items, "Unvalidated resources manually checked")
	}
	if signals.HasNetworking {
		items = append(items, "External networking changes confirmed")
	}
	if signals.HasStorage || signals.HasDatabase {
		items = append(items, "Storage or database changes approved")
	}
	return items
}

func primarySurface(resource ResourceReport) string {
	surfaces := resourceSurfaces(resource)
	if len(surfaces) == 0 {
		return ""
	}
	return surfaces[0]
}

func formatSurfaceSet(set map[string]struct{}) string {
	if len(set) == 0 {
		return ""
	}
	var parts []string
	for _, surface := range surfaceOrder {
		if _, ok := set[surface]; ok {
			parts = append(parts, surface)
		}
	}
	return strings.Join(parts, " · ")
}
