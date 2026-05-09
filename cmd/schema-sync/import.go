package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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
