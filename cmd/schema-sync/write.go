package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

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
