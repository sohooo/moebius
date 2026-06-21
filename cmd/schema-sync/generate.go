package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
)

func generateSchemas(root string, manifest sourceManifest, existingLock schemaLock, verify bool) ([]generatedSchema, schemaLock, error) {
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
	generated := make([]generatedSchema, 0, len(keys))
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
