package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
