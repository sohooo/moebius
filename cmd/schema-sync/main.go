package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
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
		if source.Version == "latest" && strings.TrimSpace(source.Repo) == "" {
			return fmt.Errorf("schema source %s must set repo when url version is latest", source.Component)
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
