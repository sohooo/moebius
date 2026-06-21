package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitRendered_UsesStableResourceKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
  namespace: demo
spec:
  replicas: 2
---
apiVersion: v1
kind: Namespace
metadata:
  name: kube-bravo
`

	out, _, err := SplitRendered(rendered, dir, SplitOptions{})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(out))
	}
	if out[0].Key != "Deployment--demo--hello-world" {
		t.Fatalf("unexpected namespaced key: %q", out[0].Key)
	}
	if out[1].Key != "Namespace--cluster--kube-bravo" {
		t.Fatalf("unexpected cluster-scoped key: %q", out[1].Key)
	}
}

func TestSplitRendered_UniquifiesDuplicateResourceKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: shared
  namespace: demo
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: shared
  namespace: demo
`

	out, _, err := SplitRendered(rendered, dir, SplitOptions{})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(out))
	}
	if out[0].Identity != out[1].Identity {
		t.Fatalf("expected duplicate resources to share identity, got %q and %q", out[0].Identity, out[1].Identity)
	}
	if out[0].Key == out[1].Key {
		t.Fatalf("expected duplicate resources to use distinct keys")
	}
}

func TestSplitRendered_WarnLastWinsForDuplicateMappingKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `apiVersion: v1
kind: Service
metadata:
  name: demo
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/scrape: "false"
`

	out, warnings, err := SplitRendered(rendered, dir, SplitOptions{DuplicateKeyMode: DuplicateKeyModeWarnLastWins})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one resource, got %d", len(out))
	}
	metadata := out[0].Value.(map[string]interface{})["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	if annotations["prometheus.io/scrape"] != "false" {
		t.Fatalf("expected last value to win, got %#v", annotations["prometheus.io/scrape"])
	}
	if len(warnings) == 0 {
		t.Fatal("expected duplicate-key warning")
	}
}

func TestSplitRendered_ErrorsOnDuplicateMappingKeysByDefault(t *testing.T) {
	dir := t.TempDir()
	rendered := `apiVersion: v1
kind: Service
metadata:
  name: demo
  labels:
    app: one
    app: two
`

	_, _, err := SplitRendered(rendered, dir, SplitOptions{DuplicateKeyMode: DuplicateKeyModeError})
	if err == nil || !strings.Contains(err.Error(), "mapping key") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestSplitRendered_SkipsZeroDocumentsAndHandlesNullMetadata(t *testing.T) {
	dir := t.TempDir()
	rendered := `
---
null
---
apiVersion: v1
kind: ConfigMap
metadata: null
`

	out, _, err := SplitRendered(rendered, dir, SplitOptions{})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one resource, got %d", len(out))
	}
	if got, want := out[0].Key, "ConfigMap--cluster--doc-0"; got != want {
		t.Fatalf("unexpected fallback key %q want %q", got, want)
	}
}

func TestLoadFileAndLoadDir(t *testing.T) {
	dir := t.TempDir()
	writeResourceFile(t, filepath.Join(dir, "b.yaml"), `apiVersion: v1
kind: Service
metadata:
  name: svc
`)
	writeResourceFile(t, filepath.Join(dir, "a.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
  namespace: demo
`)
	writeResourceFile(t, filepath.Join(dir, "ignored.txt"), `not yaml`)

	fileResource, err := LoadFile(filepath.Join(dir, "a.yaml"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if fileResource.Kind != "ConfigMap" || fileResource.Namespace != "demo" {
		t.Fatalf("unexpected file resource: %#v", fileResource)
	}
	resources, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if got, want := len(resources), 2; got != want {
		t.Fatalf("unexpected resource count %d want %d", got, want)
	}
	if _, ok := resources["a"]; !ok {
		t.Fatalf("expected ConfigMap key in %#v", resources)
	}
	if _, ok := resources["b"]; !ok {
		t.Fatalf("expected Service key in %#v", resources)
	}
}

func TestLoadFileReportsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	writeResourceFile(t, path, "apiVersion: [unterminated\n")

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("expected yaml error, got %v", err)
	}
}

func writeResourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
