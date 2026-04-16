// Package cli parses møbius command-line flags and subcommands.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

type Command string
type DiffMode string
type CommentMode string
type OutputFormat string
type RenderErrorMode string
type DuplicateKeyMode string

const (
	CommandDiff    Command = "diff"
	CommandComment Command = "comment"
	CommandVersion Command = "version"

	DiffModeRaw      DiffMode = "raw"
	DiffModeSemantic DiffMode = "semantic"
	DiffModeBoth     DiffMode = "both"

	CommentModeFull             CommentMode = "full"
	CommentModeSummary          CommentMode = "summary"
	CommentModeSummaryArtifacts CommentMode = "summary+artifacts"

	OutputFormatPlain    OutputFormat = "plain"
	OutputFormatMarkdown OutputFormat = "markdown"

	RenderErrorModeFail            RenderErrorMode = "fail"
	RenderErrorModeWarnSkipRelease RenderErrorMode = "warn-skip-release"

	DuplicateKeyModeError        DuplicateKeyMode = "error"
	DuplicateKeyModeWarnLastWins DuplicateKeyMode = "warn-last-wins"
)

type Options struct {
	Command         Command
	ClustersDir     string
	BaseRef         string
	Cluster         string
	AllClusters     bool
	OutputDir       string
	ContextLines    int
	DiffMode        DiffMode
	CommentMode     CommentMode
	MaxCommentBytes int
	OutputFormat    OutputFormat
	Validate        bool
	RenderErrorMode RenderErrorMode
	DuplicateKeyMode DuplicateKeyMode

	ProjectID       string
	MergeRequestIID string
	GitLabBaseURL   string
}

func Parse(args []string, stdout io.Writer) (Options, error) {
	var opts Options
	opts.BaseRef = "master"
	opts.ContextLines = 3
	opts.DiffMode = DiffModeSemantic
	opts.CommentMode = CommentModeFull
	opts.MaxCommentBytes = 50000
	opts.OutputFormat = OutputFormatPlain
	opts.Validate = true
	opts.RenderErrorMode = RenderErrorModeFail
	opts.DuplicateKeyMode = DuplicateKeyModeError

	fs := flag.NewFlagSet("møbius", flag.ContinueOnError)
	fs.SetOutput(stdout)
	fs.StringVar(&opts.ClustersDir, "clusters-dir", opts.ClustersDir, "Override cluster definitions directory from config.yaml")
	fs.StringVar(&opts.BaseRef, "base-ref", opts.BaseRef, "Base ref used for merge-base")
	fs.StringVar(&opts.Cluster, "cluster", "", "Render and compare a single cluster")
	fs.BoolVar(&opts.AllClusters, "all-clusters", false, "Render and compare all clusters")
	fs.StringVar(&opts.OutputDir, "output-dir", "", "Persist rendered artifacts and diffs under PATH")
	fs.IntVar(&opts.ContextLines, "context-lines", opts.ContextLines, "Unified diff context lines")
	fs.BoolVar(&opts.Validate, "validate", opts.Validate, "Validate current rendered resources against structural, schema, and semantic validators")
	fs.IntVar(&opts.MaxCommentBytes, "max-comment-bytes", opts.MaxCommentBytes, "Maximum GitLab comment body size before fallback to a compact summary")
	fs.StringVar(&opts.ProjectID, "project-id", "", "GitLab project ID override for comment mode")
	fs.StringVar(&opts.MergeRequestIID, "mr-iid", "", "GitLab merge request IID override for comment mode")
	fs.StringVar(&opts.GitLabBaseURL, "gitlab-base-url", "", "GitLab API base URL override for comment mode")
	fs.Func("diff-mode", "Diff output mode: raw, semantic, or both", func(v string) error {
		switch DiffMode(v) {
		case DiffModeRaw, DiffModeSemantic, DiffModeBoth:
			opts.DiffMode = DiffMode(v)
			return nil
		default:
			return fmt.Errorf("invalid diff mode %q", v)
		}
	})
	fs.Func("output-format", "Output format: plain or markdown", func(v string) error {
		switch OutputFormat(v) {
		case OutputFormatPlain, OutputFormatMarkdown:
			opts.OutputFormat = OutputFormat(v)
			return nil
		default:
			return fmt.Errorf("invalid output format %q", v)
		}
	})
	fs.Func("comment-mode", "Comment mode: full, summary, or summary+artifacts", func(v string) error {
		switch CommentMode(v) {
		case CommentModeFull, CommentModeSummary, CommentModeSummaryArtifacts:
			opts.CommentMode = CommentMode(v)
			return nil
		default:
			return fmt.Errorf("invalid comment mode %q", v)
		}
	})
	fs.Func("render-error-mode", "Rendered manifest error mode: fail or warn-skip-release", func(v string) error {
		switch RenderErrorMode(v) {
		case RenderErrorModeFail, RenderErrorModeWarnSkipRelease:
			opts.RenderErrorMode = RenderErrorMode(v)
			return nil
		default:
			return fmt.Errorf("invalid render error mode %q", v)
		}
	})
	fs.Func("duplicate-key-mode", "Duplicate YAML key mode: error or warn-last-wins", func(v string) error {
		switch DuplicateKeyMode(v) {
		case DuplicateKeyModeError, DuplicateKeyModeWarnLastWins:
			opts.DuplicateKeyMode = DuplicateKeyMode(v)
			return nil
		default:
			return fmt.Errorf("invalid duplicate key mode %q", v)
		}
	})

	fs.Usage = func() {
		fmt.Fprintf(stdout, "Usage:\n  møbius <diff|comment|version> [options]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fs.Usage()
		return opts, flag.ErrHelp
	}

	switch Command(args[0]) {
	case CommandDiff, CommandComment, CommandVersion:
		opts.Command = Command(args[0])
	default:
		return opts, fmt.Errorf("unknown subcommand %q", args[0])
	}

	if err := fs.Parse(args[1:]); err != nil {
		return opts, err
	}
	if opts.Cluster != "" && opts.AllClusters {
		return opts, errors.New("--cluster and --all-clusters cannot be combined")
	}
	if opts.ContextLines < 0 {
		return opts, errors.New("--context-lines must be >= 0")
	}
	if opts.MaxCommentBytes < 1024 {
		return opts, errors.New("--max-comment-bytes must be >= 1024")
	}
	if opts.Command == CommandVersion {
		return opts, nil
	}
	if opts.Command == CommandComment {
		opts.OutputFormat = OutputFormatMarkdown
	}
	return opts, nil
}
