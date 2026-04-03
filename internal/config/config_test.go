package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReleasesAllowsLegacyReleaseWithoutProject(t *testing.T) {
	root := t.TempDir()
	clusterDir := filepath.Join(root, "clusters", "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	apps := `- name: hello-world
  namespace: hello-world
  chart: charts/hello-world
`
	if err := os.WriteFile(filepath.Join(clusterDir, "apps.yaml"), []byte(apps), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	releases, err := LoadReleases(root, "clusters", "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected one release, got %d", len(releases))
	}
	if releases[0].Project != "" {
		t.Fatalf("expected empty legacy project, got %q", releases[0].Project)
	}
}

func TestOverridePathIncludesProject(t *testing.T) {
	got := OverridePath("/repo", "clusters", "kube-bravo", "test", "hello-world")
	want := filepath.Join("/repo", "clusters", "kube-bravo", "overrides", "test", "hello-world.yaml")
	if got != want {
		t.Fatalf("unexpected override path: got %q want %q", got, want)
	}
}

func TestOverridePathFallsBackToLegacyLayout(t *testing.T) {
	got := OverridePath("/repo", "clusters", "kube-bravo", "", "hello-world")
	want := filepath.Join("/repo", "clusters", "kube-bravo", "overrides", "hello-world.yaml")
	if got != want {
		t.Fatalf("unexpected legacy override path: got %q want %q", got, want)
	}
}
