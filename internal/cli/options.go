package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

type Command string
type DiffMode string
type OutputFormat string

const (
	CommandDiff    Command = "diff"
	CommandComment Command = "comment"

	DiffModeRaw      DiffMode = "raw"
	DiffModeSemantic DiffMode = "semantic"
	DiffModeBoth     DiffMode = "both"

	OutputFormatPlain    OutputFormat = "plain"
	OutputFormatMarkdown OutputFormat = "markdown"
)

type Options struct {
	Command      Command
	ClustersDir  string
	BaseRef      string
	Cluster      string
	AllClusters  bool
	OutputDir    string
	ContextLines int
	DiffMode     DiffMode
	OutputFormat OutputFormat

	ProjectID       string
	MergeRequestIID string
	GitLabBaseURL   string
}

func Parse(args []string, stdout io.Writer) (Options, error) {
	var opts Options
	opts.ClustersDir = "clusters"
	opts.BaseRef = "master"
	opts.ContextLines = 3
	opts.DiffMode = DiffModeSemantic
	opts.OutputFormat = OutputFormatPlain

	fs := flag.NewFlagSet("møbius", flag.ContinueOnError)
	fs.SetOutput(stdout)
	fs.StringVar(&opts.ClustersDir, "clusters-dir", opts.ClustersDir, "Cluster definitions directory")
	fs.StringVar(&opts.BaseRef, "base-ref", opts.BaseRef, "Base ref used for merge-base")
	fs.StringVar(&opts.Cluster, "cluster", "", "Render and compare a single cluster")
	fs.BoolVar(&opts.AllClusters, "all-clusters", false, "Render and compare all clusters")
	fs.StringVar(&opts.OutputDir, "output-dir", "", "Persist rendered artifacts and diffs under PATH")
	fs.IntVar(&opts.ContextLines, "context-lines", opts.ContextLines, "Unified diff context lines")
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

	fs.Usage = func() {
		fmt.Fprintf(stdout, "Usage:\n  møbius <diff|comment> [options]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fs.Usage()
		return opts, flag.ErrHelp
	}

	switch Command(args[0]) {
	case CommandDiff, CommandComment:
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
	if opts.Command == CommandComment {
		opts.OutputFormat = OutputFormatMarkdown
	}
	return opts, nil
}
