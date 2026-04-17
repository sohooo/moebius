// Package app wires the CLI to the report and comment services.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sohooo/moebius/internal/buildinfo"
	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/comment"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/report"
)

type commentService interface {
	Preflight(context.Context, cli.Options) (comment.StatusReport, error)
	Post(context.Context, cli.Options, []output.ClusterReport) (comment.Result, error)
}

var (
	parseOptions      = cli.Parse
	buildReports      = report.Build
	newCommentService = func() commentService { return comment.New() }
	printReports      = output.PrintReports
)

func Run(args []string) error {
	return run(args, os.Stdout)
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args, stdout)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if opts.Command == cli.CommandVersion {
		fmt.Fprint(stdout, buildinfo.String())
		return nil
	}

	var (
		ctx          = context.Background()
		service      commentService
		statusReport comment.StatusReport
		preflightErr error
	)
	if opts.Command == cli.CommandComment {
		service = newCommentService()
		statusReport, preflightErr = service.Preflight(ctx, opts)
		if opts.OutputDir != "" {
			_ = comment.WriteStatusArtifact(opts.OutputDir, statusReport)
		}
	}

	reports, outputDir, err := buildReports(opts)
	if err != nil {
		return err
	}

	switch opts.Command {
	case cli.CommandComment:
		if preflightErr != nil {
			printCommentFailure(stdout, opts, reports, outputDir, statusReport)
			return preflightErr
		}
		result, err := service.Post(ctx, opts, reports)
		if err != nil {
			statusReport.Status = comment.StatusError
			statusReport.Stage = "post"
			statusReport.Action = "failed"
			statusReport.Messages = append(statusReport.Messages, err.Error())
			if opts.OutputDir != "" {
				_ = comment.WriteStatusArtifact(outputDir, statusReport)
			}
			printCommentFailure(stdout, opts, reports, outputDir, statusReport)
			return err
		}
		statusReport.Status = comment.StatusOK
		statusReport.Stage = "post"
		statusReport.Action = result.Action
		statusReport.Messages = append(statusReport.Messages, result.Message)
		if opts.OutputDir != "" {
			_ = comment.WriteStatusArtifact(outputDir, statusReport)
		}
		fmt.Fprintln(stdout, result.Message)
	default:
		if len(reports) == 0 {
			fmt.Fprintln(stdout, "No affected clusters.")
			return nil
		}
		if err := printReports(stdout, reports, diff.Mode(opts.DiffMode), opts.OutputFormat); err != nil {
			return err
		}
		if opts.OutputDir != "" {
			fmt.Fprintf(stdout, "Artifacts written to %s\n", outputDir)
		}
	}

	return nil
}

func printCommentFailure(w io.Writer, opts cli.Options, reports []output.ClusterReport, outputDir string, status comment.StatusReport) {
	fmt.Fprintln(w, "møbius comment failed.")
	for _, message := range status.Messages {
		fmt.Fprintf(w, "- %s\n", message)
	}
	if status.ProjectID != "" || status.MergeRequestIID != "" || status.BaseURL != "" {
		fmt.Fprintf(w, "- GitLab target: project=%s mr=!%s base=%s\n", emptyOrUnknown(status.ProjectID), emptyOrUnknown(status.MergeRequestIID), emptyOrUnknown(status.BaseURL))
	}
	if status.TokenKind != "" || status.TokenSource != "" {
		fmt.Fprintf(w, "- Token: kind=%s source=%s\n", emptyOrUnknown(string(status.TokenKind)), emptyOrUnknown(status.TokenSource))
	}
	if opts.OutputDir != "" {
		fmt.Fprintf(w, "- Artifacts written to %s\n", outputDir)
	}
	if len(reports) == 0 {
		fmt.Fprintln(w, "No affected clusters.")
		return
	}
	fmt.Fprintln(w)
	_ = printReports(w, reports, diff.Mode(opts.DiffMode), opts.OutputFormat)
}

func emptyOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
