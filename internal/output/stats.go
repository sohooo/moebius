package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

func chartChangeCounts(chart ChartReport) (added, removed, changed int) {
	for _, resource := range chart.Resources {
		switch resource.State {
		case "added":
			added++
		case "removed":
			removed++
		default:
			changed++
		}
	}
	return added, removed, changed
}

func cloneReports(reports []ClusterReport) []ClusterReport {
	out := make([]ClusterReport, len(reports))
	for i, report := range reports {
		out[i] = report
		out[i].Charts = append([]ChartReport(nil), report.Charts...)
		for j, chart := range out[i].Charts {
			out[i].Charts[j].Resources = append([]ResourceReport(nil), chart.Resources...)
		}
	}
	return out
}

func sortReportsForComment(reports []ClusterReport) {
	for i := range reports {
		sort.SliceStable(reports[i].Charts, func(a, b int) bool {
			left, right := reports[i].Charts[a], reports[i].Charts[b]
			if severity.Rank(chartSeverity(left)) != severity.Rank(chartSeverity(right)) {
				return severity.Rank(chartSeverity(left)) > severity.Rank(chartSeverity(right))
			}
			la, lr, lc := chartChangeCounts(left)
			ra, rr, rc := chartChangeCounts(right)
			if lr != rr {
				return lr > rr
			}
			if la != ra {
				return la > ra
			}
			lTotal := la + lr + lc
			rTotal := ra + rr + rc
			if lTotal != rTotal {
				return lTotal > rTotal
			}
			return left.Name < right.Name
		})
		for j := range reports[i].Charts {
			sort.SliceStable(reports[i].Charts[j].Resources, func(a, b int) bool {
				left, right := reports[i].Charts[j].Resources[a], reports[i].Charts[j].Resources[b]
				if validateStatusRank(left.Validation.Status) != validateStatusRank(right.Validation.Status) {
					return validateStatusRank(left.Validation.Status) > validateStatusRank(right.Validation.Status)
				}
				if severity.Rank(left.Assessment.Level) != severity.Rank(right.Assessment.Level) {
					return severity.Rank(left.Assessment.Level) > severity.Rank(right.Assessment.Level)
				}
				if stateWeight(left.State) != stateWeight(right.State) {
					return stateWeight(left.State) < stateWeight(right.State)
				}
				if left.Name != right.Name {
					return left.Name < right.Name
				}
				return left.Kind < right.Kind
			})
		}
	}
}

func stateWeight(state string) int {
	switch state {
	case "removed":
		return 0
	case "changed":
		return 1
	case "added":
		return 2
	default:
		return 3
	}
}

type reportStats struct {
	clusters             int
	charts               int
	resources            int
	added                int
	removed              int
	changed              int
	validationErrors     int
	validationWarnings   int
	unvalidatedResources int
	renderWarnings       int
	missingVersions      int
	otherRenderWarnings  int
	renderNotices        int
}

func collectReportStats(reports []ClusterReport) reportStats {
	stats := reportStats{clusters: len(reports)}
	for _, report := range reports {
		stats.charts += len(report.Charts)
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				stats.renderWarnings++
				if isMissingVersionRenderWarning(chart.RenderWarning) {
					stats.missingVersions++
				} else {
					stats.otherRenderWarnings++
				}
			}
			stats.renderNotices += len(chart.Warnings)
			added, removed, changed := chartChangeCounts(chart)
			stats.added += added
			stats.removed += removed
			stats.changed += changed
			stats.resources += added + removed + changed
			for _, resource := range chart.Resources {
				switch resource.Validation.Status {
				case validate.StatusError:
					stats.validationErrors++
				case validate.StatusWarning:
					stats.validationWarnings++
				}
				if resource.Validation.Coverage == validate.CoverageUnvalidated {
					stats.unvalidatedResources++
				}
			}
		}
	}
	return stats
}

func renderPartialAnalysisWarnings(b *strings.Builder, stats reportStats) {
	if stats.renderWarnings > 0 || stats.renderNotices > 0 {
		fmt.Fprintln(b, "> [!important]")
		fmt.Fprintln(b, "> Analysis is partial.")
		if stats.missingVersions > 0 {
			fmt.Fprintf(b, "> %d release(s) skipped because the requested chart version is unavailable.\n", stats.missingVersions)
			fmt.Fprintf(b, "**Missing chart versions:** %d skipped release(s)\n", stats.missingVersions)
		}
		if stats.otherRenderWarnings > 0 {
			fmt.Fprintf(b, "> %d release(s) skipped due to other render warnings.\n", stats.otherRenderWarnings)
			fmt.Fprintf(b, "**Other render warnings:** %d skipped release(s)\n", stats.otherRenderWarnings)
		}
		if stats.renderWarnings > 0 {
			fmt.Fprintln(b)
		}
		if stats.renderNotices > 0 {
			fmt.Fprintf(b, "> duplicate YAML keys accepted with last-wins behavior: %d override(s).\n", stats.renderNotices)
			fmt.Fprintf(b, "**Permissive YAML warnings:** %d duplicate-key override(s)\n\n", stats.renderNotices)
		} else {
			fmt.Fprintln(b)
		}
	}
}
