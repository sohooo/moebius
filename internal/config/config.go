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
const EnvAppsFiles = "MOBIUS_APPS_FILES"

var placeholderPattern = regexp.MustCompile(`\{([^{}]+)\}`)

var canonicalFields = []string{"name", "namespace", "project", "repoURL", "chart", "targetRevision"}

type RepoConfig struct {
	Layout LayoutConfig `yaml:"layout"`
	Diff   DiffConfig   `yaml:"diff"`
}

type LoadMetadata struct {
	UsedConfigFile   bool
	UsedEnvConfig    bool
	UsedEnvAppsFiles bool
}

type LayoutConfig struct {
	ClustersDir string          `yaml:"clusters_dir"`
	Apps        AppsConfig      `yaml:"apps"`
	Overrides   OverridesConfig `yaml:"overrides"`
}

type AppsConfig struct {
	File     string           `yaml:"file"`
	Files    []string         `yaml:"files"`
	Kind     string           `yaml:"kind"`
	Fields   AppsFieldsConfig `yaml:"fields"`
	Required []string         `yaml:"required"`
}

type AppsFieldsConfig struct {
	Name           string `yaml:"name"`
	Namespace      string `yaml:"namespace"`
	Project        string `yaml:"project"`
	RepoURL        string `yaml:"repoURL"`
	Chart          string `yaml:"chart"`
	TargetRevision string `yaml:"targetRevision"`
}

type OverridesConfig struct {
	Path         string `yaml:"path"`
	FallbackPath string `yaml:"fallback_path"`
}

type DiffConfig struct {
	Ignore DiffIgnoreConfig `yaml:"ignore"`
}

type DiffIgnoreConfig struct {
	Defaults bool                     `yaml:"defaults"`
	Metadata []DiffMetadataIgnoreRule `yaml:"metadata"`
}

type DiffMetadataIgnoreRule struct {
	Locations   []string `yaml:"locations"`
	Labels      []string `yaml:"labels"`
	Annotations []string `yaml:"annotations"`
}

type Release struct {
	Name           string
	Namespace      string
	Project        string
	RepoURL        string
	Chart          string
	TargetRevision string
}

type ReleaseWarning struct {
	ReleaseName  string
	Message      string
	SelectedFile string
	IgnoredFile  string
}

type ReleaseMetadata struct {
	Release    Release
	SourceFile string
}

func Default() RepoConfig {
	return RepoConfig{
		Layout: LayoutConfig{
			ClustersDir: "clusters",
			Apps: AppsConfig{
				Files: []string{"apps.yaml", "apps-dev.yaml"},
				Kind:  "list",
				Fields: AppsFieldsConfig{
					Name:           "name",
					Namespace:      "namespace",
					Project:        "project",
					RepoURL:        "repoURL",
					Chart:          "chart",
					TargetRevision: "targetRevision",
				},
				Required: []string{"name", "namespace", "chart"},
			},
			Overrides: OverridesConfig{
				Path:         "overrides/{project}/{name}.yaml",
				FallbackPath: "overrides/{name}.yaml",
			},
		},
		Diff: DiffConfig{
			Ignore: DiffIgnoreConfig{
				Defaults: true,
			},
		},
	}
}

func LoadRepoConfig(root string) (RepoConfig, error) {
	cfg, _, err := LoadRepoConfigWithMetadata(root)
	return cfg, err
}

func LoadRepoConfigWithMetadata(root string) (RepoConfig, LoadMetadata, error) {
	cfg := Default()
	meta := LoadMetadata{}

	filePath := filepath.Join(root, "config.yaml")
	if data, err := os.ReadFile(filePath); err == nil {
		meta.UsedConfigFile = true
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return RepoConfig{}, meta, fmt.Errorf("parse %s: %w", filePath, err)
		}
	} else if !os.IsNotExist(err) {
		return RepoConfig{}, meta, fmt.Errorf("read %s: %w", filePath, err)
	}

	if envConfig := os.Getenv(EnvConfigYAML); envConfig != "" {
		meta.UsedEnvConfig = true
		if err := yaml.Unmarshal([]byte(envConfig), &cfg); err != nil {
			return RepoConfig{}, meta, fmt.Errorf("parse %s: %w", EnvConfigYAML, err)
		}
	}

	if envAppsFiles := os.Getenv(EnvAppsFiles); envAppsFiles != "" {
		meta.UsedEnvAppsFiles = true
		files, err := ParseAppsFiles(envAppsFiles)
		if err != nil {
			return RepoConfig{}, meta, fmt.Errorf("parse %s: %w", EnvAppsFiles, err)
		}
		cfg.Layout.Apps.Files = files
	}

	if err := cfg.Validate(); err != nil {
		return RepoConfig{}, meta, fmt.Errorf("invalid config after applying %s: %w", meta.SourceSummary(), err)
	}
	return cfg, meta, nil
}

func (m LoadMetadata) SourceSummary() string {
	sources := []string{"built-in defaults"}
	if m.UsedConfigFile {
		sources = append(sources, "config.yaml")
	}
	if m.UsedEnvConfig {
		sources = append(sources, EnvConfigYAML)
	}
	if m.UsedEnvAppsFiles {
		sources = append(sources, EnvAppsFiles)
	}
	return strings.Join(sources, " + ")
}

func (c *RepoConfig) Validate() error {
	if c.Layout.ClustersDir == "" {
		return fmt.Errorf("layout.clusters_dir must not be empty")
	}
	if filepath.IsAbs(c.Layout.ClustersDir) {
		return fmt.Errorf("layout.clusters_dir must be relative")
	}
	if c.Layout.Apps.File != "" {
		return fmt.Errorf("layout.apps.file is no longer supported; use layout.apps.files")
	}
	if len(c.Layout.Apps.Files) == 0 {
		return fmt.Errorf("layout.apps.files must contain at least one file")
	}
	seenAppsFiles := map[string]struct{}{}
	normalizedAppsFiles := make([]string, 0, len(c.Layout.Apps.Files))
	for _, file := range c.Layout.Apps.Files {
		file = strings.TrimSpace(file)
		if file == "" {
			return fmt.Errorf("layout.apps.files must not contain empty entries")
		}
		if filepath.IsAbs(file) {
			return fmt.Errorf("layout.apps.files entries must be relative to the cluster directory")
		}
		clean := filepath.ToSlash(filepath.Clean(file))
		if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
			return fmt.Errorf("layout.apps.files entries must stay within the cluster directory")
		}
		if _, ok := seenAppsFiles[clean]; ok {
			return fmt.Errorf("layout.apps.files contains duplicate file %q", clean)
		}
		seenAppsFiles[clean] = struct{}{}
		normalizedAppsFiles = append(normalizedAppsFiles, clean)
	}
	c.Layout.Apps.Files = normalizedAppsFiles
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
	if err := validateDiffIgnore(c.Diff.Ignore); err != nil {
		return err
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
	return filepath.Join(ClusterDir(root, layout, cluster), layout.Apps.Files[0])
}

func AppsPaths(root string, layout LayoutConfig, cluster string) []string {
	clusterDir := ClusterDir(root, layout, cluster)
	paths := make([]string, 0, len(layout.Apps.Files))
	for _, file := range layout.Apps.Files {
		paths = append(paths, filepath.Join(clusterDir, filepath.FromSlash(file)))
	}
	return paths
}

func LoadReleases(root string, layout LayoutConfig, cluster string) ([]Release, error) {
	releases, _, err := LoadReleasesWithWarnings(root, layout, cluster)
	return releases, err
}

func LoadReleasesWithWarnings(root string, layout LayoutConfig, cluster string) ([]Release, []ReleaseWarning, error) {
	metadata, warnings, err := LoadReleaseMetadataWithWarnings(root, layout, cluster)
	if err != nil {
		return nil, nil, err
	}
	releases := make([]Release, 0, len(metadata))
	for _, item := range metadata {
		releases = append(releases, item.Release)
	}
	return releases, warnings, nil
}

func LoadReleaseMetadataWithWarnings(root string, layout LayoutConfig, cluster string) ([]ReleaseMetadata, []ReleaseWarning, error) {
	paths := AppsPaths(root, layout, cluster)
	releases := make([]ReleaseMetadata, 0)
	warnings := make([]ReleaseWarning, 0)
	seenAcrossFiles := map[string]string{}
	found := false
	fieldMap := layout.Apps.Fields.Map()
	for _, path := range paths {
		raw, err := loadReleaseItems(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		found = true
		seenInFile := map[string]struct{}{}
		for _, item := range raw {
			item = normalizeMap(item)
			release := Release{
				Name:           stringField(item, fieldMap["name"]),
				Namespace:      stringField(item, fieldMap["namespace"]),
				Project:        stringField(item, fieldMap["project"]),
				RepoURL:        stringField(item, fieldMap["repoURL"]),
				Chart:          stringField(item, fieldMap["chart"]),
				TargetRevision: stringField(item, fieldMap["targetRevision"]),
			}
			if release.Name != "" {
				if _, ok := seenInFile[release.Name]; ok {
					return nil, nil, fmt.Errorf("duplicate release name %q in %s", release.Name, path)
				}
				seenInFile[release.Name] = struct{}{}
				if winnerPath, ok := seenAcrossFiles[release.Name]; ok {
					warnings = append(warnings, ReleaseWarning{
						ReleaseName:  release.Name,
						SelectedFile: filepath.Base(winnerPath),
						IgnoredFile:  filepath.Base(path),
						Message:      fmt.Sprintf("release %q is defined in both %s and %s; using the higher-priority definition from %s", release.Name, filepath.Base(winnerPath), filepath.Base(path), filepath.Base(winnerPath)),
					})
					continue
				}
			}
			if err := validateRelease(path, layout.Apps.Required, release); err != nil {
				return nil, nil, err
			}
			if _, ok := seenAcrossFiles[release.Name]; ok {
				continue
			}
			seenAcrossFiles[release.Name] = path
			releases = append(releases, ReleaseMetadata{
				Release:    release,
				SourceFile: filepath.Base(path),
			})
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("none of the configured apps files exist for cluster %q: %s", cluster, AppsFilesSummary(layout))
	}
	return releases, warnings, nil
}

func loadReleaseItems(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw []map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s must contain at least one release", path)
	}
	return raw, nil
}

func ParseAppsFiles(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	files := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		file := strings.TrimSpace(part)
		if file == "" {
			return nil, fmt.Errorf("apps files list must not contain empty entries")
		}
		if filepath.IsAbs(file) {
			return nil, fmt.Errorf("apps file %q must be relative to the cluster directory", file)
		}
		clean := filepath.ToSlash(filepath.Clean(file))
		if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
			return nil, fmt.Errorf("apps file %q must stay within the cluster directory", file)
		}
		if _, ok := seen[clean]; ok {
			return nil, fmt.Errorf("duplicate apps file %q", clean)
		}
		seen[clean] = struct{}{}
		files = append(files, clean)
	}
	return files, nil
}

func AppsFilesSummary(layout LayoutConfig) string {
	return strings.Join(layout.Apps.Files, ",")
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
		"name":           f.Name,
		"namespace":      f.Namespace,
		"project":        f.Project,
		"repoURL":        f.RepoURL,
		"chart":          f.Chart,
		"targetRevision": f.TargetRevision,
	}
}

func (r Release) IsRemoteChart() bool {
	return r.RepoURL != "" || strings.HasPrefix(r.Chart, "oci://")
}

func (r Release) ChartReference() string {
	if r.RepoURL == "" {
		return r.Chart
	}
	repoURL := strings.TrimSuffix(r.RepoURL, "/")
	chart := strings.TrimPrefix(r.Chart, "/")
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		return chart
	}
	if strings.HasPrefix(repoURL, "oci://") {
		return repoURL + "/" + chart
	}
	return "oci://" + repoURL + "/" + chart
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

func validateDiffIgnore(ignore DiffIgnoreConfig) error {
	for i, rule := range ignore.Metadata {
		prefix := fmt.Sprintf("diff.ignore.metadata[%d]", i)
		if len(rule.Locations) == 0 {
			return fmt.Errorf("%s.locations must contain at least one path", prefix)
		}
		if len(rule.Labels) == 0 && len(rule.Annotations) == 0 {
			return fmt.Errorf("%s must contain at least one label or annotation pattern", prefix)
		}
		for j, location := range rule.Locations {
			if err := validateIgnoreLocation(location); err != nil {
				return fmt.Errorf("%s.locations[%d]: %w", prefix, j, err)
			}
		}
		for j, pattern := range rule.Labels {
			if err := validateIgnoreGlob(pattern); err != nil {
				return fmt.Errorf("%s.labels[%d]: %w", prefix, j, err)
			}
		}
		for j, pattern := range rule.Annotations {
			if err := validateIgnoreGlob(pattern); err != nil {
				return fmt.Errorf("%s.annotations[%d]: %w", prefix, j, err)
			}
		}
	}
	return nil
}

func validateIgnoreLocation(location string) error {
	location = strings.TrimSpace(location)
	if location == "" {
		return fmt.Errorf("location must not be empty")
	}
	if strings.HasPrefix(location, ".") || strings.HasSuffix(location, ".") || strings.Contains(location, "..") {
		return fmt.Errorf("location %q must be a relative semantic path", location)
	}
	for _, part := range strings.Split(location, ".") {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("location %q must not contain empty path segments", location)
		}
		if strings.ContainsAny(part, "[]*") {
			return fmt.Errorf("location %q must not contain indexes or wildcards", location)
		}
	}
	return nil
}

func validateIgnoreGlob(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fmt.Errorf("pattern must not be empty")
	}
	for _, r := range pattern {
		switch r {
		case '*':
		case '[', ']', '?':
			return fmt.Errorf("pattern %q only supports * as a wildcard", pattern)
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
		"name":           release.Name,
		"namespace":      release.Namespace,
		"project":        release.Project,
		"repoURL":        release.RepoURL,
		"chart":          release.Chart,
		"targetRevision": release.TargetRevision,
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
	if release.RepoURL != "" && release.TargetRevision == "" {
		return fmt.Errorf("release %q missing targetRevision in %s", release.Name, path)
	}
	if strings.HasPrefix(release.Chart, "oci://") && release.TargetRevision == "" {
		return fmt.Errorf("release %q missing targetRevision in %s", release.Name, path)
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
