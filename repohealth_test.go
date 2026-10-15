package moebius_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestWorkflowSecurityBoundaries(t *testing.T) {
	paths, err := filepath.Glob(".github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	pinned := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var workflow struct {
			Permissions map[string]string `yaml:"permissions"`
			Jobs        map[string]struct {
				Needs       any               `yaml:"needs"`
				Permissions map[string]string `yaml:"permissions"`
				Steps       []struct {
					Uses string `yaml:"uses"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		for permission, access := range workflow.Permissions {
			if access == "write" {
				t.Errorf("%s grants global write permission: %s", path, permission)
			}
		}
		for name, job := range workflow.Jobs {
			for _, step := range job.Steps {
				if step.Uses != "" && !strings.HasPrefix(step.Uses, "./") && !pinned.MatchString(step.Uses) {
					t.Errorf("%s/%s has unpinned action %s", path, name, step.Uses)
				}
			}
			if path == ".github/workflows/ci.yml" {
				for permission, access := range job.Permissions {
					if access == "write" {
						t.Errorf("PR job has write permission: %s", permission)
					}
				}
			}
		}
		if path == ".github/workflows/release.yml" {
			if workflow.Jobs["container"].Needs != "verify" {
				t.Error("containers must depend on verification")
			}
			needs, ok := workflow.Jobs["publish"].Needs.([]any)
			if !ok || len(needs) != 2 || needs[0] != "verify" || needs[1] != "container" {
				t.Error("release publication must wait for binary and container gates")
			}
		}
	}
}

func TestUpdateCooldownRequiresReview(t *testing.T) {
	data, err := os.ReadFile("renovate.json")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"minimumReleaseAge":          "7 days",
		"minimumReleaseAgeBehaviour": "timestamp-required",
		"internalChecksFilter":       "strict",
		"automerge":                  false,
	} {
		if config[key] != want {
			t.Errorf("%s = %v, want %v", key, config[key], want)
		}
	}
}

func TestDockerfileKeepsCIImageRuntimeContract(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	content := string(data)
	required := []string{
		"golang:",
		"FROM alpine:",
		"RUN apk add --no-cache ca-certificates git",
		"COPY --from=build /out/mobius /usr/local/bin/mobius",
		"RUN ln -s /usr/local/bin/mobius /usr/local/bin/møbius",
		`ENTRYPOINT ["/usr/local/bin/møbius"]`,
		`CMD ["diff"]`,
	}
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			t.Fatalf("Dockerfile is missing %q", needle)
		}
	}
	if strings.Contains(content, "FROM scratch") || strings.Contains(content, "distroless") {
		t.Fatal("Dockerfile must keep a shell-capable Alpine runtime for GitLab CI scripts")
	}
}
