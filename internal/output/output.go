package output

import (
	"fmt"
	"io"
	"strings"

	"mobius/internal/cli"
	"mobius/internal/diff"
)

const StickyMarker = "<!-- mobius:mr-diff -->"

type ResourceReport struct {
	State    string
	Kind     string
	Name     string
	Result   diff.Result
	Semantic string
}

type ChartReport struct {
	Name      string
	Namespace string
	Resources []ResourceReport
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
	var b strings.Builder
	b.WriteString("# møbius Diff Report\n\n")
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
		b.WriteString(strings.Join(fields, "  \n"))
		b.WriteString("\n\n")
	}

	if len(reports) == 0 {
		b.WriteString("_No effective changes._\n\n")
		b.WriteString(StickyMarker)
		return b.String(), nil
	}

	body, err := RenderReports(reports, mode, cli.OutputFormatMarkdown)
	if err != nil {
		return "", err
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
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
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "Resource: %s/%s (%s)\n", resource.Kind, resource.Name, resource.State)
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(resource.Semantic) != "" {
				semanticConsole, err := diff.RenderSemanticConsole(resource.Result.Changes)
				if err != nil || strings.TrimSpace(semanticConsole) == "" {
					semanticConsole = resource.Semantic
				}
				b.WriteString(strings.TrimSpace(semanticConsole))
				b.WriteString("\n\n")
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(resource.Semantic) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
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
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "#### Resource `%s/%s` (%s)\n\n", resource.Kind, resource.Name, resource.State)
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(resource.Semantic) != "" {
				semanticMarkdown, err := diff.RenderSemanticMarkdown(resource.Result.Changes)
				if err != nil || strings.TrimSpace(semanticMarkdown) == "" {
					semanticMarkdown = resource.Semantic
				}
				fmt.Fprintln(&b, "```diff")
				fmt.Fprintln(&b, strings.TrimSpace(semanticMarkdown))
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b)
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(resource.Semantic) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
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

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}
