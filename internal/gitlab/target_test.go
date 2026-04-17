package gitlab

import (
	"testing"

	"github.com/sohooo/moebius/internal/cli"
)

func TestResolveTargetPrefersExplicitGitLabToken(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")
	t.Setenv("CI_JOB_TOKEN", "job-token")
	t.Setenv("GITLAB_TOKEN", "private-token")

	target, err := ResolveTarget(cli.Options{})
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.Token != "private-token" {
		t.Fatalf("expected explicit token, got %q", target.Token)
	}
	if target.TokenKind != TokenKindPrivate {
		t.Fatalf("expected private token kind, got %q", target.TokenKind)
	}
	if target.TokenSource != "GITLAB_TOKEN" {
		t.Fatalf("expected GITLAB_TOKEN source, got %q", target.TokenSource)
	}
}

func TestResolveTargetFallsBackToJobToken(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")
	t.Setenv("CI_JOB_TOKEN", "job-token")
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITLAB_PRIVATE_TOKEN", "")
	t.Setenv("GITLAB_API_TOKEN", "")

	target, err := ResolveTarget(cli.Options{})
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.Token != "job-token" {
		t.Fatalf("expected job token, got %q", target.Token)
	}
	if target.TokenKind != TokenKindJob {
		t.Fatalf("expected job token kind, got %q", target.TokenKind)
	}
	if target.TokenSource != "CI_JOB_TOKEN" {
		t.Fatalf("expected CI_JOB_TOKEN source, got %q", target.TokenSource)
	}
}
