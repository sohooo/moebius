package moebius_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDocsUseGenericVersionExamples(t *testing.T) {
	files := []string{
		"README.md",
		"docs/gitlab-ci.md",
		"docs/releases.md",
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`go install github\.com/sohooo/moebius/cmd/mobius@v0\.\d+\.\d+`),
		regexp.MustCompile(`image:\s*ghcr\.io/sohooo/moebius:v0\.\d+\.\d+`),
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		content := string(data)
		for _, pattern := range patterns {
			if match := pattern.FindString(content); match != "" {
				t.Fatalf("%s contains pinned version example %q; use vX.Y.Z instead", file, match)
			}
		}
	}
}

func TestReleaseWorkflowSmokeTestCoversContainerContract(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	content := string(data)
	required := []string{
		"FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true",
		"docker run --rm mobius:test version",
		"docker run --rm mobius:test diff --help >/dev/null",
		"docker run --rm --entrypoint sh mobius:test -ec 'command -v git && command -v mobius && command -v møbius'",
	}
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			t.Fatalf("release workflow is missing %q", needle)
		}
	}
}
