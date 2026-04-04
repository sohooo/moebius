package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const EnvConfigYAML = "MOBIUS_CONFIG_YAML"

var placeholderPattern = regexp.MustCompile(`\{([^{}]+)\}`)

var canonicalFields = []string{"name", "namespace", "project", "chart", "version"}

type RepoConfig struct {
	Layout LayoutConfig `yaml:"layout"`
}

type LayoutConfig struct {
	ClustersDir string          `yaml:"clusters_dir"`
	Apps        AppsConfig      `yaml:"apps"`
	Overrides   OverridesConfig `yaml:"overrides"`
}

type AppsConfig struct {
	File     string           `yaml:"file"`
	Kind     string           `yaml:"kind"`
	Fields   AppsFieldsConfig `yaml:"fields"`
	Required []string         `yaml:"required"`
}

type AppsFieldsConfig struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Project   string `yaml:"project"`
	Chart     string `yaml:"chart"`
	Version   string `yaml:"version"`
}

type OverridesConfig struct {
	Path         string `yaml:"path"`
	FallbackPath string `yaml:"fallback_path"`
}

type Release struct {
	Name      string
	Namespace string
	Project   string
	Chart     string
	Version   string
}

func Default() RepoConfig {
	return RepoConfig{
		Layout: LayoutConfig{
			ClustersDir: "clusters",
			Apps: AppsConfig{
				File: "apps.yaml",
				Kind: "list",
				Fields: AppsFieldsConfig{
					Name:      "name",
					Namespace: "namespace",
					Project:   "project",
					Chart:     "chart",
					Version:   "version",
				},
				Required: []string{"name", "namespace", "chart"},
			},
			Overrides: OverridesConfig{
				Path:         "overrides/{project}/{name}.yaml",
				FallbackPath: "overrides/{name}.yaml",
			},
		},
	}
}

func LoadRepoConfig(root string) (RepoConfig, error) {
	cfg := Default()

	filePath := filepath.Join(root, "config.yaml")
	if data, err := os.ReadFile(filePath); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return RepoConfig{}, fmt.Errorf("parse %s: %w", filePath, err)
		}
	} else if !os.IsNotExist(err) {
		return RepoConfig{}, fmt.Errorf("read %s: %w", filePath, err)
	}

	if envConfig := os.Getenv(EnvConfigYAML); envConfig != "" {
		if err := yaml.Unmarshal([]byte(envConfig), &cfg); err != nil {
			return RepoConfig{}, fmt.Errorf("parse %s: %w", EnvConfigYAML, err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return RepoConfig{}, err
	}
	return cfg, nil
}

func (c RepoConfig) Validate() error {
	if c.Layout.ClustersDir == "" {
		return fmt.Errorf("layout.clusters_dir must not be empty")
	}
	if filepath.IsAbs(c.Layout.ClustersDir) {
		return fmt.Errorf("layout.clusters_dir must be relative")
	}
	if c.Layout.Apps.File == "" {
		return fmt.Errorf("layout.apps.file must not be empty")
	}
	if filepath.IsAbs(c.Layout.Apps.File) {
		return fmt.Errorf("layout.apps.file must be relative to the cluster directory")
	}
	if c.Layout.Apps.Kind != "list" {
		return fmt.Errorf("layout.apps.kind must be %q", "list")
	}

	fieldMap := c.Layout.Apps.Fields.Map()
	for canonical, actual := range fieldMap {
		if actual == "" {
			return fmt.Errorf("layout.apps.fields.%s must not be empty", canonical)
		}
	}
	for _, required := range c.Layout.Apps.Required {
		if !slices.Contains(canonicalFields, required) {
			return fmt.Errorf("layout.apps.required contains unknown canonical field %q", required)
		}
	}

	if err := validatePattern(c.Layout.Overrides.Path, "layout.overrides.path"); err != nil {
		return err
	}
	if c.Layout.Overrides.FallbackPath != "" {
		if err := validatePattern(c.Layout.Overrides.FallbackPath, "layout.overrides.fallback_path"); err != nil {
			return err
		}
	}
	return nil
}

func (c RepoConfig) EffectiveClustersDir(override string) string {
	if override != "" {
		return override
	}
	return c.Layout.ClustersDir
}

func ClusterDir(root string, layout LayoutConfig, cluster string) string {
	return filepath.Join(root, layout.ClustersDir, cluster)
}

func AppsPath(root string, layout LayoutConfig, cluster string) string {
	return filepath.Join(ClusterDir(root, layout, cluster), layout.Apps.File)
}

func LoadReleases(root string, layout LayoutConfig, cluster string) ([]Release, error) {
	path := AppsPath(root, layout, cluster)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var raw []map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s must contain at least one release", path)
	}

	fieldMap := layout.Apps.Fields.Map()
	seen := map[string]struct{}{}
	releases := make([]Release, 0, len(raw))
	for _, item := range raw {
		item = normalizeMap(item)
		release := Release{
			Name:      stringField(item, fieldMap["name"]),
			Namespace: stringField(item, fieldMap["namespace"]),
			Project:   stringField(item, fieldMap["project"]),
			Chart:     stringField(item, fieldMap["chart"]),
			Version:   stringField(item, fieldMap["version"]),
		}
		if err := validateRelease(path, layout.Apps.Required, release); err != nil {
			return nil, err
		}
		if _, ok := seen[release.Name]; ok {
			return nil, fmt.Errorf("duplicate release name %q in %s", release.Name, path)
		}
		seen[release.Name] = struct{}{}
		releases = append(releases, release)
	}
	return releases, nil
}

func ResolveOverridePath(root string, layout LayoutConfig, cluster string, release Release) string {
	primary := filepath.Join(ClusterDir(root, layout, cluster), renderPattern(layout.Overrides.Path, cluster, release))
	if fileExists(primary) || layout.Overrides.FallbackPath == "" {
		return primary
	}
	return filepath.Join(ClusterDir(root, layout, cluster), renderPattern(layout.Overrides.FallbackPath, cluster, release))
}

func (f AppsFieldsConfig) Map() map[string]string {
	return map[string]string{
		"name":      f.Name,
		"namespace": f.Namespace,
		"project":   f.Project,
		"chart":     f.Chart,
		"version":   f.Version,
	}
}

func validatePattern(pattern, field string) error {
	if pattern == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("%s must be relative to the cluster directory", field)
	}
	matches := placeholderPattern.FindAllStringSubmatch(pattern, -1)
	for _, match := range matches {
		switch match[1] {
		case "cluster", "project", "name":
		default:
			return fmt.Errorf("%s contains unknown placeholder %q", field, match[1])
		}
	}
	return nil
}

func renderPattern(pattern, cluster string, release Release) string {
	replacer := strings.NewReplacer(
		"{cluster}", releaseOr(cluster),
		"{project}", releaseOr(release.Project),
		"{name}", releaseOr(release.Name),
	)
	return filepath.Clean(replacer.Replace(pattern))
}

func validateRelease(path string, required []string, release Release) error {
	values := map[string]string{
		"name":      release.Name,
		"namespace": release.Namespace,
		"project":   release.Project,
		"chart":     release.Chart,
		"version":   release.Version,
	}
	for _, field := range required {
		if values[field] != "" {
			continue
		}
		if release.Name != "" {
			return fmt.Errorf("release %q missing %s in %s", release.Name, field, path)
		}
		return fmt.Errorf("release missing %s in %s", field, path)
	}
	return nil
}

func normalizeMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringField(item map[string]any, key string) string {
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func releaseOr(value string) string {
	if value == "" {
		return ""
	}
	return value
}
