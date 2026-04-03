package output

import (
	"fmt"
	"io"
	"strings"

	"mobius/internal/cli"
	"mobius/internal/diff"
)

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

func PrintCluster(w io.Writer, report ClusterReport, mode diff.Mode, format cli.OutputFormat) {
	switch format {
	case cli.OutputFormatMarkdown:
		printClusterMarkdown(w, report, mode)
	default:
		printClusterPlain(w, report, mode)
	}
}

func printClusterPlain(w io.Writer, report ClusterReport, mode diff.Mode) {
	if len(report.Charts) == 0 {
		fmt.Fprintf(w, "== Cluster: %s ==\nNo effective changes.\n\n", report.Name)
		return
	}

	fmt.Fprintf(w, "== Cluster: %s ==\n", report.Name)
	for _, chart := range report.Charts {
		fmt.Fprintf(w, "-- Chart: %s (namespace: %s) --\n", chart.Name, emptyToNone(chart.Namespace))
		for _, resource := range chart.Resources {
			fmt.Fprintf(w, "Resource: %s/%s (%s)\n", resource.Kind, resource.Name, resource.State)
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(resource.Semantic) != "" {
				semanticConsole, err := diff.RenderSemanticConsole(resource.Result.Changes)
				if err != nil || strings.TrimSpace(semanticConsole) == "" {
					semanticConsole = resource.Semantic
				}
				fmt.Fprintln(w, strings.TrimSpace(semanticConsole))
				fmt.Fprintln(w)
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(resource.Semantic) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
				fmt.Fprintln(w, strings.TrimSpace(resource.Result.RawDiff))
				fmt.Fprintln(w)
			}
		}
	}
	fmt.Fprintf(w, "Summary for %s: added=%d removed=%d changed=%d\n\n", report.Name, report.Added, report.Removed, report.Changed)
}

func printClusterMarkdown(w io.Writer, report ClusterReport, mode diff.Mode) {
	fmt.Fprintf(w, "## Cluster `%s`\n\n", report.Name)
	fmt.Fprintln(w, "| Added | Removed | Changed |")
	fmt.Fprintln(w, "| ---: | ---: | ---: |")
	fmt.Fprintf(w, "| %d | %d | %d |\n\n", report.Added, report.Removed, report.Changed)

	if len(report.Charts) == 0 {
		fmt.Fprintln(w, "_No effective changes._")
		fmt.Fprintln(w)
		return
	}

	for _, chart := range report.Charts {
		fmt.Fprintf(w, "### Chart `%s`\n\n", chart.Name)
		fmt.Fprintf(w, "- Namespace: `%s`\n\n", emptyToNone(chart.Namespace))
		for _, resource := range chart.Resources {
			fmt.Fprintf(w, "#### Resource `%s/%s` (%s)\n\n", resource.Kind, resource.Name, resource.State)
			if (mode == diff.ModeSemantic || mode == diff.ModeBoth) && strings.TrimSpace(resource.Semantic) != "" {
				semanticMarkdown, err := diff.RenderSemanticMarkdown(resource.Result.Changes)
				if err != nil || strings.TrimSpace(semanticMarkdown) == "" {
					semanticMarkdown = resource.Semantic
				}
				fmt.Fprintln(w, "```diff")
				fmt.Fprintln(w, strings.TrimSpace(semanticMarkdown))
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w)
			}
			if ((mode == diff.ModeRaw || mode == diff.ModeBoth) || (mode == diff.ModeSemantic && strings.TrimSpace(resource.Semantic) == "")) && strings.TrimSpace(resource.Result.RawDiff) != "" {
				label := "Raw diff"
				if mode == diff.ModeRaw {
					label = "Diff"
				}
				fmt.Fprintf(w, "**%s**\n\n", label)
				fmt.Fprintln(w, "```diff")
				fmt.Fprintln(w, strings.TrimSpace(resource.Result.RawDiff))
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w)
			}
		}
	}
}

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}
