package buildinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestMergeUsesGoBuildInfoWhenLinkerValuesAreUnset(t *testing.T) {
	info := merge(Info{Version: "dev"}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef123456"},
			{Key: "vcs.time", Value: "2026-04-07T20:00:00Z"},
		},
	})

	if info.Version != "v0.1.0" {
		t.Fatalf("expected build info version, got %q", info.Version)
	}
	if info.Commit != "abcdef123456" {
		t.Fatalf("expected build info commit, got %q", info.Commit)
	}
	if info.Date != "2026-04-07T20:00:00Z" {
		t.Fatalf("expected build info date, got %q", info.Date)
	}
}

func TestMergePrefersExplicitLinkerValues(t *testing.T) {
	info := merge(Info{
		Version: "v9.9.9",
		Commit:  "deadbeef",
		Date:    "2026-04-01T00:00:00Z",
	}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef123456"},
			{Key: "vcs.time", Value: "2026-04-07T20:00:00Z"},
		},
	})

	if info.Version != "v9.9.9" || info.Commit != "deadbeef" || info.Date != "2026-04-01T00:00:00Z" {
		t.Fatalf("expected explicit linker values to win, got %+v", info)
	}
}

func TestStringIncludesVersionCommitAndDate(t *testing.T) {
	previousVersion, previousCommit, previousDate := Version, Commit, Date
	t.Cleanup(func() {
		Version = previousVersion
		Commit = previousCommit
		Date = previousDate
	})

	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-04-07T20:00:00Z"

	out := String()
	for _, want := range []string{"version: v1.2.3", "commit: abc123", "built: 2026-04-07T20:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output %q", want, out)
		}
	}
}
