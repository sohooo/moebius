package output

import (
	"fmt"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/validate"
)

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
	if opts.Mode == "" {
		opts.Mode = cli.CommentModeFull
	}
	var b strings.Builder
	b.WriteString("# møbius Diff Report\n\n")
	if opts.Status != "" {
		fmt.Fprintf(&b, "**Status:** %s\n\n", opts.Status)
	}

	if len(reports) == 0 {
		b.WriteString("_No effective changes._\n\n")
		renderFooter(&b, opts, reportStats{}, meta)
		if opts.target == renderTargetNote {
			b.WriteString(StickyMarker)
		}
		return b.String(), nil
	}

	renderedReports := cloneReports(reports)
	sortReportsForComment(renderedReports)
	stats := collectReportStats(renderedReports)
	renderPartialAnalysisWarnings(&b, stats)
	renderReviewFocus(&b, renderedReports, opts.target)
	if opts.target == renderTargetDescription && opts.Mode == cli.CommentModeFull {
		renderReviewChecklist(&b, renderedReports)
	}
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b)
	renderCommentTOC(&b, renderedReports, opts.target)
	b.WriteByte('\n')

	if err := renderClusterDetails(&b, renderedReports, mode, opts); err != nil {
		return "", err
	}
	renderFooter(&b, opts, stats, meta)
	if opts.target == renderTargetNote {
		b.WriteString(StickyMarker)
	}
	return b.String(), nil
}

func renderClusterComment(report ClusterReport, mode diff.Mode, opts NoteRenderOptions) (string, error) {
	var b strings.Builder
	if opts.target == renderTargetNote {
		fmt.Fprintf(&b, "<a id=\"%s\"></a>\n", clusterAnchor(report.Name))
		fmt.Fprintf(&b, "## Cluster `%s`\n\n", report.Name)
	} else {
		fmt.Fprintf(&b, "## %s\n\n", descriptionClusterHeading(report.Name))
	}

	if len(report.Charts) == 0 {
		fmt.Fprintln(&b, "_No effective changes._")
		return strings.TrimRight(b.String(), "\n"), nil
	}

	for _, chart := range report.Charts {
		added, removed, changed := chartChangeCounts(chart)
		if opts.target == renderTargetNote {
			fmt.Fprintf(&b, "<a id=\"%s\"></a>\n", chartAnchor(report.Name, chart.Name))
		} else {
			fmt.Fprintf(&b, "### %s\n\n", descriptionChartHeading(report.Name, chart.Name))
		}
		fmt.Fprintf(&b, "<details>\n<summary>%s</summary>\n\n", chartSummaryLine(chart, added, removed, changed))
		if chart.RenderWarning != "" {
			fmt.Fprintf(&b, "> [!important]\n> Render warning: %s\n\n", chartRenderWarningSummary(chart.RenderWarning))
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
		renderChartReviewHints(&b, chart)
		linkChanges := opts.Mode == cli.CommentModeFull
		changes := collectChartResourceChanges(report.Name, chart, opts.target, linkChanges)
		if len(changes) > 0 {
			fmt.Fprintln(&b, "**Changes**")
			for _, change := range changes {
				fmt.Fprintf(&b, "- %s\n", change)
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
			}
			fmt.Fprintf(&b, "- %s\n", resourceMetadataLine(report.Name, chart.Name, resource, opts.target))
			if resource.Validation.Status != "" && resource.Validation.Status != validate.StatusValid {
				for _, finding := range topValidationFindings(resource, 3) {
					fmt.Fprintf(&b, "- validation: %s\n", finding)
				}
			}
			fmt.Fprintln(&b)
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

func renderClusterDetails(b *strings.Builder, reports []ClusterReport, mode diff.Mode, opts NoteRenderOptions) error {
	for i := range reports {
		chunk, err := renderClusterComment(reports[i], mode, opts)
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(chunk, "\n"))
		b.WriteString("\n\n")
	}
	return nil
}

func renderCommentTOC(b *strings.Builder, reports []ClusterReport, target renderTarget) {
	fmt.Fprintln(b, "**Navigation**")
	fmt.Fprintln(b)
	for _, report := range reports {
		anchor := clusterAnchor(report.Name)
		if target == renderTargetDescription {
			anchor = descriptionAnchor(descriptionClusterHeading(report.Name))
		}
		parts := []string{
			fmt.Sprintf("[%s](#%s)", report.Name, anchor),
			fmt.Sprintf("added %d", report.Added),
			fmt.Sprintf("removed %d", report.Removed),
			fmt.Sprintf("changed %d", report.Changed),
		}
		if summary := formatSeveritySummaryIcons(clusterSeverityCounts(report)); summary != "" {
			parts = append(parts, "severity: "+summary)
		}
		fmt.Fprintf(b, "- %s\n", strings.Join(parts, " · "))
	}
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
