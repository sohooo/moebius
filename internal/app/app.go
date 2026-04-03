// Package app wires the CLI to the report and comment services.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"mobius/internal/cli"
	"mobius/internal/comment"
	"mobius/internal/diff"
	"mobius/internal/output"
	"mobius/internal/report"
)

func Run(args []string) error {
	opts, err := cli.Parse(args, os.Stdout)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	reports, outputDir, err := report.Build(opts)
	if err != nil {
		return err
	}

	switch opts.Command {
	case cli.CommandComment:
		service := comment.New()
		result, err := service.Post(context.Background(), opts, reports)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, result.Message)
	default:
		if len(reports) == 0 {
			fmt.Fprintln(os.Stdout, "No affected clusters.")
			return nil
		}
		if err := output.PrintReports(os.Stdout, reports, diff.Mode(opts.DiffMode), opts.OutputFormat); err != nil {
			return err
		}
		if opts.OutputDir != "" {
			fmt.Fprintf(os.Stdout, "Artifacts written to %s\n", outputDir)
		}
	}

	return nil
}
