package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadRepoConfigUsesDefaultsWhenNoFileOrEnvIsPresent(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.ClustersDir != "clusters" {
		t.Fatalf("expected default clusters_dir, got %q", cfg.Layout.ClustersDir)
	}
	if !slices.Equal(cfg.Layout.Apps.Files, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("expected default apps files, got %v", cfg.Layout.Apps.Files)
	}
	if cfg.Layout.Overrides.CommonPath != "overrides/common.yaml" {
		t.Fatalf("expected default common override path, got %q", cfg.Layout.Overrides.CommonPath)
	}
	if !cfg.Diff.Ignore.Defaults {
		t.Fatalf("expected default diff ignore rules to be enabled")
	}
}

func TestLoadRepoConfigReadsOptionalConfigFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  clusters_dir: custom-clusters\n")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.ClustersDir != "custom-clusters" {
		t.Fatalf("unexpected clusters_dir: %q", cfg.Layout.ClustersDir)
	}
	if !slices.Equal(cfg.Layout.Apps.Files, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("expected default apps files, got %v", cfg.Layout.Apps.Files)
	}
}

func TestLoadRepoConfigReadsAppsFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  apps:\n    files:\n      - apps.yaml\n      - apps-dev.yaml\n")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if !slices.Equal(cfg.Layout.Apps.Files, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("unexpected apps files: %v", cfg.Layout.Apps.Files)
	}
}

func TestLoadRepoConfigReadsDiffIgnoreConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")
	writeFile(t, filepath.Join(root, "config.yaml"), `diff:
  ignore:
    defaults: false
    metadata:
      - locations:
          - metadata
          - spec.template.metadata
        labels:
          - app.kubernetes.io/version
        annotations:
          - checksum/*
`)

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Diff.Ignore.Defaults {
		t.Fatalf("expected defaults to be disabled")
	}
	if len(cfg.Diff.Ignore.Metadata) != 1 {
		t.Fatalf("expected one metadata ignore rule, got %d", len(cfg.Diff.Ignore.Metadata))
	}
	rule := cfg.Diff.Ignore.Metadata[0]
	if !slices.Equal(rule.Locations, []string{"metadata", "spec.template.metadata"}) {
		t.Fatalf("unexpected locations: %v", rule.Locations)
	}
	if !slices.Equal(rule.Labels, []string{"app.kubernetes.io/version"}) {
		t.Fatalf("unexpected labels: %v", rule.Labels)
	}
	if !slices.Equal(rule.Annotations, []string{"checksum/*"}) {
		t.Fatalf("unexpected annotations: %v", rule.Annotations)
	}
}

func TestLoadRepoConfigRejectsInvalidDiffIgnoreConfig(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		needle string
	}{
		{
			name: "empty locations",
			body: `diff:
  ignore:
    metadata:
      - labels: ["helm.sh/chart"]
`,
			needle: "locations must contain at least one path",
		},
		{
			name: "empty labels and annotations",
			body: `diff:
  ignore:
    metadata:
      - locations: ["metadata"]
`,
			needle: "must contain at least one label or annotation pattern",
		},
		{
			name: "invalid location",
			body: `diff:
  ignore:
    metadata:
      - locations: ["spec..metadata"]
        labels: ["helm.sh/chart"]
`,
			needle: "relative semantic path",
		},
		{
			name: "invalid glob",
			body: `diff:
  ignore:
    metadata:
      - locations: ["metadata"]
        annotations: ["checksum/?"]
`,
			needle: "only supports *",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv(EnvConfigYAML, "")
			t.Setenv(EnvAppsFiles, "")
			writeFile(t, filepath.Join(root, "config.yaml"), tc.body)

			_, err := LoadRepoConfig(root)
			if err == nil {
				t.Fatal("expected config validation error")
			}
			if !strings.Contains(err.Error(), tc.needle) {
				t.Fatalf("expected error to contain %q, got %v", tc.needle, err)
			}
		})
	}
}

func TestLoadRepoConfigRejectsDeprecatedAppsFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  apps:\n    file: releases.yaml\n")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected deprecated apps file error")
	}
	if !strings.Contains(err.Error(), "layout.apps.file is no longer supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRepoConfigReadsEnvYAMLWithoutFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "layout:\n  clusters_dir: environments\n")
	t.Setenv(EnvAppsFiles, "")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.ClustersDir != "environments" {
		t.Fatalf("unexpected clusters_dir: %q", cfg.Layout.ClustersDir)
	}
}

func TestLoadRepoConfigEnvAppsFilesOverridesConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  apps:\n    files:\n      - apps.yaml\n")
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "apps.yaml, apps-dev.yaml")

	cfg, meta, err := LoadRepoConfigWithMetadata(root)
	if err != nil {
		t.Fatalf("LoadRepoConfigWithMetadata returned error: %v", err)
	}
	if !slices.Equal(cfg.Layout.Apps.Files, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("unexpected apps files: %v", cfg.Layout.Apps.Files)
	}
	if !meta.UsedEnvAppsFiles {
		t.Fatalf("expected env apps files metadata, got %+v", meta)
	}
	if !strings.Contains(meta.SourceSummary(), EnvAppsFiles) {
		t.Fatalf("expected source summary to include %s, got %q", EnvAppsFiles, meta.SourceSummary())
	}
}

func TestLoadRepoConfigEnvOverridesFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  clusters_dir: clusters-from-file\n")
	t.Setenv(EnvConfigYAML, "layout:\n  clusters_dir: clusters-from-env\n")
	t.Setenv(EnvAppsFiles, "")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.ClustersDir != "clusters-from-env" {
		t.Fatalf("unexpected clusters_dir: %q", cfg.Layout.ClustersDir)
	}
}

func TestLoadRepoConfigWithMetadataReportsAppliedSources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  clusters_dir: clusters-from-file\n")
	t.Setenv(EnvConfigYAML, "layout:\n  clusters_dir: clusters-from-env\n")
	t.Setenv(EnvAppsFiles, "")

	_, meta, err := LoadRepoConfigWithMetadata(root)
	if err != nil {
		t.Fatalf("LoadRepoConfigWithMetadata returned error: %v", err)
	}
	if !meta.UsedConfigFile || !meta.UsedEnvConfig {
		t.Fatalf("expected config file and env metadata, got %+v", meta)
	}
	if got := meta.SourceSummary(); got != "built-in defaults + config.yaml + MOBIUS_CONFIG_YAML" {
		t.Fatalf("unexpected source summary: %q", got)
	}
}

func TestLoadRepoConfigRejectsInvalidEnvYAML(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigYAML, "layout: [")
	t.Setenv(EnvAppsFiles, "")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected env parse error")
	}
	if !strings.Contains(err.Error(), EnvConfigYAML) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRepoConfigRejectsUnknownPlaceholder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  overrides:\n    path: overrides/{team}/{name}.yaml\n")
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected invalid placeholder error")
	}
	if !strings.Contains(err.Error(), `unknown placeholder "team"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRepoConfigReadsCommonOverridePath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  overrides:\n    common_path: values/common.yaml\n")
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")

	cfg, err := LoadRepoConfig(root)
	if err != nil {
		t.Fatalf("LoadRepoConfig returned error: %v", err)
	}
	if cfg.Layout.Overrides.CommonPath != "values/common.yaml" {
		t.Fatalf("unexpected common override path %q", cfg.Layout.Overrides.CommonPath)
	}
}

func TestLoadRepoConfigRejectsInvalidCommonOverridePath(t *testing.T) {
	for _, path := range []string{"/values/common.yaml", "../values/common.yaml"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  overrides:\n    common_path: "+path+"\n")
			t.Setenv(EnvConfigYAML, "")
			t.Setenv(EnvAppsFiles, "")

			_, err := LoadRepoConfig(root)
			if err == nil {
				t.Fatal("expected invalid common override path error")
			}
			if !strings.Contains(err.Error(), "layout.overrides.common_path") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadRepoConfigRejectsUnknownRequiredField(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.yaml"), "layout:\n  apps:\n    required:\n      - release_name\n")
	t.Setenv(EnvConfigYAML, "")
	t.Setenv(EnvAppsFiles, "")

	_, err := LoadRepoConfig(root)
	if err == nil {
		t.Fatal("expected invalid required field error")
	}
	if !strings.Contains(err.Error(), `unknown canonical field "release_name"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesUsesConfiguredFieldNames(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Fields = AppsFieldsConfig{
		Name:           "release_name",
		Namespace:      "target_namespace",
		Project:        "argocd_project",
		RepoURL:        "repo_url",
		Chart:          "chart_ref",
		TargetRevision: "chart_target_revision",
	}

	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.Files[0]), `- release_name: hello-world
  target_namespace: hello-world
  argocd_project: test
  repo_url: internal.oci.repo/helm-int
  chart_ref: charts/hello-world
  chart_target_revision: 0.1.0
`)

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected one release, got %d", len(releases))
	}
	if releases[0].Name != "hello-world" || releases[0].Project != "test" || releases[0].Chart != "charts/hello-world" || releases[0].RepoURL != "internal.oci.repo/helm-int" || releases[0].TargetRevision != "0.1.0" {
		t.Fatalf("unexpected normalized release: %#v", releases[0])
	}
}

func TestLoadReleasesRequiresTargetRevisionForRemoteCharts(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.Files[0]), `- name: argocd
  namespace: argocd
  project: default
  repoURL: internal.oci.repo/helm-int
  chart: argo-cd
`)

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing targetRevision") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseChartReferenceComposesOCIRefFromRepoURL(t *testing.T) {
	release := Release{
		RepoURL:        "internal.oci.repo/helm-int",
		Chart:          "argo-cd",
		TargetRevision: "3.1.0",
	}

	if got := release.ChartReference(); got != "oci://internal.oci.repo/helm-int/argo-cd" {
		t.Fatalf("unexpected chart reference: %q", got)
	}
}

func TestLoadReleasesValidatesRequiredFields(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.Files[0]), `- name: hello-world
  chart: charts/hello-world
`)

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing namespace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesRejectsNonListApps(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(clusterDir, layout.Apps.Files[0]), "apps:\n  - name: hello-world\n")

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveOverridePathUsesPrimaryAndFallback(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}

	clusterDir := ClusterDir(root, layout, cluster)
	if err := os.MkdirAll(filepath.Join(clusterDir, "overrides", "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	primary := filepath.Join(clusterDir, "overrides", "test", "hello-world.yaml")
	writeFile(t, primary, "replicaCount: 3\n")

	got := ResolveOverridePath(root, layout, cluster, release)
	if got != primary {
		t.Fatalf("expected primary override path, got %q", got)
	}

	if err := os.Remove(primary); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	fallback := filepath.Join(clusterDir, "overrides", "hello-world.yaml")
	writeFile(t, fallback, "replicaCount: 2\n")

	got = ResolveOverridePath(root, layout, cluster, release)
	if got != fallback {
		t.Fatalf("expected fallback override path, got %q", got)
	}
}

func TestResolveOverridePathHonorsCustomPatterns(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.ClustersDir = "environments"
	layout.Apps.Files = []string{"releases.yaml"}
	layout.Overrides.Path = "values/{cluster}/{project}/{name}.yaml"
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}

	want := filepath.Join(root, "environments", "kube-bravo", "values", "kube-bravo", "test", "hello-world.yaml")
	writeFile(t, want, "replicaCount: 3\n")
	got := ResolveOverridePath(root, layout, cluster, release)
	if got != want {
		t.Fatalf("unexpected override path: got %q want %q", got, want)
	}
}

func TestResolveOverrideValueFilesIncludesCommonThenSpecific(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}
	clusterDir := ClusterDir(root, layout, cluster)

	common := filepath.Join(clusterDir, "overrides", "common.yaml")
	primary := filepath.Join(clusterDir, "overrides", "test", "hello-world.yaml")
	writeFile(t, common, "cluster:\n  name: kube-bravo\n")
	writeFile(t, primary, "replicaCount: 3\n")

	got := ResolveOverrideValueFiles(root, layout, cluster, release)
	if !slices.Equal(got, []string{common, primary}) {
		t.Fatalf("unexpected values files: got %v want %v", got, []string{common, primary})
	}
}

func TestResolveOverrideValueFilesIncludesCommonThenFallback(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}
	clusterDir := ClusterDir(root, layout, cluster)

	common := filepath.Join(clusterDir, "overrides", "common.yaml")
	fallback := filepath.Join(clusterDir, "overrides", "hello-world.yaml")
	writeFile(t, common, "cluster:\n  name: kube-bravo\n")
	writeFile(t, fallback, "replicaCount: 2\n")

	got := ResolveOverrideValueFiles(root, layout, cluster, release)
	if !slices.Equal(got, []string{common, fallback}) {
		t.Fatalf("unexpected values files: got %v want %v", got, []string{common, fallback})
	}
}

func TestResolveOverrideValueFilesAllowsOnlyCommonOrNone(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	cluster := "kube-bravo"
	release := Release{Name: "hello-world", Project: "test"}
	clusterDir := ClusterDir(root, layout, cluster)

	common := filepath.Join(clusterDir, "overrides", "common.yaml")
	writeFile(t, common, "cluster:\n  name: kube-bravo\n")
	if got := ResolveOverrideValueFiles(root, layout, cluster, release); !slices.Equal(got, []string{common}) {
		t.Fatalf("unexpected common-only values files: %v", got)
	}
	if err := os.Remove(common); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := ResolveOverrideValueFiles(root, layout, cluster, release); len(got) != 0 {
		t.Fatalf("expected no values files, got %v", got)
	}
}

func TestParseAppsFilesValidatesList(t *testing.T) {
	files, err := ParseAppsFiles("apps.yaml, apps-dev.yaml")
	if err != nil {
		t.Fatalf("ParseAppsFiles returned error: %v", err)
	}
	if !slices.Equal(files, []string{"apps.yaml", "apps-dev.yaml"}) {
		t.Fatalf("unexpected files: %v", files)
	}

	for _, value := range []string{"apps.yaml,", "/apps.yaml", "../apps.yaml", "apps.yaml,apps.yaml"} {
		if _, err := ParseAppsFiles(value); err == nil {
			t.Fatalf("expected ParseAppsFiles(%q) to fail", value)
		}
	}
}

func TestLoadReleasesMergesAppsFilesInPrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: hello-world
  namespace: prod
  project: default
  chart: charts/hello-world
`)
	writeFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), `- name: hello-world
  namespace: dev
  project: default
  chart: charts/hello-world-dev
- name: debug-app
  namespace: dev
  project: default
  chart: charts/debug-app
`)

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected two releases, got %d: %#v", len(releases), releases)
	}
	if releases[0].Name != "hello-world" || releases[0].Namespace != "prod" || releases[0].Chart != "charts/hello-world" {
		t.Fatalf("expected first file to win for duplicate release, got %#v", releases[0])
	}
	if releases[1].Name != "debug-app" {
		t.Fatalf("expected additional release from secondary file, got %#v", releases[1])
	}

	_, warnings, err := LoadReleasesWithWarnings(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleasesWithWarnings returned error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one duplicate warning, got %#v", warnings)
	}
	if warnings[0].ReleaseName != "hello-world" || !strings.Contains(warnings[0].Message, "apps-dev.yaml") {
		t.Fatalf("unexpected duplicate warning: %#v", warnings[0])
	}
}

func TestLoadReleasesSkipsEmptySecondaryAppsFile(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: hello-world
  namespace: prod
  project: default
  chart: charts/hello-world
`)
	writeFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), "")

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 || releases[0].Name != "hello-world" {
		t.Fatalf("expected release from apps.yaml only, got %#v", releases)
	}
}

func TestLoadReleasesErrorsWhenExistingAppsFilesContainNoReleases(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), "")

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected empty configured apps files error")
	}
	if !strings.Contains(err.Error(), `configured apps files for cluster "kube-bravo" contain no releases`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesIgnoresInvalidLowerPriorityDuplicate(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: hello-world
  namespace: prod
  project: default
  chart: charts/hello-world
`)
	writeFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), `- name: hello-world
  chart: charts/broken
`)

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 || releases[0].Namespace != "prod" {
		t.Fatalf("expected higher-priority release to win, got %#v", releases)
	}
}

func TestLoadReleasesRejectsDuplicateNamesInSameAppsFile(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps.yaml"), `- name: hello-world
  namespace: prod
  project: default
  chart: charts/hello-world
- name: hello-world
  namespace: dev
  project: default
  chart: charts/hello-world
`)

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected duplicate release error")
	}
	if !strings.Contains(err.Error(), `duplicate release name "hello-world"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadReleasesSkipsMissingAppsFiles(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}
	clusterDir := ClusterDir(root, layout, "kube-bravo")
	writeFile(t, filepath.Join(clusterDir, "apps-dev.yaml"), `- name: debug-app
  namespace: dev
  project: default
  chart: charts/debug-app
`)

	releases, err := LoadReleases(root, layout, "kube-bravo")
	if err != nil {
		t.Fatalf("LoadReleases returned error: %v", err)
	}
	if len(releases) != 1 || releases[0].Name != "debug-app" {
		t.Fatalf("unexpected releases: %#v", releases)
	}
}

func TestLoadReleasesErrorsWhenNoAppsFilesExist(t *testing.T) {
	root := t.TempDir()
	layout := Default().Layout
	layout.Apps.Files = []string{"apps.yaml", "apps-dev.yaml"}

	_, err := LoadReleases(root, layout, "kube-bravo")
	if err == nil {
		t.Fatal("expected missing apps files error")
	}
	if !strings.Contains(err.Error(), "none of the configured apps files exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
