package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepoConfigFailsWhenMissing(t *testing.T) {
	_, err := LoadRepoConfig(t.TempDir())
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !strings.Contains(err.Error(), "missing required config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRepoConfigAppliesDefaults(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  clusters_dir: custom-clusters\n")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.ClustersDir != "custom-clusters" {
		t.Fatalf("unexpected clusters_dir: %q", cfg.Layout.ClustersDir)
	}
	if cfg.Layout.Apps.File != "apps.yaml" {
		t.Fatalf("expected default apps file, got %q", cfg.Layout.Apps.File)
	}
	if cfg.Layout.Overrides.FallbackPath != "overrides/{name}.yaml" {
		t.Fatalf("expected default fallback path, got %q", cfg.Layout.Overrides.FallbackPath)
	}
}

func TestLoadRepoConfigRejectsUnknownPlaceholder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  overrides:\n    path: overrides/{team}/{name}.yaml\n")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected invalid placeholder error")
	}
	if !strings.Contains(err.Error(), `unknown placeholder "team"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRepoConfigRejectsUnknownRequiredField(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  apps:\n    required:\n      - release_name\n")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected invalid required field error")
	}
	if !strings.Contains(err.Error(), `unknown canonical field "release_name"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesUsesConfiguredFieldNames(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Fields = AppsFieldsConfig{
		Name:      "release_name",
		Namespace: "target_namespace",
		Project:   "argocd_project",
		Chart:     "chart_ref",
		Version:   "chart_version",
	}

	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.File), `- release_name: hello-world
  target_namespace: hello-world
  argocd_project: test
  chart_ref: charts/hello-world
  chart_version: 0.1.0
`)

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected one release, got %d", len(releases))
	}
	if releases[0].Name != "hello-world" || releases[0].Project != "test" || releases[0].Chart != "charts/hello-world" {
		t.Fatalf("unexpected normalized release: %#v", releases[0])
	}
}

func TestLoadReleasesValidatesRequiredFields(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.File), `- name: hello-world
  chart: charts/hello-world
`)

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing namespace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesRejectsNonListApps(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.File), "apps:\n  - name: hello-world\n")

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveOverridePathUsesPrimaryAndFallback(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}

	clusterDir := ClusterDir(root, layout, cluster)
	if err := os.MkdirAll(filepath.Join(clusterDir, "overrides", "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	primary := filepath.Join(clusterDir, "overrides", "test", "hello-world.yaml")
	writeFile(t, primary, "replicaCount: 3\n")

	got := ResolveOverridePath(root, layout, cluster, release)
	if got != primary {
		t.Fatalf("expected primary override path, got %q", got)
	}

	if err := os.Remove(primary); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	fallback := filepath.Join(clusterDir, "overrides", "hello-world.yaml")
	writeFile(t, fallback, "replicaCount: 2\n")

	got = ResolveOverridePath(root, layout, cluster, release)
	if got != fallback {
		t.Fatalf("expected fallback override path, got %q", got)
	}
}

func TestResolveOverridePathHonorsCustomPatterns(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.ClustersDir = "environments"
	layout.Apps.File = "releases.yaml"
	layout.Overrides.Path = "values/{cluster}/{project}/{name}.yaml"
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}

	want := filepath.Join(root, "environments", "kube-bravo", "values", "kube-bravo", "test", "hello-world.yaml")
	writeFile(t, want, "replicaCount: 3\n")
	got := ResolveOverridePath(root, layout, cluster, release)
	if got != want {
		t.Fatalf("unexpected override path: got %q want %q", got, want)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
