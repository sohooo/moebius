package cli

import (
	"bytes"
	"errors"
	"flag"
	"slices"
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

func TestParseClustersAndDoctorCommands(t *testing.T) {
	for _, command := range []Command{CommandClusters, CommandDoctor} {
		opts, err := Parse([]string{string(command)}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", command, err)
		}
		if opts.Command != command {
			t.Fatalf("expected command %q, got %q", command, opts.Command)
		}
	}
}

func TestParseHelpListsVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	_, err := Parse([]string{"--help"}, &stdout)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	if !strings.Contains(stdout.String(), "<diff|comment|version|clusters|doctor>") {
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

func TestParseGitLabToken(t *testing.T) {
	opts, err := Parse([]string{"comment", "--gitlab-token", "secret"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.GitLabToken != "secret" {
		t.Fatalf("expected gitlab token override, got %q", opts.GitLabToken)
	}
}

func TestParseAppsFiles(t *testing.T) {
	opts, err := Parse([]string{"diff", "--apps-files", "apps.yaml, apps-dev.yaml"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !slices.Equal(opts.AppsFiles, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("unexpected apps files: %v", opts.AppsFiles)
	}
}

func TestParseAppsFilesRejectsInvalidList(t *testing.T) {
	for _, value := range []string{"apps.yaml,", "/apps.yaml", "../apps.yaml", "apps.yaml,apps.yaml"} {
		if _, err := Parse([]string{"diff", "--apps-files", value}, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected --apps-files %q to fail", value)
		}
	}
}

func TestParseChartRepositoryFlags(t *testing.T) {
	opts, err := Parse([]string{"comment", "--chart-path", ".", "--values-files", "values.yaml, values-ci.yaml", "--release-name", "app", "--namespace", "demo"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.ChartPath != "." {
		t.Fatalf("unexpected chart path %q", opts.ChartPath)
	}
	if !slices.Equal(opts.ValuesFiles, []string{"values.yaml", "values-ci.yaml"}) {
		t.Fatalf("unexpected values files: %v", opts.ValuesFiles)
	}
	if opts.ReleaseName != "app" {
		t.Fatalf("unexpected release name %q", opts.ReleaseName)
	}
	if opts.Namespace != "demo" {
		t.Fatalf("unexpected namespace %q", opts.Namespace)
	}
}

func TestParseValuesFilesRejectsInvalidList(t *testing.T) {
	for _, value := range []string{"values.yaml,", "/values.yaml", "../values.yaml", "values.yaml,values.yaml"} {
		if _, err := Parse([]string{"diff", "--values-files", value}, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected --values-files %q to fail", value)
		}
	}
}

func TestParseChartPathCannotCombineWithClusterSelection(t *testing.T) {
	if _, err := Parse([]string{"diff", "--chart-path", ".", "--cluster", "kube-bravo"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected --chart-path with --cluster to fail")
	}
	if _, err := Parse([]string{"diff", "--chart-path", ".", "--all-clusters"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected --chart-path with --all-clusters to fail")
	}
}

func TestParsePublishTargetDefaultsToDescription(t *testing.T) {
	opts, err := Parse([]string{"comment"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.PublishTarget != PublishTargetDescription {
		t.Fatalf("expected description publish target, got %q", opts.PublishTarget)
	}
}

func TestParsePublishTargetNote(t *testing.T) {
	opts, err := Parse([]string{"comment", "--publish-target", "note"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.PublishTarget != PublishTargetNote {
		t.Fatalf("expected note publish target, got %q", opts.PublishTarget)
	}
}
