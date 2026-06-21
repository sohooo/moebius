package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	generated, _, err := generateSchemas(root, manifest, schemaLock{}, false)
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
	generated, _, err := generateSchemas(root, manifest, schemaLock{}, false)
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

func TestLoadManifest_RequiresRepoForLatestURLSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemasources.yaml"), `
sources:
  - component: demo
    version: latest
    source_type: url
    urls:
      - https://schemas.example.invalid/{version}/demo.yaml
`)

	_, err := loadManifest(filepath.Join(root, "schemasources.yaml"))
	if err == nil {
		t.Fatal("expected latest URL source without repo to be rejected")
	}
}

func TestLatestPlainSemverTagIgnoresSchemaVariantTags(t *testing.T) {
	t.Parallel()

	got, ok := latestPlainSemverTag([]githubTag{
		{Name: "v1.15-v1.18"},
		{Name: "v1.36.0-standalone-strict"},
		{Name: "v1.35.4"},
		{Name: "v1.36.0-rc.0"},
		{Name: "v1.36.0"},
	})
	if !ok {
		t.Fatal("expected a latest tag")
	}
	if want := "v1.36.0"; got != want {
		t.Fatalf("unexpected latest tag %q want %q", got, want)
	}
}

func TestLatestPlainSemverTagAcceptsDirectoryNames(t *testing.T) {
	t.Parallel()

	items := []githubContent{
		{Name: "master-standalone-strict", Type: "dir"},
		{Name: "v1.35.4", Type: "dir"},
		{Name: "v1.36.0", Type: "dir"},
	}
	tags := make([]githubTag, 0, len(items))
	for _, item := range items {
		if item.Type == "dir" {
			tags = append(tags, githubTag{Name: item.Name})
		}
	}
	got, ok := latestPlainSemverTag(tags)
	if !ok {
		t.Fatal("expected a latest tag")
	}
	if want := "v1.36.0"; got != want {
		t.Fatalf("unexpected latest tag %q want %q", got, want)
	}
}

func TestLoadManifest_AllowsGitHubReleaseSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemasources.yaml"), `
sources:
  - component: demo
    version: latest
    source_type: github_release
    repo: example/project
    asset_name: crds.yaml
`)

	manifest, err := loadManifest(filepath.Join(root, "schemasources.yaml"))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if len(manifest.Sources) != 1 || manifest.Sources[0].SourceType != "github_release" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestResolvePath_PreservesAbsolutePaths(t *testing.T) {
	t.Parallel()

	if got, want := resolvePath("/repo", "/tmp/schema.yaml"), "/tmp/schema.yaml"; got != want {
		t.Fatalf("unexpected path %q want %q", got, want)
	}
}

func TestVerifyLockFileDetectsDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lockPath := filepath.Join(root, "schemas.lock.yaml")
	if err := writeLockFile(lockPath, schemaLock{
		Sources: []lockedSource{{
			Component:       "demo",
			SourceType:      "github_release",
			Version:         "latest",
			ResolvedVersion: "v1.0.0",
			Repo:            "example/project",
		}},
	}); err != nil {
		t.Fatalf("writeLockFile: %v", err)
	}

	err := verifyLockFile(lockPath, schemaLock{
		Sources: []lockedSource{{
			Component:       "demo",
			SourceType:      "github_release",
			Version:         "latest",
			ResolvedVersion: "v1.0.1",
			Repo:            "example/project",
		}},
	})
	if err == nil {
		t.Fatal("expected lock drift error, got nil")
	}
}

func TestLoadLockAndFindLockedSource(t *testing.T) {
	root := t.TempDir()
	missing, err := loadLock(filepath.Join(root, "missing.yaml"))
	if err != nil {
		t.Fatalf("load missing lock: %v", err)
	}
	if len(missing.Sources) != 0 {
		t.Fatalf("expected empty missing lock, got %#v", missing)
	}

	lockPath := filepath.Join(root, "schemas.lock.yaml")
	if err := writeLockFile(lockPath, schemaLock{Sources: []lockedSource{{
		Component:       "demo",
		SourceType:      "github_release",
		Version:         "latest",
		ResolvedVersion: "v1.2.3",
		Repo:            "example/demo",
		AssetName:       "crds.yaml",
	}}}); err != nil {
		t.Fatalf("writeLockFile: %v", err)
	}
	loaded, err := loadLock(lockPath)
	if err != nil {
		t.Fatalf("loadLock: %v", err)
	}
	locked, ok := findLockedSource(loaded, schemaSource{
		Component:  "demo",
		SourceType: "github_release",
		Version:    "latest",
		Repo:       "example/demo",
		AssetName:  "crds.yaml",
	})
	if !ok {
		t.Fatalf("expected locked source in %#v", loaded)
	}
	if got, want := locked.ResolvedVersion, "v1.2.3"; got != want {
		t.Fatalf("unexpected resolved version %q want %q", got, want)
	}
	if _, ok := findLockedSource(loaded, schemaSource{Component: "other", SourceType: "github_release", Version: "latest"}); ok {
		t.Fatal("did not expect mismatched source to resolve")
	}
}

func TestLoadSourceDocumentsVerifyRequiresLockEntry(t *testing.T) {
	_, _, err := loadSourceDocuments("", schemaSource{
		Component:  "demo",
		SourceType: "url",
		Version:    "latest",
		Repo:       "example/demo",
		URLs:       []string{"https://schemas.example.test/{version}/crd.yaml"},
	}, schemaLock{}, true)
	if err == nil || !strings.Contains(err.Error(), "missing lock entry") {
		t.Fatalf("expected missing lock entry error, got %v", err)
	}

	_, _, err = loadSourceDocuments("", schemaSource{
		Component:  "demo",
		SourceType: "github_release",
		Version:    "latest",
		Repo:       "example/demo",
		Paths:      []string{"demo-{version_nov}/crds/*.yaml"},
	}, schemaLock{}, true)
	if err == nil || !strings.Contains(err.Error(), "missing lock entry") {
		t.Fatalf("expected missing lock entry error, got %v", err)
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

func TestLoadURLSourceDocumentsForSourceUsesLockedVersionInVerifyMode(t *testing.T) {
	baseURL := "https://schemas.example.test"
	withHTTPHandler(t, func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.Path, "/v1.2.3/crd.yaml"; got != want {
			t.Fatalf("unexpected request path %q want %q", got, want)
		}
		return httpResponse(http.StatusOK, `apiVersion: apiextensions.k8s.io/v1`), nil
	})

	source := schemaSource{
		Component:  "demo",
		SourceType: "url",
		Version:    "latest",
		Repo:       "example/demo",
		URLs:       []string{baseURL + "/{version}/crd.yaml"},
	}
	docs, locked, err := loadURLSourceDocumentsForSource(source, schemaLock{Sources: []lockedSource{{
		Component:       "demo",
		SourceType:      "url",
		Version:         "latest",
		ResolvedVersion: "v1.2.3",
		Repo:            "example/demo",
	}}}, true)
	if err != nil {
		t.Fatalf("loadURLSourceDocumentsForSource: %v", err)
	}
	if len(docs) != 1 || !strings.Contains(string(docs[0].Data), "apiextensions.k8s.io/v1") {
		t.Fatalf("unexpected docs: %#v", docs)
	}
	if got, want := locked.ResolvedVersion, "v1.2.3"; got != want {
		t.Fatalf("unexpected resolved version %q want %q", got, want)
	}
}

func TestLoadURLSourceDocumentsReportsHTTPStatus(t *testing.T) {
	withHTTPHandler(t, func(r *http.Request) (*http.Response, error) {
		return httpResponse(http.StatusNotFound, "missing"), nil
	})

	_, err := loadURLSourceDocuments([]string{"https://schemas.example.test/missing.yaml"})
	if err == nil || !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("expected non-2xx status error, got %v", err)
	}
}

func TestLoadGitHubReleaseSourceDocumentsDownloadsAssetFromConfiguredBase(t *testing.T) {
	releaseBase := "https://github.example.test"
	withHTTPHandler(t, func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.Path, "/example/demo/releases/download/v1.2.3/crds.yaml"; got != want {
			t.Fatalf("unexpected asset path %q want %q", got, want)
		}
		return httpResponse(http.StatusOK, `kind: CustomResourceDefinition`), nil
	})
	withGitHubBases(t, "", releaseBase, "")

	source := schemaSource{
		Component:  "demo",
		SourceType: "github_release",
		Version:    "latest",
		Repo:       "example/demo",
		AssetName:  "crds.yaml",
	}
	docs, locked, err := loadGitHubReleaseSourceDocuments("", source, schemaLock{Sources: []lockedSource{{
		Component:       "demo",
		SourceType:      "github_release",
		Version:         "latest",
		ResolvedVersion: "v1.2.3",
		Repo:            "example/demo",
		AssetName:       "crds.yaml",
	}}}, true)
	if err != nil {
		t.Fatalf("loadGitHubReleaseSourceDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Name != releaseBase+"/example/demo/releases/download/v1.2.3/crds.yaml" {
		t.Fatalf("unexpected docs: %#v", docs)
	}
	if got, want := locked.ResolvedVersion, "v1.2.3"; got != want {
		t.Fatalf("unexpected resolved version %q want %q", got, want)
	}
}

func TestLoadGitHubReleaseSourceDocumentsReadsMatchingArchivePaths(t *testing.T) {
	archive := tarGz(t, map[string]string{
		"demo-1.2.3/config/crds/widgets.yaml": `kind: CustomResourceDefinition`,
		"demo-1.2.3/config/ignored/skip.yaml": `kind: CustomResourceDefinition`,
	})
	archiveBase := "https://github.example.test"
	withHTTPHandler(t, func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.Path, "/example/demo/archive/refs/tags/v1.2.3.tar.gz"; got != want {
			t.Fatalf("unexpected archive path %q want %q", got, want)
		}
		return httpResponseBytes(http.StatusOK, archive), nil
	})
	withGitHubBases(t, "", "", archiveBase)

	source := schemaSource{
		Component:  "demo",
		SourceType: "github_release",
		Version:    "latest",
		Repo:       "example/demo",
		Paths:      []string{"demo-{version_nov}/config/crds/*.yaml"},
	}
	docs, _, err := loadGitHubReleaseSourceDocuments("", source, schemaLock{Sources: []lockedSource{{
		Component:       "demo",
		SourceType:      "github_release",
		Version:         "latest",
		ResolvedVersion: "v1.2.3",
		Repo:            "example/demo",
	}}}, true)
	if err != nil {
		t.Fatalf("loadGitHubReleaseSourceDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Name != "demo-1.2.3/config/crds/widgets.yaml" {
		t.Fatalf("unexpected archive docs: %#v", docs)
	}
}

func TestResolveLatestGitHubReleaseFallbacksToTagsAndDirectories(t *testing.T) {
	requests := map[string]int{}
	apiBase := "https://api.github.example.test"
	withHTTPHandler(t, func(r *http.Request) (*http.Response, error) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/repos/example/tags/releases/latest":
			return httpResponse(http.StatusNotFound, "not found"), nil
		case "/repos/example/tags/tags":
			return jsonHTTPResponse(t, []githubTag{{Name: "v1.0.0"}, {Name: "v1.4.2"}, {Name: "v1.4.2-rc.1"}}), nil
		case "/repos/example/dirs/releases/latest":
			return httpResponse(http.StatusNotFound, "not found"), nil
		case "/repos/example/dirs/tags":
			return jsonHTTPResponse(t, []githubTag{{Name: "not-semver"}}), nil
		case "/repos/example/dirs/contents":
			return jsonHTTPResponse(t, []githubContent{{Name: "v1.2.0", Type: "dir"}, {Name: "v1.3.0", Type: "dir"}, {Name: "README.md", Type: "file"}}), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
			return nil, nil
		}
	})
	withGitHubBases(t, apiBase, "", "")

	got, err := resolveLatestGitHubRelease("example/tags")
	if err != nil {
		t.Fatalf("resolveLatestGitHubRelease tags: %v", err)
	}
	if got != "v1.4.2" {
		t.Fatalf("unexpected tag fallback version %q", got)
	}
	got, err = resolveLatestGitHubRelease("example/dirs")
	if err != nil {
		t.Fatalf("resolveLatestGitHubRelease dirs: %v", err)
	}
	if got != "v1.3.0" {
		t.Fatalf("unexpected directory fallback version %q", got)
	}
	if requests["/repos/example/tags/releases/latest"] != 1 || requests["/repos/example/dirs/contents"] != 1 {
		t.Fatalf("unexpected request counts: %#v", requests)
	}
}

func TestExpandPatternsAndArchivePatternMatching(t *testing.T) {
	got := expandPatterns([]string{
		"repo-{version}/crds/*.yaml",
		"repo-{version_nov}/schemas/schema.json",
	}, "v1.2.3")
	want := []string{"repo-v1.2.3/crds/*.yaml", "repo-1.2.3/schemas/schema.json"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected expanded patterns: got %v want %v", got, want)
	}
	if !matchesAnyArchivePattern("repo-v1.2.3/crds/policy.yaml", []string{"repo-v1.2.3/crds/*.yaml"}) {
		t.Fatal("expected direct wildcard path match")
	}
	if !matchesAnyArchivePattern("repo-v1.2.3/crds/nested/policy.yaml", []string{"repo-v1.2.3/crds/*"}) {
		t.Fatal("expected trailing wildcard prefix match")
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

func withGitHubBases(t *testing.T, apiBase, releaseBase, archiveBase string) {
	t.Helper()
	oldAPI, oldRelease, oldArchive := githubAPIBaseURL, githubReleaseDownloadBase, githubArchiveBase
	if apiBase != "" {
		githubAPIBaseURL = apiBase
	}
	if releaseBase != "" {
		githubReleaseDownloadBase = releaseBase
	}
	if archiveBase != "" {
		githubArchiveBase = archiveBase
	}
	t.Cleanup(func() {
		githubAPIBaseURL = oldAPI
		githubReleaseDownloadBase = oldRelease
		githubArchiveBase = oldArchive
	})
}

func withHTTPHandler(t *testing.T, handler func(*http.Request) (*http.Response, error)) {
	t.Helper()
	oldClient := schemaHTTPClient
	schemaHTTPClient = &http.Client{Transport: roundTripFunc(handler)}
	t.Cleanup(func() { schemaHTTPClient = oldClient })
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func httpResponse(status int, body string) *http.Response {
	return httpResponseBytes(status, []byte(body))
}

func httpResponseBytes(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func jsonHTTPResponse(t *testing.T, payload interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	return httpResponseBytes(http.StatusOK, data)
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		data := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
			t.Fatalf("write tar data: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
