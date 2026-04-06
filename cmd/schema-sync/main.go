package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type sourceManifest struct {
	Sources []struct {
		Component  string   `yaml:"component"`
		Version    string   `yaml:"version"`
		SourceType string   `yaml:"source_type"`
		URLs       []string `yaml:"urls"`
		Note       string   `yaml:"note"`
	} `yaml:"sources"`
}

type schemaIndex struct {
	Schemas map[string]string `json:"schemas"`
}

func main() {
	var (
		verify       bool
		manifestPath string
		root         string
	)
	flag.BoolVar(&verify, "verify", false, "Verify that the generated schema index is up to date")
	flag.StringVar(&manifestPath, "manifest", "schemasources.yaml", "Path to the schema source manifest")
	flag.StringVar(&root, "root", ".", "Repository root")
	flag.Parse()

	if err := loadManifest(filepath.Join(root, manifestPath)); err != nil {
		fatal(err)
	}

	idx, err := buildIndex(filepath.Join(root, "internal/validate/schemas"))
	if err != nil {
		fatal(err)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')

	indexPath := filepath.Join(root, "internal/validate/schemas/index.json")
	if verify {
		current, err := os.ReadFile(indexPath)
		if err != nil {
			fatal(err)
		}
		if !bytes.Equal(current, data) {
			fatal(fmt.Errorf("schema index is out of date; run cmd/schema-sync"))
		}
		return
	}
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		fatal(err)
	}
}

func loadManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest sourceManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if len(manifest.Sources) == 0 {
		return fmt.Errorf("manifest %s contains no sources", path)
	}
	return nil
}

func buildIndex(schemaRoot string) (schemaIndex, error) {
	schemas := map[string]string{}
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
		key, err := schemaKeyFromFilename(filepath.Base(path))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(schemaRoot, path)
		if err != nil {
			return err
		}
		schemas[key] = filepath.ToSlash(filepath.Join("schemas", rel))
		return nil
	})
	if err != nil {
		return schemaIndex{}, err
	}
	return schemaIndex{Schemas: sortSchemas(schemas)}, nil
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
	if group == "core" {
		group = "core"
	}
	return fmt.Sprintf("%s/%s/%s", group, version, kind), nil
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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
