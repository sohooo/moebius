package cli

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseVersionCommand(t *testing.T) {
	opts, err := Parse([]string{"version"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.Command != CommandVersion {
		t.Fatalf("expected version command, got %q", opts.Command)
	}
}

func TestParseHelpListsVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	_, err := Parse([]string{"--help"}, &stdout)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	if !strings.Contains(stdout.String(), "<diff|comment|version>") {
		t.Fatalf("expected usage to mention version command, got %q", stdout.String())
	}
}

func TestParseRenderErrorMode(t *testing.T) {
	opts, err := Parse([]string{"comment", "--render-error-mode", "warn-skip-release"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.RenderErrorMode != RenderErrorModeWarnSkipRelease {
		t.Fatalf("expected warn-skip-release, got %q", opts.RenderErrorMode)
	}
}

func TestParseDuplicateKeyMode(t *testing.T) {
	opts, err := Parse([]string{"comment", "--duplicate-key-mode", "warn-last-wins"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.DuplicateKeyMode != DuplicateKeyModeWarnLastWins {
		t.Fatalf("expected warn-last-wins, got %q", opts.DuplicateKeyMode)
	}
}
