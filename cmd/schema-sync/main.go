package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

type sourceManifest struct {
	Sources []schemaSource `yaml:"sources"`
}

type schemaSource struct {
	Component    string   `yaml:"component"`
	Version      string   `yaml:"version"`
	SourceType   string   `yaml:"source_type"`
	Paths        []string `yaml:"paths"`
	URLs         []string `yaml:"urls"`
	Repo         string   `yaml:"repo"`
	AssetName    string   `yaml:"asset_name"`
	IncludeKinds []string `yaml:"include_kinds"`
	Note         string   `yaml:"note"`
}

type schemaIndex struct {
	Schemas map[string]string `json:"schemas"`
}

type schemaLock struct {
	Sources []lockedSource `yaml:"sources"`
}

type lockedSource struct {
	Component       string `yaml:"component"`
	SourceType      string `yaml:"source_type"`
	Version         string `yaml:"version"`
	ResolvedVersion string `yaml:"resolved_version"`
	Repo            string `yaml:"repo,omitempty"`
	AssetName       string `yaml:"asset_name,omitempty"`
}

type generatedSchema struct {
	RelativePath string
	CanonicalGVK string
	Content      []byte
}

type sourceDocument struct {
	Name string
	Data []byte
}

func main() {
	var (
		verify       bool
		manifestPath string
		root         string
	)
	flag.BoolVar(&verify, "verify", false, "Verify that the generated schema bundle is up to date")
	flag.StringVar(&manifestPath, "manifest", "schemasources.yaml", "Path to the schema source manifest")
	flag.StringVar(&root, "root", ".", "Repository root")
	flag.Parse()

	manifest, err := loadManifest(resolvePath(root, manifestPath))
	if err != nil {
		fatal(err)
	}
	lockPath := filepath.Join(root, "schemas.lock.yaml")
	lock, err := loadLock(lockPath)
	if err != nil {
		fatal(err)
	}

	schemaRoot := filepath.Join(root, "internal/validate/schemas")
	generated, nextLock, err := generateSchemas(root, manifest, lock, verify)
	if err != nil {
		fatal(err)
	}
	index, err := buildGeneratedIndex(generated)
	if err != nil {
		fatal(err)
	}

	if verify {
		if err := verifyLockFile(lockPath, nextLock); err != nil {
			fatal(err)
		}
		if err := verifyGeneratedFiles(schemaRoot, generated, index); err != nil {
			fatal(err)
		}
		return
	}

	if err := writeLockFile(lockPath, nextLock); err != nil {
		fatal(err)
	}
	if err := writeGeneratedFiles(schemaRoot, generated, index); err != nil {
		fatal(err)
	}
}

func loadManifest(path string) (sourceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sourceManifest{}, err
	}
	var manifest sourceManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return sourceManifest{}, err
	}
	if len(manifest.Sources) == 0 {
		return sourceManifest{}, fmt.Errorf("manifest %s contains no sources", path)
	}
	for _, source := range manifest.Sources {
		if err := validateSource(source); err != nil {
			return sourceManifest{}, err
		}
	}
	return manifest, nil
}

func validateSource(source schemaSource) error {
	if strings.TrimSpace(source.Component) == "" {
		return errors.New("schema source is missing component")
	}
	if strings.TrimSpace(source.Version) == "" {
		return fmt.Errorf("schema source %s is missing version", source.Component)
	}
	switch source.SourceType {
	case "file":
		if len(source.Paths) == 0 {
			return fmt.Errorf("schema source %s is missing paths", source.Component)
		}
	case "url":
		if len(source.URLs) == 0 {
			return fmt.Errorf("schema source %s is missing urls", source.Component)
		}
	case "github_release":
		if strings.TrimSpace(source.Repo) == "" {
			return fmt.Errorf("schema source %s is missing repo", source.Component)
		}
		if len(source.Paths) == 0 && strings.TrimSpace(source.AssetName) == "" {
			return fmt.Errorf("schema source %s must set paths for source archive import or asset_name for release asset import", source.Component)
		}
	default:
		return fmt.Errorf("schema source %s has unsupported source_type %q", source.Component, source.SourceType)
	}
	return nil
}

func generateSchemas(root string, manifest sourceManifest, existingLock schemaLock, verify bool) ([]generatedSchema, schemaLock, error) {
	var generated []generatedSchema
	seen := map[string]generatedSchema{}
	nextLock := schemaLock{Sources: make([]lockedSource, 0, len(manifest.Sources))}

	for _, source := range manifest.Sources {
		documents, locked, err := loadSourceDocuments(root, source, existingLock, verify)
		if err != nil {
			return nil, schemaLock{}, err
		}
		nextLock.Sources = append(nextLock.Sources, locked)
		effectiveSource := source
		if locked.ResolvedVersion != "" && locked.ResolvedVersion != source.Version {
			effectiveSource.Version = locked.ResolvedVersion
		}
		for _, document := range documents {
			items, err := importDocument(effectiveSource, document)
			if err != nil {
				return nil, schemaLock{}, fmt.Errorf("%s: %w", document.Name, err)
			}
			for _, item := range items {
				if prior, ok := seen[item.RelativePath]; ok && !bytes.Equal(prior.Content, item.Content) {
					return nil, schemaLock{}, fmt.Errorf("schema path collision for %s", item.RelativePath)
				}
				seen[item.RelativePath] = item
			}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	generated = make([]generatedSchema, 0, len(keys))
	for _, key := range keys {
		generated = append(generated, seen[key])
	}
	return generated, nextLock, nil
}

func loadSourceDocuments(root string, source schemaSource, existingLock schemaLock, verify bool) ([]sourceDocument, lockedSource, error) {
	switch source.SourceType {
	case "file":
		docs, err := loadFileSourceDocuments(root, source.Paths)
		return docs, lockedSource{
			Component:       source.Component,
			SourceType:      source.SourceType,
			Version:         source.Version,
			ResolvedVersion: source.Version,
		}, err
	case "url":
		docs, err := loadURLSourceDocuments(source.URLs)
		return docs, lockedSource{
			Component:       source.Component,
			SourceType:      source.SourceType,
			Version:         source.Version,
			ResolvedVersion: source.Version,
		}, err
	case "github_release":
		return loadGitHubReleaseSourceDocuments(root, source, existingLock, verify)
	default:
		return nil, lockedSource{}, fmt.Errorf("unsupported source type %q", source.SourceType)
	}
}

func loadFileSourceDocuments(root string, patterns []string) ([]sourceDocument, error) {
	var documents []sourceDocument
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(resolvePath(root, pattern))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("schema source path %q matched no files", pattern)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			data, err := os.ReadFile(match)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(root, match)
			if err != nil {
				rel = match
			}
			documents = append(documents, sourceDocument{Name: filepath.ToSlash(rel), Data: data})
		}
	}
	return documents, nil
}

func loadGitHubReleaseSourceDocuments(root string, source schemaSource, existingLock schemaLock, verify bool) ([]sourceDocument, lockedSource, error) {
	resolvedVersion := source.Version
	if verify {
		locked, ok := findLockedSource(existingLock, source)
		if !ok {
			return nil, lockedSource{}, fmt.Errorf("missing lock entry for %s; run cmd/schema-sync", source.Component)
		}
		resolvedVersion = locked.ResolvedVersion
	} else if source.Version == "latest" {
		var err error
		resolvedVersion, err = resolveLatestGitHubRelease(source.Repo)
		if err != nil {
			return nil, lockedSource{}, err
		}
	}

	locked := lockedSource{
		Component:       source.Component,
		SourceType:      source.SourceType,
		Version:         source.Version,
		ResolvedVersion: resolvedVersion,
		Repo:            source.Repo,
		AssetName:       source.AssetName,
	}

	if source.AssetName != "" {
		downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", source.Repo, resolvedVersion, source.AssetName)
		docs, err := loadURLSourceDocuments([]string{downloadURL})
		return docs, locked, err
	}

	archiveURL := fmt.Sprintf("https://github.com/%s/archive/refs/tags/%s.tar.gz", source.Repo, resolvedVersion)
	documents, err := loadGitHubArchiveDocuments(root, archiveURL, expandPatterns(source.Paths, resolvedVersion))
	return documents, locked, err
}

func loadURLSourceDocuments(rawURLs []string) ([]sourceDocument, error) {
	client := &http.Client{}
	documents := make([]sourceDocument, 0, len(rawURLs))
	for _, rawURL := range rawURLs {
		resp, err := client.Get(rawURL)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("fetch %s: unexpected status %s", rawURL, resp.Status)
		}
		documents = append(documents, sourceDocument{Name: rawURL, Data: data})
	}
	return documents, nil
}

func loadGitHubArchiveDocuments(root string, rawURL string, patterns []string) ([]sourceDocument, error) {
	client := &http.Client{}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", rawURL, resp.Status)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var documents []sourceDocument
	seen := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(header.Name)
		if !matchesAnyArchivePattern(name, patterns) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}
		seen[name] = struct{}{}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			rel = name
		}
		documents = append(documents, sourceDocument{Name: filepath.ToSlash(rel), Data: data})
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("github source archive %s matched no files", rawURL)
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].Name < documents[j].Name })
	return documents, nil
}

func importDocument(source schemaSource, document sourceDocument) ([]generatedSchema, error) {
	if isSchemaJSON(document.Name, document.Data) {
		item, err := importJSONSchema(source, document)
		if err != nil {
			return nil, err
		}
		return []generatedSchema{item}, nil
	}
	return importCRDSchemas(source, document)
}

func isSchemaJSON(name string, data []byte) bool {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return true
	}
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func importJSONSchema(source schemaSource, document sourceDocument) (generatedSchema, error) {
	var schema any
	if err := json.Unmarshal(document.Data, &schema); err != nil {
		return generatedSchema{}, err
	}
	key, err := schemaKeyFromFilename(filepath.Base(document.Name))
	if err != nil {
		return generatedSchema{}, err
	}
	group, version, kind, err := splitCanonicalGVK(key)
	if err != nil {
		return generatedSchema{}, err
	}
	normalized, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return generatedSchema{}, err
	}
	normalized = append(normalized, '\n')
	filename := schemaFilename(group, version, kind)
	return generatedSchema{
		RelativePath: outputPathForSchema(source, filename),
		CanonicalGVK: key,
		Content:      normalized,
	}, nil
}

func importCRDSchemas(source schemaSource, document sourceDocument) ([]generatedSchema, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(document.Data))
	includeKinds := make(map[string]struct{}, len(source.IncludeKinds))
	for _, kind := range source.IncludeKinds {
		includeKinds[kind] = struct{}{}
	}

	var out []generatedSchema
	for {
		var item map[string]any
		err := decoder.Decode(&item)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(item) == 0 {
			continue
		}
		if kind, _ := item["kind"].(string); kind != "CustomResourceDefinition" {
			continue
		}
		schemas, err := extractSchemasFromCRD(source, item, includeKinds)
		if err != nil {
			return nil, err
		}
		out = append(out, schemas...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no CustomResourceDefinition schemas found")
	}
	return out, nil
}

func extractSchemasFromCRD(source schemaSource, root map[string]any, includeKinds map[string]struct{}) ([]generatedSchema, error) {
	spec, _ := root["spec"].(map[string]any)
	if spec == nil {
		return nil, errors.New("CRD missing spec")
	}
	group, _ := spec["group"].(string)
	names, _ := spec["names"].(map[string]any)
	kind, _ := names["kind"].(string)
	if len(includeKinds) > 0 {
		if _, ok := includeKinds[kind]; !ok {
			return nil, nil
		}
	}
	versions, _ := spec["versions"].([]any)
	if group == "" || kind == "" || len(versions) == 0 {
		return nil, errors.New("CRD missing group, kind, or versions")
	}

	var out []generatedSchema
	for _, versionItem := range versions {
		versionMap, _ := versionItem.(map[string]any)
		if versionMap == nil {
			continue
		}
		served, _ := versionMap["served"].(bool)
		if !served {
			continue
		}
		version, _ := versionMap["name"].(string)
		schemaRoot, _ := versionMap["schema"].(map[string]any)
		openAPISchema, ok := schemaRoot["openAPIV3Schema"]
		if !ok || version == "" {
			continue
		}
		normalized, err := json.MarshalIndent(openAPISchema, "", "  ")
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, '\n')
		filename := schemaFilename(group, version, kind)
		out = append(out, generatedSchema{
			RelativePath: outputPathForSchema(source, filename),
			CanonicalGVK: canonicalGVK(group, version, kind),
			Content:      normalized,
		})
	}
	return out, nil
}

func outputPathForSchema(source schemaSource, filename string) string {
	if source.Component == "kubernetes" {
		return filepath.ToSlash(filepath.Join("kubernetes", source.Version, filename))
	}
	return filepath.ToSlash(filepath.Join("platform", source.Component, source.Version, filename))
}

func buildGeneratedIndex(generated []generatedSchema) (schemaIndex, error) {
	schemas := make(map[string]string, len(generated))
	for _, item := range generated {
		if _, exists := schemas[item.CanonicalGVK]; exists {
			return schemaIndex{}, fmt.Errorf("duplicate schema for %s", item.CanonicalGVK)
		}
		schemas[item.CanonicalGVK] = filepath.ToSlash(filepath.Join("schemas", item.RelativePath))
	}
	return schemaIndex{Schemas: sortSchemas(schemas)}, nil
}

func writeGeneratedFiles(schemaRoot string, generated []generatedSchema, index schemaIndex) error {
	if err := os.MkdirAll(schemaRoot, 0o755); err != nil {
		return err
	}
	if err := removeStaleGeneratedFiles(schemaRoot, generated); err != nil {
		return err
	}
	for _, item := range generated {
		path := filepath.Join(schemaRoot, filepath.FromSlash(item.RelativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, item.Content, 0o644); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(schemaRoot, "index.json"), data, 0o644)
}

func verifyGeneratedFiles(schemaRoot string, generated []generatedSchema, index schemaIndex) error {
	expectedFiles := map[string][]byte{}
	for _, item := range generated {
		expectedFiles[filepath.ToSlash(item.RelativePath)] = item.Content
	}

	existingFiles, err := existingSchemaFiles(schemaRoot)
	if err != nil {
		return err
	}
	if len(existingFiles) != len(expectedFiles) {
		return fmt.Errorf("schema bundle drift detected; run cmd/schema-sync")
	}
	for path, expected := range expectedFiles {
		current, ok := existingFiles[path]
		if !ok || !bytes.Equal(current, expected) {
			return fmt.Errorf("schema bundle drift detected; run cmd/schema-sync")
		}
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	currentIndex, err := os.ReadFile(filepath.Join(schemaRoot, "index.json"))
	if err != nil {
		return err
	}
	if !bytes.Equal(currentIndex, data) {
		return fmt.Errorf("schema index is out of date; run cmd/schema-sync")
	}
	return nil
}

func existingSchemaFiles(schemaRoot string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if _, err := os.Stat(schemaRoot); errors.Is(err, os.ErrNotExist) {
		return files, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(schemaRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "index.json" || filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(schemaRoot, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

func removeStaleGeneratedFiles(schemaRoot string, generated []generatedSchema) error {
	expected := map[string]struct{}{}
	for _, item := range generated {
		expected[filepath.ToSlash(item.RelativePath)] = struct{}{}
	}
	files, err := existingSchemaFiles(schemaRoot)
	if err != nil {
		return err
	}
	for rel := range files {
		if _, ok := expected[rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(schemaRoot, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	return nil
}

func schemaKeyFromFilename(name string) (string, error) {
	trimmed := strings.TrimSuffix(name, filepath.Ext(name))
	if key, ok := legacySchemaFilenameKeys[trimmed]; ok {
		return key, nil
	}
	parts := strings.Split(trimmed, "_")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid schema filename %q", name)
	}
	kind := parts[len(parts)-1]
	version := parts[len(parts)-2]
	group := strings.Join(parts[:len(parts)-2], "_")
	group = strings.ReplaceAll(group, "_", ".")
	return canonicalGVK(group, version, kind), nil
}

var legacySchemaFilenameKeys = map[string]string{
	"deployment-apps-v1":    "apps/v1/Deployment",
	"job-batch-v1":          "batch/v1/Job",
	"service-v1":            "core/v1/Service",
	"ingress-networking-v1": "networking.k8s.io/v1/Ingress",
}

func schemaFilename(group string, version string, kind string) string {
	safeGroup := strings.ReplaceAll(group, ".", "_")
	if group == "" {
		safeGroup = "core"
	}
	return fmt.Sprintf("%s_%s_%s.json", safeGroup, version, kind)
}

func canonicalGVK(group string, version string, kind string) string {
	if group == "" || group == "core" {
		return fmt.Sprintf("core/%s/%s", version, kind)
	}
	return fmt.Sprintf("%s/%s/%s", group, version, kind)
}

func splitCanonicalGVK(key string) (group string, version string, kind string, err error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid canonical gvk %q", key)
	}
	group = parts[0]
	if group == "core" {
		group = ""
	}
	return group, parts[1], parts[2], nil
}

func sortSchemas(in map[string]string) map[string]string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func resolveLatestGitHubRelease(repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mobius-schema-sync")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resolveLatestGitHubTag(repo)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve latest release for %s: unexpected status %s", repo, resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("resolve latest release for %s: missing tag_name", repo)
	}
	return payload.TagName, nil
}

func resolveLatestGitHubTag(repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/tags?per_page=100", repo)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mobius-schema-sync")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve latest tag for %s: unexpected status %s", repo, resp.Status)
	}
	var payload []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("resolve latest tag for %s: no tags found", repo)
	}
	best := payload[0].Name
	bestVersion, _ := semver.NewVersion(strings.TrimPrefix(best, "v"))
	for _, item := range payload[1:] {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		candidateVersion, err := semver.NewVersion(strings.TrimPrefix(item.Name, "v"))
		if err != nil {
			if bestVersion == nil && item.Name > best {
				best = item.Name
			}
			continue
		}
		if bestVersion == nil || candidateVersion.GreaterThan(bestVersion) {
			best = item.Name
			bestVersion = candidateVersion
		}
	}
	return best, nil
}

func loadLock(path string) (schemaLock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return schemaLock{}, nil
	}
	if err != nil {
		return schemaLock{}, err
	}
	var lock schemaLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return schemaLock{}, err
	}
	return lock, nil
}

func writeLockFile(path string, lock schemaLock) error {
	data, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func verifyLockFile(path string, expected schemaLock) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	want, err := yaml.Marshal(expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, want) {
		return fmt.Errorf("schema lock is out of date; run cmd/schema-sync")
	}
	return nil
}

func findLockedSource(lock schemaLock, source schemaSource) (lockedSource, bool) {
	for _, item := range lock.Sources {
		if item.Component == source.Component && item.SourceType == source.SourceType && item.Repo == source.Repo && item.AssetName == source.AssetName {
			return item, true
		}
	}
	return lockedSource{}, false
}

func matchesAnyArchivePattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		matchPattern := filepath.ToSlash(pattern)
		matchPattern = strings.TrimPrefix(matchPattern, "/")
		ok, err := filepath.Match(matchPattern, name)
		if err == nil && ok {
			return true
		}
		if strings.HasSuffix(matchPattern, "/*") {
			prefix := strings.TrimSuffix(matchPattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

func expandPatterns(patterns []string, resolvedVersion string) []string {
	trimmedVersion := strings.TrimPrefix(resolvedVersion, "v")
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.ReplaceAll(pattern, "{version}", resolvedVersion)
		pattern = strings.ReplaceAll(pattern, "{version_nov}", trimmedVersion)
		out = append(out, pattern)
	}
	return out
}

func resolvePath(root string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
