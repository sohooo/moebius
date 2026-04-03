package output

import (
	"fmt"
	"io"
	"strings"

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

func PrintCluster(w io.Writer, report ClusterReport, mode diff.Mode) {
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

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}
