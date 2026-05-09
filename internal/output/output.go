// Package output renders cluster reports for terminals, markdown, and MR notes.
package output

import (
	"io"
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
