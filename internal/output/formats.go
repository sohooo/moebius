package output

import (
	"fmt"
	"strings"

	"github.com/sohooo/moebius/internal/diff"
)

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
