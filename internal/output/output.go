// Package output renders cluster reports for terminals, markdown, and MR notes.
package output

import (
	"fmt"
	"io"
	"sort"
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
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "Resource: %s/%s (%s)\n", resource.Kind, resource.Name, resource.State)
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
		for _, resource := range chart.Resources {
			fmt.Fprintf(&b, "#### Resource `%s/%s` (%s)\n\n", resource.Kind, resource.Name, resource.State)
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
		fmt.Fprintf(&b, "<details>\n<summary>Chart `%s` · namespace `%s` · added %d · removed %d · changed %d</summary>\n\n", chart.Name, emptyToNone(chart.Namespace), added, removed, changed)
		fmt.Fprintf(&b, "- Kinds affected: %s\n", strings.Join(chartKinds(chart), ", "))
		if onlyValueTweaks(chart) {
			fmt.Fprintln(&b, "- Scope: value-level tweaks only")
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
			fmt.Fprintf(&b, "#### Resource `%s/%s` (%s)\n\n", resource.Kind, resource.Name, resource.State)
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
	var highlights []string
	var riskLabels []string
	riskSet := map[string]struct{}{}

	for _, report := range reports {
		totalCharts += len(report.Charts)
		for _, chart := range report.Charts {
			added, removed, changed := chartChangeCounts(chart)
			c.added += added
			c.removed += removed
			c.changed += changed
			totalResources += added + removed + changed
			if len(highlights) < 5 {
				if label := chartHighlight(chart); label != "" {
					highlights = append(highlights, label)
				}
			}
			for _, resource := range chart.Resources {
				if risk := riskyKindLabel(resource.Kind); risk != "" {
					riskSet[risk] = struct{}{}
				}
			}
		}
	}

	for risk := range riskSet {
		riskLabels = append(riskLabels, risk)
	}
	sort.Strings(riskLabels)

	fmt.Fprintln(b, "## Review Summary")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "| Clusters | Charts | Resources | Added | Removed | Changed |")
	fmt.Fprintln(b, "| ---: | ---: | ---: | ---: | ---: | ---: |")
	fmt.Fprintf(b, "| %d | %d | %d | %d | %d | %d |\n\n", totalClusters, totalCharts, totalResources, c.added, c.removed, c.changed)
	if len(riskLabels) > 0 {
		fmt.Fprintf(b, "**Risk labels:** %s\n\n", strings.Join(riskLabels, ", "))
	}
	if len(highlights) > 0 {
		fmt.Fprintln(b, "**Highlights**")
		fmt.Fprintln(b)
		for _, highlight := range highlights {
			fmt.Fprintf(b, "- %s\n", highlight)
		}
	}
}

func chartHighlight(chart ChartReport) string {
	if notables := collectNotableChanges(chart); len(notables) > 0 {
		return notables[0]
	}
	added, removed, changed := chartChangeCounts(chart)
	return fmt.Sprintf("Chart `%s`: %d added, %d removed, %d changed", chart.Name, added, removed, changed)
}

func chartKinds(chart ChartReport) []string {
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
	for _, resource := range chart.Resources {
		if resource.State != "changed" {
			return false
		}
	}
	return true
}

func collectNotableChanges(chart ChartReport) []string {
	var out []string
	for _, resource := range chart.Resources {
		for _, line := range notableChanges(resource) {
			out = append(out, fmt.Sprintf("`%s/%s`: %s", resource.Kind, resource.Name, line))
			if len(out) >= 5 {
				return out
			}
		}
	}
	return out
}

func notableChanges(resource ResourceReport) []string {
	var out []string
	clusterScoped := resource.Result.Changes == nil && resource.State != "" // no-op placeholder not used
	_ = clusterScoped
	if risk := riskyKindLabel(resource.Kind); risk != "" {
		out = append(out, risk)
	}
	for _, change := range resource.Result.Changes {
		path := diff.PathString(change.Path)
		switch {
		case path == "spec.replicas":
			out = append(out, fmt.Sprintf("replicas changed %v -> %v", change.Old, change.New))
		case strings.Contains(path, ".image") && !strings.Contains(path, "imagePullPolicy"):
			out = append(out, fmt.Sprintf("image changed %v -> %v", change.Old, change.New))
		case strings.HasSuffix(path, "imagePullPolicy"):
			out = append(out, fmt.Sprintf("image pull policy changed %v -> %v", change.Old, change.New))
		case strings.Contains(path, ".resources.requests.") || strings.Contains(path, ".resources.limits."):
			out = append(out, fmt.Sprintf("resource sizing changed at `%s`", path))
		case path == "spec.type":
			out = append(out, fmt.Sprintf("service type changed %v -> %v", change.Old, change.New))
		case strings.Contains(path, ".host") || strings.Contains(path, ".hosts"):
			out = append(out, fmt.Sprintf("ingress host changed at `%s`", path))
		case strings.Contains(path, ".path") || strings.Contains(path, ".paths"):
			if strings.Contains(path, "rules") || strings.Contains(path, "http") {
				out = append(out, fmt.Sprintf("ingress path changed at `%s`", path))
			}
		}
		if len(out) >= 5 {
			break
		}
	}
	return dedupeStrings(out)
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

func riskyKindLabel(kind string) string {
	switch kind {
	case "Namespace", "CustomResourceDefinition", "ClusterRole", "ClusterRoleBinding", "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration":
		return fmt.Sprintf("risk: %s changed", kind)
	default:
		return ""
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
	if len(reports) == 0 {
		return "no effective changes"
	}
	return "changes detected"
}
