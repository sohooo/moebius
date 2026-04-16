// Package output renders cluster reports for terminals, markdown, and MR notes.
package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

const StickyMarker = "<!-- mobius:mr-diff -->"

type ResourceReport struct {
	State      string
	Kind       string
	Name       string
	Namespace  string
	Result     diff.Result
	Semantic   string
	Assessment severity.Assessment
	Validation validate.Result
}

type ChartReport struct {
	Name          string
	Namespace     string
	Resources     []ResourceReport
	RenderWarning string
}

type ClusterReport struct {
	Name    string
	Charts  []ChartReport
	Added   int
	Removed int
	Changed int
}

type NoteMetadata struct {
	PipelineURL string
	JobURL      string
	CommitSHA   string
	BaseRef     string
	DiffMode    string
	GeneratedAt string
}

type NoteRenderOptions struct {
	Mode                 cli.CommentMode
	IncludeArtifactsHint bool
	Status               string
}

func PrintReports(w io.Writer, reports []ClusterReport, mode diff.Mode, format cli.OutputFormat) error {
	text, err := RenderReports(reports, mode, format)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, text)
	return err
}

func RenderReports(reports []ClusterReport, mode diff.Mode, format cli.OutputFormat) (string, error) {
	var b strings.Builder
	for i, report := range reports {
		var chunk string
		var err error
		switch format {
		case cli.OutputFormatMarkdown:
			chunk, err = renderClusterMarkdown(report, mode)
		default:
			chunk, err = renderClusterPlain(report, mode)
		}
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(chunk)
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func RenderCommentBody(reports []ClusterReport, mode diff.Mode, meta NoteMetadata) (string, error) {
	return RenderCommentBodyWithOptions(reports, mode, meta, NoteRenderOptions{
		Mode:   cli.CommentModeFull,
		Status: defaultStatus(reports),
	})
}

func RenderCommentBodyWithOptions(reports []ClusterReport, mode diff.Mode, meta NoteMetadata, opts NoteRenderOptions) (string, error) {
	var b strings.Builder
	b.WriteString("# møbius Diff Report\n\n")
	if opts.Status != "" {
		fmt.Fprintf(&b, "**Status:** %s\n\n", opts.Status)
	}
	if meta.PipelineURL != "" || meta.JobURL != "" || meta.CommitSHA != "" {
		var fields []string
		if meta.PipelineURL != "" {
			fields = append(fields, fmt.Sprintf("Pipeline: %s", meta.PipelineURL))
		}
		if meta.JobURL != "" {
			fields = append(fields, fmt.Sprintf("Job: %s", meta.JobURL))
		}
		if meta.CommitSHA != "" {
			fields = append(fields, fmt.Sprintf("Commit: `%s`", meta.CommitSHA))
		}
		if meta.BaseRef != "" {
			fields = append(fields, fmt.Sprintf("Base ref: `%s`", meta.BaseRef))
		}
		if meta.DiffMode != "" {
			fields = append(fields, fmt.Sprintf("Diff mode: `%s`", meta.DiffMode))
		}
		if meta.GeneratedAt != "" {
			fields = append(fields, fmt.Sprintf("Generated: `%s`", meta.GeneratedAt))
		}
		b.WriteString(strings.Join(fields, "  \n"))
		b.WriteString("\n\n")
	}

	if len(reports) == 0 {
		b.WriteString("_No effective changes._\n\n")
		b.WriteString(StickyMarker)
		return b.String(), nil
	}

	renderedReports := cloneReports(reports)
	sortReportsForComment(renderedReports)
	renderTopSummary(&b, renderedReports)
	b.WriteByte('\n')

	for i := range reports {
		chunk, err := renderClusterComment(renderedReports[i], mode, opts)
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(chunk, "\n"))
		b.WriteString("\n\n")
	}
	renderFooter(&b, opts)
	b.WriteString(StickyMarker)
	return b.String(), nil
}

func renderClusterPlain(report ClusterReport, mode diff.Mode) (string, error) {
	var b strings.Builder
	if len(report.Charts) == 0 {
		fmt.Fprintf(&b, "== Cluster: %s ==\nNo effective changes.\n", report.Name)
		return b.String(), nil
	}

	fmt.Fprintf(&b, "== Cluster: %s ==\n", report.Name)
	for _, chart := range report.Charts {
		fmt.Fprintf(&b, "-- Chart: %s (namespace: %s) --\n", chart.Name, emptyToNone(chart.Namespace))
		if chart.RenderWarning != "" {
			fmt.Fprintf(&b, "! render warning: %s\n\n", chart.RenderWarning)
			continue
		}
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "Resource: %s/%s (%s, severity: %s%s)\n", resource.Kind, resource.Name, resource.State, resource.Assessment.Level, validationSuffix(resource.Validation))
			if detail := validationCoverageLine(resource.Validation); detail != "" {
				fmt.Fprintf(&b, "= %s\n", detail)
			}
			for _, finding := range topValidationFindings(resource, 3) {
				fmt.Fprintf(&b, "! %s\n", finding)
			}
			for _, finding := range topFindings(resource, 3) {
				fmt.Fprintf(&b, "- %s\n", finding)
			}
			if len(resource.Assessment.Findings) > 0 || len(resource.Validation.Findings) > 0 {
				b.WriteByte('\n')
			}
			semanticConsole, err := diff.RenderSemanticConsole(resource.Result.Changes)
			if err != nil || strings.TrimSpace(semanticConsole) == "" {
				semanticConsole = resource.Semantic
			}
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(semanticConsole) != "" {
				b.WriteString(strings.TrimSpace(semanticConsole))
				b.WriteString("\n\n")
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(semanticConsole) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
				b.WriteString(strings.TrimSpace(resource.Result.RawDiff))
				b.WriteString("\n\n")
			}
		}
	}
	fmt.Fprintf(&b, "Summary for %s: added=%d removed=%d changed=%d\n", report.Name, report.Added, report.Removed, report.Changed)
	return strings.TrimRight(b.String(), "\n"), nil
}

func renderClusterMarkdown(report ClusterReport, mode diff.Mode) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Cluster `%s`\n\n", report.Name)
	fmt.Fprintln(&b, "| Added | Removed | Changed |")
	fmt.Fprintln(&b, "| ---: | ---: | ---: |")
	fmt.Fprintf(&b, "| %d | %d | %d |\n\n", report.Added, report.Removed, report.Changed)

	if len(report.Charts) == 0 {
		fmt.Fprintln(&b, "_No effective changes._")
		return strings.TrimRight(b.String(), "\n"), nil
	}

	for _, chart := range report.Charts {
		fmt.Fprintf(&b, "### Chart `%s`\n\n", chart.Name)
		fmt.Fprintf(&b, "- Namespace: `%s`\n\n", emptyToNone(chart.Namespace))
		if chart.RenderWarning != "" {
			fmt.Fprintf(&b, "> Render warning: %s\n\n", chart.RenderWarning)
			continue
		}
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "#### Resource `%s/%s` (%s, severity: %s%s)\n\n", resource.Kind, resource.Name, resource.State, resource.Assessment.Level, validationSuffix(resource.Validation))
			if detail := validationCoverageLine(resource.Validation); detail != "" {
				fmt.Fprintf(&b, "- validation coverage: %s\n\n", detail)
			}
			if findings := topValidationFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- validation: %s\n", finding)
				}
				fmt.Fprintln(&b)
			}
			if findings := topFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- %s\n", finding)
				}
				fmt.Fprintln(&b)
			}
			semanticMarkdown, err := diff.RenderSemanticMarkdown(resource.Result.Changes)
			if err != nil || strings.TrimSpace(semanticMarkdown) == "" {
				semanticMarkdown = resource.Semantic
			}
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(semanticMarkdown) != "" {
				fmt.Fprintln(&b, "```diff")
				fmt.Fprintln(&b, strings.TrimSpace(semanticMarkdown))
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b)
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(semanticMarkdown) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
				label := "Raw diff"
				if mode == diff.ModeRaw {
					label = "Diff"
				}
				fmt.Fprintf(&b, "**%s**\n\n", label)
				fmt.Fprintln(&b, "```diff")
				fmt.Fprintln(&b, strings.TrimSpace(resource.Result.RawDiff))
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func renderClusterComment(report ClusterReport, mode diff.Mode, opts NoteRenderOptions) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Cluster `%s`\n\n", report.Name)
	fmt.Fprintln(&b, "| Added | Removed | Changed |")
	fmt.Fprintln(&b, "| ---: | ---: | ---: |")
	fmt.Fprintf(&b, "| %d | %d | %d |\n\n", report.Added, report.Removed, report.Changed)

	if len(report.Charts) == 0 {
		fmt.Fprintln(&b, "_No effective changes._")
		return strings.TrimRight(b.String(), "\n"), nil
	}

	fmt.Fprintf(&b, "Charts with changes: %d\n\n", len(report.Charts))

	for _, chart := range report.Charts {
		added, removed, changed := chartChangeCounts(chart)
		fmt.Fprintf(&b, "<details>\n<summary>Chart `%s` · namespace `%s` · severity `%s` · added %d · removed %d · changed %d</summary>\n\n", chart.Name, emptyToNone(chart.Namespace), chartSeverity(chart), added, removed, changed)
		if chart.RenderWarning != "" {
			fmt.Fprintf(&b, "- Render warning: %s\n\n", chart.RenderWarning)
			fmt.Fprintln(&b, "</details>")
			fmt.Fprintln(&b)
			continue
		}
		fmt.Fprintf(&b, "- Kinds affected: %s\n", strings.Join(chartKinds(chart), ", "))
		if onlyValueTweaks(chart) {
			fmt.Fprintln(&b, "- Scope: value-level tweaks only")
		}
		fmt.Fprintf(&b, "- Severity summary: %s\n", formatSeveritySummary(chartSeverityCounts(chart)))
		errors, warnings, unvalidated := chartValidationCounts(chart)
		if errors > 0 || warnings > 0 || unvalidated > 0 {
			fmt.Fprintf(&b, "- Validation: %d errors, %d warnings, %d unvalidated\n", errors, warnings, unvalidated)
		}
		notables := collectNotableChanges(chart)
		if len(notables) > 0 {
			fmt.Fprintln(&b, "- Notable changes:")
			for _, notable := range notables {
				fmt.Fprintf(&b, "  - %s\n", notable)
			}
		}
		if opts.Mode == cli.CommentModeSummaryArtifacts || opts.IncludeArtifactsHint {
			fmt.Fprintln(&b, "- Full detailed report is available in pipeline artifacts.")
		}
		fmt.Fprintln(&b)
		if opts.Mode == cli.CommentModeSummary || opts.Mode == cli.CommentModeSummaryArtifacts {
			fmt.Fprintln(&b, "</details>")
			fmt.Fprintln(&b)
			continue
		}
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "#### Resource `%s/%s` (%s, severity: %s%s)\n\n", resource.Kind, resource.Name, resource.State, resource.Assessment.Level, validationSuffix(resource.Validation))
			if detail := validationCoverageLine(resource.Validation); detail != "" {
				fmt.Fprintf(&b, "- validation coverage: %s\n\n", detail)
			}
			if findings := topValidationFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- validation: %s\n", finding)
				}
				fmt.Fprintln(&b)
			}
			if findings := topFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- %s\n", finding)
				}
				fmt.Fprintln(&b)
			}
			semanticMarkdown, err := diff.RenderSemanticMarkdown(resource.Result.Changes)
			if err != nil || strings.TrimSpace(semanticMarkdown) == "" {
				semanticMarkdown = resource.Semantic
			}
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(semanticMarkdown) != "" {
				fmt.Fprintln(&b, "```diff")
				fmt.Fprintln(&b, strings.TrimSpace(semanticMarkdown))
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b)
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(semanticMarkdown) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
				label := "Raw diff"
				if mode == diff.ModeRaw {
					label = "Diff"
				}
				fmt.Fprintf(&b, "**%s**\n\n", label)
				fmt.Fprintln(&b, "```diff")
				fmt.Fprintln(&b, strings.TrimSpace(resource.Result.RawDiff))
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b)
			}
		}
		fmt.Fprintln(&b, "</details>")
		fmt.Fprintln(&b)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

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
				if left.Kind != right.Kind {
					return left.Kind < right.Kind
				}
				return left.Name < right.Name
			})
		}
	}
}

func stateWeight(state string) int {
	switch state {
	case "removed":
		return 0
	case "added":
		return 1
	default:
		return 2
	}
}

func renderTopSummary(b *strings.Builder, reports []ClusterReport) {
	type counts struct{ added, removed, changed int }
	totalClusters := len(reports)
	totalCharts := 0
	totalResources := 0
	c := counts{}
	severityCounts := map[severity.Level]int{}
	validationErrors := 0
	validationWarnings := 0
	unvalidatedResources := 0
	renderWarnings := 0
	highlights := collectReviewHighlights(reports, 5)

	for _, report := range reports {
		totalCharts += len(report.Charts)
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				renderWarnings++
			}
			added, removed, changed := chartChangeCounts(chart)
			c.added += added
			c.removed += removed
			c.changed += changed
			totalResources += added + removed + changed
			for _, resource := range chart.Resources {
				severityCounts[resource.Assessment.Level]++
				switch resource.Validation.Status {
				case validate.StatusError:
					validationErrors++
				case validate.StatusWarning:
					validationWarnings++
				}
				if resource.Validation.Coverage == validate.CoverageUnvalidated {
					unvalidatedResources++
				}
			}
		}
	}

	fmt.Fprintln(b, "## Review Summary")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "| Clusters | Charts | Resources | Added | Removed | Changed |")
	fmt.Fprintln(b, "| ---: | ---: | ---: | ---: | ---: | ---: |")
	fmt.Fprintf(b, "| %d | %d | %d | %d | %d | %d |\n\n", totalClusters, totalCharts, totalResources, c.added, c.removed, c.changed)
	if summary := formatSeveritySummary(severityCounts); summary != "" {
		fmt.Fprintf(b, "**Severity:** %s\n\n", summary)
	}
	fmt.Fprintf(b, "**Validation:** %d errors, %d warnings, %d unvalidated\n\n", validationErrors, validationWarnings, unvalidatedResources)
	if renderWarnings > 0 {
		fmt.Fprintf(b, "**Render warnings:** %d skipped release(s)\n\n", renderWarnings)
	}
	if len(highlights) > 0 {
		fmt.Fprintln(b, "**Highlights**")
		fmt.Fprintln(b)
		for _, highlight := range highlights {
			fmt.Fprintf(b, "- %s\n", highlight)
		}
	}
}

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

func collectNotableChanges(chart ChartReport) []string {
	if chart.RenderWarning != "" {
		return []string{fmt.Sprintf("[render-warning] %s", chart.RenderWarning)}
	}
	var out []string
	for _, resource := range chart.Resources {
		for _, line := range topValidationFindings(resource, 2) {
			out = append(out, fmt.Sprintf("`%s/%s` [validation:%s]: %s", resource.Kind, resource.Name, resource.Validation.Status, line))
			if len(out) >= 5 {
				return out
			}
		}
		for _, line := range topFindings(resource, 3) {
			out = append(out, fmt.Sprintf("`%s/%s` [%s]: %s", resource.Kind, resource.Name, resource.Assessment.Level, line))
			if len(out) >= 5 {
				return out
			}
		}
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

func collectReviewHighlights(reports []ClusterReport, limit int) []string {
	type highlight struct {
		validation validate.Status
		level      severity.Level
		text       string
	}
	var items []highlight
	for _, report := range reports {
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				items = append(items, highlight{
					validation: validate.StatusWarning,
					level:      severity.LevelInfo,
					text:       fmt.Sprintf("Cluster `%s` · chart `%s` [render-warning]: %s", report.Name, chart.Name, chart.RenderWarning),
				})
			}
			for _, resource := range chart.Resources {
				for _, finding := range topValidationFindings(resource, 2) {
					items = append(items, highlight{
						validation: resource.Validation.Status,
						level:      resource.Assessment.Level,
						text:       fmt.Sprintf("Cluster `%s` · `%s/%s` [validation:%s]: %s", report.Name, resource.Kind, resource.Name, resource.Validation.Status, finding),
					})
				}
				for _, finding := range topFindings(resource, 2) {
					items = append(items, highlight{
						validation: resource.Validation.Status,
						level:      resource.Assessment.Level,
						text:       fmt.Sprintf("Cluster `%s` · `%s/%s` [%s]: %s", report.Name, resource.Kind, resource.Name, resource.Assessment.Level, finding),
					})
				}
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if validateStatusRank(items[i].validation) != validateStatusRank(items[j].validation) {
			return validateStatusRank(items[i].validation) > validateStatusRank(items[j].validation)
		}
		if severity.Rank(items[i].level) != severity.Rank(items[j].level) {
			return severity.Rank(items[i].level) > severity.Rank(items[j].level)
		}
		return items[i].text < items[j].text
	})
	out := make([]string, 0, limit)
	for _, item := range items {
		out = append(out, item.text)
		if len(out) >= limit {
			break
		}
	}
	return dedupeStrings(out)
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

func renderFooter(b *strings.Builder, opts NoteRenderOptions) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteString("\n")
	}
	fmt.Fprintln(b, "---")
	fmt.Fprintln(b)
	if opts.Mode == cli.CommentModeSummaryArtifacts {
		fmt.Fprintln(b, "_Compact summary mode. Full details are available in pipeline artifacts._")
	} else if opts.Mode == cli.CommentModeSummary {
		fmt.Fprintln(b, "_Summary mode. Full resource diffs are omitted from this MR note._")
	} else {
		fmt.Fprintln(b, "_Report compares merge-base and current MR state._")
	}
	fmt.Fprintln(b)
}

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}

func defaultStatus(reports []ClusterReport) string {
	for _, report := range reports {
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				return "warnings detected"
			}
		}
	}
	if len(reports) == 0 {
		return "no effective changes"
	}
	return "changes detected"
}
