package helmrender

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyLocateChartError_MissingVersion(t *testing.T) {
	err := classifyLocateChartError(
		"oci://internal.oci.repo/helm-int/argo-cd",
		"",
		"1.2.3",
		errors.New("manifest unknown: requested version not found"),
	)

	versionErr, ok := err.(*MissingVersionError)
	if !ok {
		t.Fatalf("expected MissingVersionError, got %T", err)
	}
	if versionErr.TargetRevision != "1.2.3" {
		t.Fatalf("unexpected target revision %q", versionErr.TargetRevision)
	}
	if versionErr.ChartRef != "oci://internal.oci.repo/helm-int/argo-cd" {
		t.Fatalf("unexpected chart ref %q", versionErr.ChartRef)
	}
	if !IsMissingVersionError(err) {
		t.Fatalf("expected IsMissingVersionError to detect wrapped error")
	}
}

func TestClassifyLocateChartError_GenericError(t *testing.T) {
	original := errors.New("failed to fetch chart metadata")
	err := classifyLocateChartError("argo-cd", "https://charts.example.com", "1.2.3", original)
	if err != original {
		t.Fatalf("expected generic error to pass through unchanged")
	}
	if IsMissingVersionError(err) {
		t.Fatalf("did not expect missing version classification")
	}
}

func TestMissingVersionErrorFormatsAndUnwraps(t *testing.T) {
	underlying := errors.New("not found")
	err := &MissingVersionError{ChartRef: "app", TargetRevision: "1.2.3", UnderlyingError: underlying}
	if !strings.Contains(err.Error(), `chart version "1.2.3" unavailable for app`) {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("expected errors.Is to unwrap underlying error")
	}
	if got := (*MissingVersionError)(nil).Error(); got != "" {
		t.Fatalf("nil MissingVersionError returned %q", got)
	}
}

func TestRenderRejectsRemoteChartsWithoutTargetRevision(t *testing.T) {
	root := t.TempDir()
	renderer := New(filepath.Join(root, ".cache"))

	_, err := renderer.Render(root, "argo-cd", "https://charts.example.test", "", "argo", "argocd", "")
	if err == nil || !strings.Contains(err.Error(), `remote chart "argo-cd" requires targetRevision`) {
		t.Fatalf("expected remote chart targetRevision error, got %v", err)
	}
	_, err = renderer.Render(root, "oci://registry.example.test/charts/argo-cd", "", "", "argo", "argocd", "")
	if err == nil || !strings.Contains(err.Error(), `oci chart "oci://registry.example.test/charts/argo-cd" requires targetRevision`) {
		t.Fatalf("expected oci chart targetRevision error, got %v", err)
	}
}

func TestNewStoresCacheDirForRepositorySettings(t *testing.T) {
	root := t.TempDir()
	renderer := New(filepath.Join(root, "cache"))
	if renderer.cacheDir != filepath.Join(root, "cache") {
		t.Fatalf("unexpected cache dir %q", renderer.cacheDir)
	}
}

func TestRenderLocalChartUsesReleaseNamespaceAndOverrides(t *testing.T) {
	root := t.TempDir()
	writeChart(t, filepath.Join(root, "charts/app"), map[string]string{
		"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
data:
  message: {{ .Values.message | quote }}
`,
		"templates/NOTES.txt": `this should not be rendered`,
	}, "message: default\n")
	override := filepath.Join(root, "clusters/kube/overrides/app.yaml")
	writeFile(t, override, "message: overridden\n")

	rendered, err := New(filepath.Join(root, ".cache")).Render(root, "charts/app", "", "", "hello", "demo", override)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, needle := range []string{
		"name: hello",
		"namespace: demo",
		`message: "overridden"`,
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("expected rendered output to contain %q:\n%s", needle, rendered)
		}
	}
	if strings.Contains(rendered, "NOTES") || strings.Contains(rendered, "this should not be rendered") {
		t.Fatalf("expected NOTES.txt to be skipped:\n%s", rendered)
	}
}

func TestRenderLocalChartIgnoresMissingOverridePath(t *testing.T) {
	root := t.TempDir()
	writeChart(t, filepath.Join(root, "charts/app"), map[string]string{
		"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
data:
  message: {{ .Values.message | quote }}
`,
	}, "message: default\n")

	rendered, err := New(filepath.Join(root, ".cache")).Render(root, "charts/app", "", "", "hello", "demo", filepath.Join(root, "missing.yaml"))
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(rendered, `message: "default"`) {
		t.Fatalf("expected default value with missing override path:\n%s", rendered)
	}
}

func TestRenderLocalChartReportsInvalidOverrideYAML(t *testing.T) {
	root := t.TempDir()
	writeChart(t, filepath.Join(root, "charts/app"), map[string]string{
		"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`,
	}, "")
	override := filepath.Join(root, "bad-values.yaml")
	writeFile(t, override, "message: [unterminated\n")

	_, err := New(filepath.Join(root, ".cache")).Render(root, "charts/app", "", "", "hello", "demo", override)
	if err == nil || !strings.Contains(err.Error(), "read overrides") {
		t.Fatalf("expected read overrides error, got %v", err)
	}
}

func TestRenderLocalChartReturnsTemplateError(t *testing.T) {
	root := t.TempDir()
	writeChart(t, filepath.Join(root, "charts/app"), map[string]string{
		"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ required "message required" .Values.message }}
`,
	}, "")

	_, err := New(filepath.Join(root, ".cache")).Render(root, "charts/app", "", "", "hello", "demo", "")
	if err == nil || !strings.Contains(err.Error(), "message required") {
		t.Fatalf("expected template error, got %v", err)
	}
}

func TestRenderLocalChartOrdersManifestsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeChart(t, filepath.Join(root, "charts/app"), map[string]string{
		"templates/z-configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
`,
		"templates/a-namespace.yaml": `apiVersion: v1
kind: Namespace
metadata:
  name: demo
`,
	}, "")

	rendered, err := New(filepath.Join(root, ".cache")).Render(root, "charts/app", "", "", "hello", "demo", "")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	namespaceIndex := strings.Index(rendered, "kind: Namespace")
	configMapIndex := strings.Index(rendered, "kind: ConfigMap")
	if namespaceIndex == -1 || configMapIndex == -1 || namespaceIndex > configMapIndex {
		t.Fatalf("expected Namespace before ConfigMap:\n%s", rendered)
	}
}

func writeChart(t *testing.T, dir string, templates map[string]string, values string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "Chart.yaml"), `apiVersion: v2
name: app
version: 0.1.0
`)
	if values != "" {
		writeFile(t, filepath.Join(dir, "values.yaml"), values)
	}
	for name, content := range templates {
		writeFile(t, filepath.Join(dir, name), content)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
