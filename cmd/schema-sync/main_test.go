package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSchemas_FromLocalCRDYAML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemasources.yaml"), `
sources:
  - component: demo
    version: v1
    source_type: file
    paths:
      - testdata/crd.yaml
`)
	writeFile(t, filepath.Join(root, "testdata/crd.yaml"), `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
  versions:
    - name: v1
      served: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                size:
                  type: integer
`)

	manifest, err := loadManifest(filepath.Join(root, "schemasources.yaml"))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	generated, err := generateSchemas(root, manifest)
	if err != nil {
		t.Fatalf("generateSchemas: %v", err)
	}
	if len(generated) != 1 {
		t.Fatalf("expected one generated schema, got %d", len(generated))
	}
	if got, want := generated[0].RelativePath, "platform/demo/v1/example_com_v1_Widget.json"; got != want {
		t.Fatalf("unexpected path %q want %q", got, want)
	}
	if got, want := generated[0].CanonicalGVK, "example.com/v1/Widget"; got != want {
		t.Fatalf("unexpected key %q want %q", got, want)
	}
}

func TestGenerateSchemas_FromLocalJSONSchema(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemasources.yaml"), `
sources:
  - component: kubernetes
    version: v1
    source_type: file
    paths:
      - schemas/apps_v1_Deployment.json
`)
	writeFile(t, filepath.Join(root, "schemas/apps_v1_Deployment.json"), `{"type":"object","properties":{"spec":{"type":"object"}}}`)

	manifest, err := loadManifest(filepath.Join(root, "schemasources.yaml"))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	generated, err := generateSchemas(root, manifest)
	if err != nil {
		t.Fatalf("generateSchemas: %v", err)
	}
	if len(generated) != 1 {
		t.Fatalf("expected one generated schema, got %d", len(generated))
	}
	if got, want := generated[0].RelativePath, "kubernetes/v1/apps_v1_Deployment.json"; got != want {
		t.Fatalf("unexpected path %q want %q", got, want)
	}
	if got, want := generated[0].CanonicalGVK, "apps/v1/Deployment"; got != want {
		t.Fatalf("unexpected key %q want %q", got, want)
	}
}

func TestLoadManifest_AllowsURLSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemasources.yaml"), `
sources:
  - component: demo
    version: v1
    source_type: url
    urls:
      - https://schemas.example.invalid/demo.yaml
`)

	manifest, err := loadManifest(filepath.Join(root, "schemasources.yaml"))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if len(manifest.Sources) != 1 || manifest.Sources[0].SourceType != "url" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestVerifyGeneratedFiles_DetectsDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	schemaRoot := filepath.Join(root, "internal/validate/schemas")
	writeFile(t, filepath.Join(schemaRoot, "kubernetes/v1/apps_v1_Deployment.json"), `{"type":"object"}`)
	writeFile(t, filepath.Join(schemaRoot, "index.json"), `{"schemas":{}}`)

	generated := []generatedSchema{{
		RelativePath: "kubernetes/v1/apps_v1_Deployment.json",
		CanonicalGVK: "apps/v1/Deployment",
		Content:      []byte("{\n  \"type\": \"object\",\n  \"properties\": {}\n}\n"),
	}}
	index, err := buildGeneratedIndex(generated)
	if err != nil {
		t.Fatalf("buildGeneratedIndex: %v", err)
	}
	if err := verifyGeneratedFiles(schemaRoot, generated, index); err == nil {
		t.Fatal("expected drift error, got nil")
	}
}

func TestWriteGeneratedFilesAndVerify(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	schemaRoot := filepath.Join(root, "internal/validate/schemas")
	generated := []generatedSchema{
		{
			RelativePath: "platform/demo/v1/example.com_v1_Widget.json",
			CanonicalGVK: "example.com/v1/Widget",
			Content:      []byte("{\n  \"type\": \"object\"\n}\n"),
		},
	}
	index, err := buildGeneratedIndex(generated)
	if err != nil {
		t.Fatalf("buildGeneratedIndex: %v", err)
	}
	if err := writeGeneratedFiles(schemaRoot, generated, index); err != nil {
		t.Fatalf("writeGeneratedFiles: %v", err)
	}
	if err := verifyGeneratedFiles(schemaRoot, generated, index); err != nil {
		t.Fatalf("verifyGeneratedFiles: %v", err)
	}

	indexBytes, err := os.ReadFile(filepath.Join(schemaRoot, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(indexBytes), "example.com/v1/Widget") {
		t.Fatalf("index missing schema key: %s", string(indexBytes))
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
