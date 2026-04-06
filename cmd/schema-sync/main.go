package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	IncludeKinds []string `yaml:"include_kinds"`
	Note         string   `yaml:"note"`
}

type schemaIndex struct {
	Schemas map[string]string `json:"schemas"`
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

	manifest, err := loadManifest(filepath.Join(root, manifestPath))
	if err != nil {
		fatal(err)
	}

	schemaRoot := filepath.Join(root, "internal/validate/schemas")
	generated, err := generateSchemas(root, manifest)
	if err != nil {
		fatal(err)
	}
	index, err := buildGeneratedIndex(generated)
	if err != nil {
		fatal(err)
	}

	if verify {
		if err := verifyGeneratedFiles(schemaRoot, generated, index); err != nil {
			fatal(err)
		}
		return
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
	default:
		return fmt.Errorf("schema source %s has unsupported source_type %q", source.Component, source.SourceType)
	}
	return nil
}

func generateSchemas(root string, manifest sourceManifest) ([]generatedSchema, error) {
	var generated []generatedSchema
	seen := map[string]generatedSchema{}

	for _, source := range manifest.Sources {
		documents, err := loadSourceDocuments(root, source)
		if err != nil {
			return nil, err
		}
		for _, document := range documents {
			items, err := importDocument(source, document)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", document.Name, err)
			}
			for _, item := range items {
				if prior, ok := seen[item.RelativePath]; ok && !bytes.Equal(prior.Content, item.Content) {
					return nil, fmt.Errorf("schema path collision for %s", item.RelativePath)
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
	return generated, nil
}

func loadSourceDocuments(root string, source schemaSource) ([]sourceDocument, error) {
	switch source.SourceType {
	case "file":
		return loadFileSourceDocuments(root, source.Paths)
	case "url":
		return loadURLSourceDocuments(source.URLs)
	default:
		return nil, fmt.Errorf("unsupported source type %q", source.SourceType)
	}
}

func loadFileSourceDocuments(root string, patterns []string) ([]sourceDocument, error) {
	var documents []sourceDocument
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
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
	filename := filepath.Base(document.Name)
	key, err := schemaKeyFromFilename(filename)
	if err != nil {
		return generatedSchema{}, err
	}
	normalized, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return generatedSchema{}, err
	}
	normalized = append(normalized, '\n')
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

func inferFilenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Base(rawURL)
	}
	return filepath.Base(parsed.Path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
