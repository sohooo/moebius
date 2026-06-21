package gitlab

import (
	"strings"
	"testing"

	"github.com/sohooo/moebius/internal/cli"
)

func TestResolveTargetPrefersExplicitGitLabToken(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")
	t.Setenv("CI_JOB_TOKEN", "job-token")
	t.Setenv("GITLAB_API_TOKEN", "api-token")
	t.Setenv("GITLAB_TOKEN", "private-token")

	target, err := ResolveTarget(cli.Options{})
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.Token != "api-token" {
		t.Fatalf("expected explicit token, got %q", target.Token)
	}
	if target.TokenKind != TokenKindPrivate {
		t.Fatalf("expected private token kind, got %q", target.TokenKind)
	}
	if target.TokenSource != "GITLAB_API_TOKEN" {
		t.Fatalf("expected GITLAB_API_TOKEN source, got %q", target.TokenSource)
	}
}

func TestResolveTargetPrefersCLIOverEnvironmentTokens(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")
	t.Setenv("GITLAB_API_TOKEN", "api-token")

	target, err := ResolveTarget(cli.Options{GitLabToken: "cli-token"})
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.Token != "cli-token" || target.TokenSource != "--gitlab-token" {
		t.Fatalf("expected CLI token to win, got token=%q source=%q", target.Token, target.TokenSource)
	}
}

func TestResolveTargetUsesLegacyAliasWhenAPITokenMissing(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")
	t.Setenv("GITLAB_TOKEN", "legacy-token")

	target, err := ResolveTarget(cli.Options{})
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.Token != "legacy-token" || target.TokenSource != "GITLAB_TOKEN" {
		t.Fatalf("expected legacy token alias, got token=%q source=%q", target.Token, target.TokenSource)
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

func TestResolveTargetMissingTokenMentionsPreferredToken(t *testing.T) {
	t.Setenv("CI_API_V4_URL", "https://gitlab.example/api/v4")
	t.Setenv("CI_PROJECT_ID", "42")
	t.Setenv("CI_MERGE_REQUEST_IID", "7")

	_, err := ResolveTarget(cli.Options{})
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if !strings.Contains(err.Error(), "GITLAB_API_TOKEN") || strings.Contains(err.Error(), "CI_JOB_TOKEN") {
		t.Fatalf("expected preferred API token guidance, got %v", err)
	}
}
