package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Release struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Chart     string `yaml:"chart"`
	Version   string `yaml:"version"`
}

func LoadReleases(root, clustersDir, cluster string) ([]Release, error) {
	path := filepath.Join(root, clustersDir, cluster, "apps.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var releases []Release
	if err := yaml.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%s must contain at least one release", path)
	}

	seen := map[string]struct{}{}
	for _, release := range releases {
		if release.Name == "" {
			return nil, fmt.Errorf("release without name in %s", path)
		}
		if release.Namespace == "" {
			return nil, fmt.Errorf("release %q missing namespace in %s", release.Name, path)
		}
		if release.Chart == "" {
			return nil, fmt.Errorf("release %q missing chart in %s", release.Name, path)
		}
		if _, ok := seen[release.Name]; ok {
			return nil, fmt.Errorf("duplicate release name %q in %s", release.Name, path)
		}
		seen[release.Name] = struct{}{}
	}
	return releases, nil
}

func OverridePath(root, clustersDir, cluster, releaseName string) string {
	return filepath.Join(root, clustersDir, cluster, "overrides", releaseName+".yaml")
}
