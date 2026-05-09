package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	Repo         string   `yaml:"repo"`
	AssetName    string   `yaml:"asset_name"`
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
