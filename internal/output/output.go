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

type renderTarget int

const (
	renderTargetNote renderTarget = iota
	renderTargetDescription
)

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
	Name                   string
	Namespace              string
	Resources              []ResourceReport
	RenderWarning          string
	Warnings               []string
	BaselineTargetRevision string
	CurrentTargetRevision  string
	HasRemoteSource        bool
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
	target               renderTarget
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
	opts.target = renderTargetNote
	return renderReportBodyWithOptions(reports, mode, meta, opts)
}

func RenderDescriptionBodyWithOptions(reports []ClusterReport, mode diff.Mode, meta NoteMetadata, opts NoteRenderOptions) (string, error) {
	opts.target = renderTargetDescription
	return renderReportBodyWithOptions(reports, mode, meta, opts)
}

func renderReportBodyWithOptions(reports []ClusterReport, mode diff.Mode, meta NoteMetadata, opts NoteRenderOptions) (string, error) {
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
		if opts.target == renderTargetNote {
			b.WriteString(StickyMarker)
		}
		return b.String(), nil
	}

	renderedReports := cloneReports(reports)
	sortReportsForComment(renderedReports)
	renderTopSummary(&b, renderedReports, opts.target)
	b.WriteByte('\n')
	renderCommentTOC(&b, renderedReports, opts.target)
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
	if opts.target == renderTargetNote {
		b.WriteString(StickyMarker)
	}
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
		for _, warning := range chart.Warnings {
			fmt.Fprintf(&b, "! warning: %s\n", warning)
		}
		if len(chart.Warnings) > 0 {
			b.WriteByte('\n')
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
		for _, warning := range chart.Warnings {
			fmt.Fprintf(&b, "- warning: %s\n", warning)
		}
		if len(chart.Warnings) > 0 {
			fmt.Fprintln(&b)
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
	if opts.target == renderTargetNote {
		fmt.Fprintf(&b, "<a id=\"%s\"></a>\n", clusterAnchor(report.Name))
		fmt.Fprintf(&b, "## Cluster `%s`\n\n", report.Name)
	} else {
		fmt.Fprintf(&b, "## %s\n\n", descriptionClusterHeading(report.Name))
	}
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
		if opts.target == renderTargetNote {
			fmt.Fprintf(&b, "<a id=\"%s\"></a>\n", chartAnchor(report.Name, chart.Name))
		} else {
			fmt.Fprintf(&b, "### %s\n\n", descriptionChartHeading(report.Name, chart.Name))
		}
		fmt.Fprintf(&b, "<details>\n<summary>%s</summary>\n\n", chartSummaryLine(chart, added, removed, changed))
		if chart.RenderWarning != "" {
			fmt.Fprintf(&b, "> [!important]\n> Render warning: %s\n\n", chart.RenderWarning)
			renderChartSignalTable(&b, chart, added, removed, changed)
			fmt.Fprintln(&b, "</details>")
			fmt.Fprintln(&b)
			continue
		}
		for i, warning := range chart.Warnings {
			if i == 0 {
				fmt.Fprintln(&b, "> [!warning]")
			}
			fmt.Fprintf(&b, "> %s\n", warning)
		}
		if len(chart.Warnings) > 0 {
			fmt.Fprintln(&b)
		}
		renderChartSignalTable(&b, chart, added, removed, changed)
		notables := collectNotableResourceChanges(chart, 5)
		if len(notables) > 0 {
			fmt.Fprintln(&b, "**Notable changes**")
			for _, notable := range notables {
				fmt.Fprintf(&b, "- %s\n", notable)
			}
		}
		if opts.Mode == cli.CommentModeSummaryArtifacts || opts.IncludeArtifactsHint {
			fmt.Fprintln(&b, "> [!note]")
			fmt.Fprintln(&b, "> Full detailed report is available in pipeline artifacts.")
		}
		fmt.Fprintln(&b)
		if opts.Mode == cli.CommentModeSummary || opts.Mode == cli.CommentModeSummaryArtifacts {
			fmt.Fprintln(&b, "</details>")
			fmt.Fprintln(&b)
			continue
		}
		for _, resource := range chart.Resources {
			if opts.target == renderTargetNote {
				fmt.Fprintf(&b, "<a id=\"%s\"></a>\n", resourceAnchor(report.Name, resource.Kind, resource.Name))
				fmt.Fprintf(&b, "#### Resource `%s · %s/%s` (%s, severity: %s%s)\n\n", report.Name, resource.Kind, resource.Name, resource.State, resource.Assessment.Level, validationSuffix(resource.Validation))
			} else {
				fmt.Fprintf(&b, "#### %s\n\n", descriptionResourceHeading(report.Name, chart.Name, resource.Namespace, resource.Kind, resource.Name))
				fmt.Fprintf(&b, "**Resource:** `%s · %s/%s` (%s, severity: %s%s)\n\n", report.Name, resource.Kind, resource.Name, resource.State, resource.Assessment.Level, validationSuffix(resource.Validation))
			}
			if detail := validationCoverageLine(resource.Validation); detail != "" {
				fmt.Fprintf(&b, "- validation coverage: %s\n", detail)
			}
			if findings := topValidationFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- validation: %s\n", finding)
				}
			}
			if findings := topFindings(resource, 3); len(findings) > 0 {
				for _, finding := range findings {
					fmt.Fprintf(&b, "- %s\n", finding)
				}
			}
			if detail := validationCoverageLine(resource.Validation); detail != "" || len(topValidationFindings(resource, 3)) > 0 || len(topFindings(resource, 3)) > 0 {
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

type reviewHighlight struct {
	validation validate.Status
	level      severity.Level
	cluster    string
	kind       string
	name       string
	finding    string
	anchor     string
	state      string
	priority   int
}

func renderTopSummary(b *strings.Builder, reports []ClusterReport, target renderTarget) {
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
	renderNotices := 0
	highlights := collectReviewHighlights(reports, 5, target)

	for _, report := range reports {
		totalCharts += len(report.Charts)
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				renderWarnings++
			}
			renderNotices += len(chart.Warnings)
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
	if fingerprint := formatChangeFingerprint(c.added, c.removed, c.changed, severityCounts, unvalidatedResources); fingerprint != "" {
		fmt.Fprintf(b, "**Change fingerprint:** %s\n\n", fingerprint)
	}
	if validationErrors > 0 || severityCounts[severity.LevelCritical] > 0 {
		fmt.Fprintln(b, "> [!caution]")
		if validationErrors > 0 {
			fmt.Fprintf(b, "> Validation errors detected: %d\n", validationErrors)
		}
		if severityCounts[severity.LevelCritical] > 0 {
			fmt.Fprintf(b, "> Critical findings detected: %d\n", severityCounts[severity.LevelCritical])
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintf(b, "**Validation:** %d errors, %d warnings, %d unvalidated\n\n", validationErrors, validationWarnings, unvalidatedResources)
	if renderWarnings > 0 || renderNotices > 0 {
		fmt.Fprintln(b, "> [!important]")
		fmt.Fprintln(b, "> Analysis is partial.")
		if renderWarnings > 0 {
			fmt.Fprintf(b, "> %d release(s) skipped due to render warnings.\n", renderWarnings)
			fmt.Fprintf(b, "**Render warnings:** %d skipped release(s)\n\n", renderWarnings)
		}
		if renderNotices > 0 {
			fmt.Fprintf(b, "> duplicate YAML keys accepted with last-wins behavior: %d override(s).\n", renderNotices)
			fmt.Fprintf(b, "**Permissive YAML warnings:** %d duplicate-key override(s)\n\n", renderNotices)
		} else {
			fmt.Fprintln(b)
		}
	}
	if len(highlights) > 0 {
		fmt.Fprintln(b, "**Highlights**")
		fmt.Fprintln(b)
		fmt.Fprintln(b, "| Severity | Cluster | Resource | Finding |")
		fmt.Fprintln(b, "| --- | --- | --- | --- |")
		for _, highlight := range highlights {
			fmt.Fprintf(b, "| %s | `%s` | [%s](#%s) | %s |\n", severityBadge(highlight.level), highlight.cluster, fmt.Sprintf("`%s/%s`", highlight.kind, highlight.name), highlight.anchor, escapeTable(highlight.finding))
		}
		fmt.Fprintln(b)
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

func collectNotableResourceChanges(chart ChartReport, limit int) []string {
	if limit <= 0 || chart.RenderWarning != "" || len(chart.Warnings) > 0 {
		return nil
	}
	var out []string
	for _, resource := range chart.Resources {
		if line := primaryResourceHighlight(resource); line != "" {
			out = append(out, fmt.Sprintf("%s `%s/%s` **%s** · %s", severityIcon(resource.Assessment.Level), resource.Kind, resource.Name, resource.Assessment.Level, line))
			if len(out) >= limit {
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

func collectReviewHighlights(reports []ClusterReport, limit int, target renderTarget) []reviewHighlight {
	var items []reviewHighlight
	for _, report := range reports {
		for _, chart := range report.Charts {
			chartLink := chartAnchor(report.Name, chart.Name)
			if target == renderTargetDescription {
				chartLink = descriptionAnchor(descriptionChartHeading(report.Name, chart.Name))
			}
			if versionChange := chartVersionChange(chart); versionChange != "" {
				items = append(items, reviewHighlight{
					validation: validate.StatusValid,
					level:      chartSeverity(chart),
					cluster:    report.Name,
					kind:       "Chart",
					name:       chart.Name,
					finding:    "version upgrade: " + versionChange,
					anchor:     chartLink,
					priority:   2,
				})
			}
			if chart.RenderWarning != "" {
				items = append(items, reviewHighlight{
					validation: validate.StatusWarning,
					level:      severity.LevelInfo,
					cluster:    report.Name,
					kind:       "Chart",
					name:       chart.Name,
					finding:    "analysis partial: render warning skipped detailed diff",
					anchor:     chartLink,
					priority:   1,
				})
			}
			if len(chart.Warnings) > 0 {
				items = append(items, reviewHighlight{
					validation: validate.StatusWarning,
					level:      severity.LevelInfo,
					cluster:    report.Name,
					kind:       "Chart",
					name:       chart.Name,
					finding:    fmt.Sprintf("analysis partial: duplicate YAML keys accepted with last-wins behavior (%d override(s))", len(chart.Warnings)),
					anchor:     chartLink,
					priority:   1,
				})
			}
			for _, resource := range chart.Resources {
				if finding := primaryResourceHighlight(resource); finding != "" {
					resourceLink := resourceAnchor(report.Name, resource.Kind, resource.Name)
					if target == renderTargetDescription {
						resourceLink = descriptionAnchor(descriptionResourceHeading(report.Name, chart.Name, resource.Namespace, resource.Kind, resource.Name))
					}
					items = append(items, reviewHighlight{
						validation: resource.Validation.Status,
						level:      resource.Assessment.Level,
						cluster:    report.Name,
						kind:       resource.Kind,
						name:       resource.Name,
						finding:    finding,
						anchor:     resourceLink,
						state:      resource.State,
						priority:   resourceHighlightPriority(resource),
					})
				}
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		if validateStatusRank(items[i].validation) != validateStatusRank(items[j].validation) {
			return validateStatusRank(items[i].validation) > validateStatusRank(items[j].validation)
		}
		if severity.Rank(items[i].level) != severity.Rank(items[j].level) {
			return severity.Rank(items[i].level) > severity.Rank(items[j].level)
		}
		if stateWeight(items[i].state) != stateWeight(items[j].state) {
			return stateWeight(items[i].state) < stateWeight(items[j].state)
		}
		if items[i].cluster != items[j].cluster {
			return items[i].cluster < items[j].cluster
		}
		if items[i].kind != items[j].kind {
			return items[i].kind < items[j].kind
		}
		if items[i].name != items[j].name {
			return items[i].name < items[j].name
		}
		return items[i].finding < items[j].finding
	})
	out := make([]reviewHighlight, 0, limit)
	for _, item := range items {
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return dedupeHighlights(out)
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
		if opts.target == renderTargetDescription {
			fmt.Fprintln(b, "_Summary mode. Full resource diffs are omitted from this MR description report._")
		} else {
			fmt.Fprintln(b, "_Summary mode. Full resource diffs are omitted from this MR note._")
		}
	} else {
		fmt.Fprintln(b, "_Report compares merge-base and current MR state._")
	}
	fmt.Fprintln(b)
}

func renderCommentTOC(b *strings.Builder, reports []ClusterReport, target renderTarget) {
	fmt.Fprintln(b, "**Navigation**")
	fmt.Fprintln(b)
	for _, report := range reports {
		anchor := clusterAnchor(report.Name)
		if target == renderTargetDescription {
			anchor = descriptionAnchor(descriptionClusterHeading(report.Name))
		}
		fmt.Fprintf(b, "- [%s](#%s) · added %d · removed %d · changed %d\n", report.Name, anchor, report.Added, report.Removed, report.Changed)
	}
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
	fmt.Fprintln(b)
}

func chartSummaryLine(chart ChartReport, added, removed, changed int) string {
	parts := []string{fmt.Sprintf("Chart `%s`", chart.Name)}
	if versionChange := chartVersionChange(chart); versionChange != "" {
		parts = append(parts, "version "+versionChange)
	}
	parts = append(parts,
		fmt.Sprintf("namespace `%s`", emptyToNone(chart.Namespace)),
		fmt.Sprintf("severity `%s`", chartSeverity(chart)),
		fmt.Sprintf("added %d", added),
		fmt.Sprintf("removed %d", removed),
		fmt.Sprintf("changed %d", changed),
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
		parts = append(parts, "render skipped")
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

var surfaceOrder = []string{
	"security",
	"database",
	"ci/cd",
	"networking",
	"workload",
	"configuration",
	"storage",
	"policy",
	"platform",
	"observability",
	"custom",
}

func formatChartSurfaces(chart ChartReport) string {
	if chart.RenderWarning != "" {
		return ""
	}
	set := map[string]struct{}{}
	for _, resource := range chart.Resources {
		for _, surface := range resourceSurfaces(resource) {
			set[surface] = struct{}{}
		}
	}
	if len(set) == 0 {
		return ""
	}
	out := make([]string, 0, len(set))
	for _, surface := range surfaceOrder {
		if _, ok := set[surface]; ok {
			out = append(out, surface)
		}
	}
	return strings.Join(out, " · ")
}

func resourceSurfaces(resource ResourceReport) []string {
	primary := surfaceForKind(resource.Kind)
	set := map[string]struct{}{}
	if primary != "" {
		set[primary] = struct{}{}
	}
	for _, finding := range resource.Assessment.Findings {
		if surface := surfaceForFinding(finding); surface != "" {
			set[surface] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for _, surface := range surfaceOrder {
		if _, ok := set[surface]; ok {
			out = append(out, surface)
		}
	}
	return out
}

func surfaceForFinding(finding severity.Finding) string {
	reason := strings.ToLower(finding.Reason)
	switch {
	case strings.Contains(reason, "cloudnativepg"):
		return "database"
	case strings.Contains(reason, "argo cd") || strings.Contains(reason, "argocd"):
		return "ci/cd"
	case strings.Contains(reason, "keycloak") || strings.Contains(reason, "openbao") || strings.Contains(reason, "vault"):
		return "security"
	case strings.Contains(reason, "cilium") || strings.Contains(reason, "gateway"):
		return "networking"
	case strings.Contains(reason, "longhorn"):
		return "storage"
	}
	switch finding.Category {
	case "security":
		return "security"
	case "network":
		return "networking"
	case "workload", "capacity":
		return "workload"
	case "storage":
		return "storage"
	case "policy":
		return "policy"
	case "platform":
		return "platform"
	case "metadata":
		return "configuration"
	default:
		return ""
	}
}

func surfaceForKind(kind string) string {
	switch kind {
	case "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding", "ServiceAccount", "PodSecurityPolicy", "SecurityPolicy", "ReferenceGrant", "AuthorizationPolicy", "PeerAuthentication", "VaultConnection", "VaultAuth", "VaultStaticSecret", "VaultDynamicSecret", "VaultPKISecret", "VaultPKISecretRole", "VaultTransitSecret", "VaultPolicy", "VaultRole", "VaultDatabaseSecret", "VaultWrite", "VaultTransformSecret", "Keycloak", "KeycloakRealmImport", "KeycloakClient", "KeycloakRealm", "KeycloakUser", "KeycloakBackup", "KeycloakRestore", "Certificate", "CertificateRequest", "Issuer", "ClusterIssuer":
		return "security"
	case "Database", "Backup", "ScheduledBackup", "Pooler", "Publication", "Subscription", "ImageCatalog", "ClusterImageCatalog":
		return "database"
	case "Application", "ApplicationSet", "AppProject", "Rollout", "AnalysisRun", "AnalysisTemplate", "ClusterAnalysisTemplate", "Experiment":
		return "ci/cd"
	case "Service", "Ingress", "GatewayClass", "Gateway", "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute", "VirtualService", "DestinationRule", "NetworkPolicy", "CiliumClusterwideNetworkPolicy", "CiliumNetworkPolicy", "CiliumCIDRGroup", "CiliumEgressGatewayPolicy", "CiliumEndpointSlice", "CiliumEnvoyConfig", "CiliumNodeConfig", "CiliumBGPClusterConfig", "CiliumBGPPeerConfig", "CiliumBGPAdvertisement", "CiliumLoadBalancerIPPool", "CiliumL2AnnouncementPolicy", "EnvoyProxy", "BackendTrafficPolicy", "ClientTrafficPolicy", "EnvoyPatchPolicy", "BackendTLSPolicy":
		return "networking"
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "ReplicaSet":
		return "workload"
	case "ConfigMap", "Secret":
		return "configuration"
	case "PersistentVolume", "PersistentVolumeClaim", "StorageClass", "BackingImage", "BackupBackingImage", "BackupTarget", "BackupVolume", "Engine", "EngineImage", "InstanceManager", "Node", "Orphan", "RecurringJob", "Replica", "Setting", "ShareManager", "Snapshot", "SupportBundle", "SystemBackup", "SystemRestore", "Volume":
		return "storage"
	case "PodDisruptionBudget", "ResourceQuota", "LimitRange", "HorizontalPodAutoscaler", "VerticalPodAutoscaler", "PriorityClass", "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration":
		return "policy"
	case "Namespace", "CustomResourceDefinition", "APIService", "RuntimeClass", "ControllerRevision", "Lease":
		return "platform"
	case "ServiceMonitor", "PodMonitor", "PrometheusRule", "Probe", "AlertmanagerConfig", "Prometheus", "Alertmanager", "ThanosRuler", "GrafanaDashboard", "OpenTelemetryCollector", "Instrumentation":
		return "observability"
	default:
		return "custom"
	}
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

func clusterAnchor(cluster string) string {
	return "cluster-" + anchorSlug(cluster)
}

func chartAnchor(cluster, chart string) string {
	return "chart-" + anchorSlug(cluster) + "-" + anchorSlug(chart)
}

func resourceAnchor(cluster, kind, name string) string {
	return "resource-" + anchorSlug(cluster) + "-" + anchorSlug(kind) + "-" + anchorSlug(name)
}

func descriptionClusterHeading(cluster string) string {
	return fmt.Sprintf("mobius cluster %s", cluster)
}

func descriptionChartHeading(cluster, chart string) string {
	return fmt.Sprintf("mobius chart %s %s", cluster, chart)
}

func descriptionResourceHeading(cluster, chart, namespace, kind, name string) string {
	return fmt.Sprintf("mobius resource %s %s %s %s %s", cluster, chart, emptyToNone(namespace), kind, name)
}

func descriptionAnchor(heading string) string {
	return "user-content-" + anchorSlug(heading)
}

func anchorSlug(parts ...string) string {
	raw := strings.ToLower(strings.Join(parts, "-"))
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func escapeTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func dedupeHighlights(in []reviewHighlight) []reviewHighlight {
	seen := map[string]struct{}{}
	out := make([]reviewHighlight, 0, len(in))
	for _, item := range in {
		key := item.cluster + "\x00" + item.kind + "\x00" + item.name + "\x00" + item.finding + "\x00" + item.anchor + "\x00" + string(item.level) + "\x00" + string(item.validation)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
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

func resourceHighlightPriority(resource ResourceReport) int {
	switch resource.Validation.Status {
	case validate.StatusError:
		return 0
	case validate.StatusWarning:
		return 0
	}
	if resource.Assessment.Level == severity.LevelCritical || resource.Assessment.Level == severity.LevelHigh {
		return 3
	}
	switch resource.State {
	case "removed":
		return 4
	case "changed":
		return 5
	case "added":
		return 6
	default:
		return 7
	}
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
			if chart.RenderWarning != "" || len(chart.Warnings) > 0 {
				return "warnings detected"
			}
		}
	}
	if len(reports) == 0 {
		return "no effective changes"
	}
	return "changes detected"
}
