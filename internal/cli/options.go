// Package cli parses møbius command-line flags and subcommands.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sohooo/moebius/internal/config"
)

type Command string
type DiffMode string
type CommentMode string
type OutputFormat string
type RenderErrorMode string
type DuplicateKeyMode string
type PublishTarget string

const (
	CommandDiff     Command = "diff"
	CommandComment  Command = "comment"
	CommandVersion  Command = "version"
	CommandClusters Command = "clusters"
	CommandDoctor   Command = "doctor"

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

	PublishTargetDescription PublishTarget = "description"
	PublishTargetNote        PublishTarget = "note"
)

type Options struct {
	Command          Command
	ClustersDir      string
	AppsFiles        []string
	BaseRef          string
	Cluster          string
	AllClusters      bool
	ChartPath        string
	ValuesFiles      []string
	ReleaseName      string
	Namespace        string
	OutputDir        string
	ContextLines     int
	DiffMode         DiffMode
	CommentMode      CommentMode
	PublishTarget    PublishTarget
	MaxCommentBytes  int
	OutputFormat     OutputFormat
	Validate         bool
	RenderErrorMode  RenderErrorMode
	DuplicateKeyMode DuplicateKeyMode

	ProjectID       string
	MergeRequestIID string
	GitLabBaseURL   string
	GitLabToken     string
}

func Parse(args []string, stdout io.Writer) (Options, error) {
	var opts Options
	opts.ContextLines = 3
	opts.DiffMode = DiffModeSemantic
	opts.CommentMode = CommentModeFull
	opts.PublishTarget = PublishTargetDescription
	opts.MaxCommentBytes = 50000
	opts.OutputFormat = OutputFormatPlain
	opts.Validate = true
	opts.RenderErrorMode = RenderErrorModeFail
	opts.DuplicateKeyMode = DuplicateKeyModeError
	opts.Namespace = "default"

	fs := flag.NewFlagSet("møbius", flag.ContinueOnError)
	fs.SetOutput(stdout)
	fs.StringVar(&opts.ClustersDir, "clusters-dir", opts.ClustersDir, "Override cluster definitions directory from config.yaml")
	fs.Func("apps-files", "Comma-separated apps files relative to each cluster directory, in precedence order", func(v string) error {
		files, err := config.ParseAppsFiles(v)
		if err != nil {
			return err
		}
		opts.AppsFiles = files
		return nil
	})
	fs.StringVar(&opts.BaseRef, "base-ref", opts.BaseRef, "Base ref used for merge-base (default: origin/HEAD, then main, then master)")
	fs.StringVar(&opts.Cluster, "cluster", "", "Render and compare a single cluster")
	fs.BoolVar(&opts.AllClusters, "all-clusters", false, "Render and compare all clusters")
	fs.Func("chart-path", "Chart repository mode: path to the Helm chart (default in chart mode: .)", func(v string) error {
		path, err := parseRelativePath(v, "chart path")
		if err != nil {
			return err
		}
		opts.ChartPath = path
		return nil
	})
	fs.Func("values-files", "Chart repository mode: comma-separated values files relative to --chart-path, in precedence order", func(v string) error {
		files, err := ParseValuesFiles(v)
		if err != nil {
			return err
		}
		opts.ValuesFiles = files
		return nil
	})
	fs.StringVar(&opts.ReleaseName, "release-name", opts.ReleaseName, "Chart repository mode: Helm release name (default: Chart.yaml name)")
	fs.StringVar(&opts.Namespace, "namespace", opts.Namespace, "Chart repository mode: Helm release namespace")
	fs.StringVar(&opts.OutputDir, "output-dir", "", "Persist rendered artifacts and diffs under PATH")
	fs.IntVar(&opts.ContextLines, "context-lines", opts.ContextLines, "Unified diff context lines")
	fs.BoolVar(&opts.Validate, "validate", opts.Validate, "Validate current rendered resources against structural, schema, and semantic validators")
	fs.IntVar(&opts.MaxCommentBytes, "max-comment-bytes", opts.MaxCommentBytes, "Maximum GitLab comment body size before fallback to a compact summary")
	fs.StringVar(&opts.ProjectID, "project-id", "", "GitLab project ID override for comment mode")
	fs.StringVar(&opts.MergeRequestIID, "mr-iid", "", "GitLab merge request IID override for comment mode")
	fs.StringVar(&opts.GitLabBaseURL, "gitlab-base-url", "", "GitLab API base URL override for comment mode")
	fs.StringVar(&opts.GitLabToken, "gitlab-token", "", "GitLab API token override for comment mode (preferred over CI_JOB_TOKEN)")
	fs.Func("publish-target", "GitLab publish target for comment mode: description or note", func(v string) error {
		switch PublishTarget(v) {
		case PublishTargetDescription, PublishTargetNote:
			opts.PublishTarget = PublishTarget(v)
			return nil
		default:
			return fmt.Errorf("invalid publish target %q", v)
		}
	})
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
		fmt.Fprintf(stdout, "Usage:\n  møbius <diff|comment|version|clusters|doctor> [options]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fs.Usage()
		return opts, flag.ErrHelp
	}

	switch Command(args[0]) {
	case CommandDiff, CommandComment, CommandVersion, CommandClusters, CommandDoctor:
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
	if opts.ChartPath != "" && (opts.Cluster != "" || opts.AllClusters) {
		return opts, errors.New("--chart-path cannot be combined with --cluster or --all-clusters")
	}
	if opts.ReleaseName != "" && strings.TrimSpace(opts.ReleaseName) != opts.ReleaseName {
		return opts, errors.New("--release-name must not have surrounding whitespace")
	}
	if strings.TrimSpace(opts.Namespace) == "" || strings.TrimSpace(opts.Namespace) != opts.Namespace {
		return opts, errors.New("--namespace must not be empty or have surrounding whitespace")
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

func ParseValuesFiles(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	files := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		file, err := parseRelativePath(part, "values file")
		if err != nil {
			return nil, err
		}
		if _, ok := seen[file]; ok {
			return nil, fmt.Errorf("duplicate values file %q", file)
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	return files, nil
}

func parseRelativePath(value, label string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", fmt.Errorf("%s must not be empty", label)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%s %q must be relative", label, path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%s %q must stay within its base directory", label, path)
	}
	return clean, nil
}
