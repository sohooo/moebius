package output

import (
	"fmt"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
)

func renderFooter(b *strings.Builder, opts NoteRenderOptions, stats reportStats, meta NoteMetadata) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteString("\n")
	}
	fmt.Fprintln(b, "---")
	fmt.Fprintln(b)
	fields := []string{footerModeText(opts), validationMetadata(stats)}
	if meta.CommitSHA != "" {
		fields = append(fields, fmt.Sprintf("commit: `%s`", meta.CommitSHA))
	}
	if meta.BaseRef != "" {
		fields = append(fields, fmt.Sprintf("base ref: `%s`", meta.BaseRef))
	}
	if meta.DiffMode != "" {
		fields = append(fields, fmt.Sprintf("diff mode: `%s`", meta.DiffMode))
	}
	if meta.GeneratedAt != "" {
		fields = append(fields, fmt.Sprintf("generated: `%s`", meta.GeneratedAt))
	}
	if meta.PipelineURL != "" {
		fields = append(fields, fmt.Sprintf("[pipeline](%s)", meta.PipelineURL))
	}
	if meta.JobURL != "" {
		fields = append(fields, fmt.Sprintf("[job](%s)", meta.JobURL))
	}
	fmt.Fprintf(b, "_%s._\n", strings.Join(fields, " | "))
	fmt.Fprintln(b)
}

func footerModeText(opts NoteRenderOptions) string {
	if opts.Mode == cli.CommentModeSummaryArtifacts {
		return "Compact summary mode. Full details are available in pipeline artifacts"
	}
	if opts.Mode == cli.CommentModeSummary {
		if opts.target == renderTargetDescription {
			return "Summary mode. Full resource diffs are omitted from this MR description report"
		}
		return "Summary mode. Full resource diffs are omitted from this MR note"
	}
	return "Report compares merge-base and current MR state"
}

func validationMetadata(stats reportStats) string {
	if stats.validationErrors == 0 && stats.validationWarnings == 0 && stats.unvalidatedResources == 0 {
		return "validation: clean"
	}
	return fmt.Sprintf("validation: %d errors, %d warnings, %d unvalidated", stats.validationErrors, stats.validationWarnings, stats.unvalidatedResources)
}
