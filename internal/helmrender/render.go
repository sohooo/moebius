package helmrender

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/releaseutil"
)

type Renderer struct {
	cacheDir string
}

type MissingVersionError struct {
	ChartRef        string
	RepoURL         string
	TargetRevision  string
	UnderlyingError error
}

func (e *MissingVersionError) Error() string {
	if e == nil {
		return ""
	}
	ref := e.ChartRef
	if ref == "" {
		ref = "<unknown>"
	}
	if e.TargetRevision == "" {
		return fmt.Sprintf("chart version unavailable for %s: %v", ref, e.UnderlyingError)
	}
	return fmt.Sprintf("chart version %q unavailable for %s: %v", e.TargetRevision, ref, e.UnderlyingError)
}

func (e *MissingVersionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.UnderlyingError
}

func IsMissingVersionError(err error) bool {
	var target *MissingVersionError
	return errors.As(err, &target)
}

func New(cacheDir string) *Renderer {
	return &Renderer{cacheDir: cacheDir}
}

func (r *Renderer) Render(root string, chartRef string, repoURL string, targetRevision string, releaseName string, namespace string, overridePath string) (string, error) {
	ch, err := r.loadChart(root, chartRef, repoURL, targetRevision)
	if err != nil {
		return "", err
	}

	values := map[string]interface{}{}
	if overridePath != "" {
		if _, err := os.Stat(overridePath); err == nil {
			values, err = chartutil.ReadValuesFile(overridePath)
			if err != nil {
				return "", fmt.Errorf("read overrides %s: %w", overridePath, err)
			}
		}
	}

	options := chartutil.ReleaseOptions{
		Name:      releaseName,
		Namespace: namespace,
		Revision:  1,
		IsInstall: true,
	}
	renderValues, err := chartutil.ToRenderValues(ch, values, options, nil)
	if err != nil {
		return "", fmt.Errorf("prepare render values for %s: %w", releaseName, err)
	}
	return renderChart(ch, renderValues)
}

func (r *Renderer) loadChart(root string, chartRef string, repoURL string, targetRevision string) (*chart.Chart, error) {
	if strings.HasPrefix(chartRef, "oci://") {
		if targetRevision == "" {
			return nil, fmt.Errorf("oci chart %q requires targetRevision", chartRef)
		}
		settings := cli.New()
		settings.RepositoryConfig = filepath.Join(r.cacheDir, "repositories.yaml")
		settings.RepositoryCache = filepath.Join(r.cacheDir, "repository")
		if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
			return nil, err
		}

		registryClient, err := registry.NewClient()
		if err != nil {
			return nil, err
		}
		install := action.NewInstall(&action.Configuration{RegistryClient: registryClient})
		install.SetRegistryClient(registryClient)
		install.Version = targetRevision
		chartPath, err := install.ChartPathOptions.LocateChart(chartRef, settings)
		if err != nil {
			return nil, classifyLocateChartError(chartRef, "", targetRevision, err)
		}
		return loader.Load(chartPath)
	}

	if repoURL != "" {
		if targetRevision == "" {
			return nil, fmt.Errorf("remote chart %q requires targetRevision", chartRef)
		}
		settings := cli.New()
		settings.RepositoryConfig = filepath.Join(r.cacheDir, "repositories.yaml")
		settings.RepositoryCache = filepath.Join(r.cacheDir, "repository")
		if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
			return nil, err
		}

		install := action.NewInstall(&action.Configuration{})
		install.Version = targetRevision
		install.ChartPathOptions.RepoURL = repoURL
		chartPath, err := install.ChartPathOptions.LocateChart(chartRef, settings)
		if err != nil {
			return nil, classifyLocateChartError(chartRef, repoURL, targetRevision, err)
		}
		return loader.Load(chartPath)
	}

	return loader.Load(filepath.Join(root, chartRef))
}

func renderChart(ch *chart.Chart, values chartutil.Values) (string, error) {
	if err := chartutil.ProcessDependencies(ch, values); err != nil {
		return "", err
	}

	renderedFiles, err := engine.Render(ch, values)
	if err != nil {
		return "", err
	}

	manifestMap := make(map[string]string)
	for name, content := range renderedFiles {
		if strings.HasSuffix(name, "NOTES.txt") {
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		manifestMap[name] = content
	}

	hooks, manifests, err := releaseutil.SortManifests(manifestMap, chartutil.VersionSet{}, releaseutil.InstallOrder)
	if err != nil {
		return "", err
	}
	_ = hooks

	var ordered []string
	for _, manifest := range manifests {
		content := strings.TrimSpace(manifest.Content)
		if content != "" {
			ordered = append(ordered, content)
		}
	}
	if len(ordered) == 0 {
		names := make([]string, 0, len(manifestMap))
		for name := range manifestMap {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ordered = append(ordered, strings.TrimSpace(manifestMap[name]))
		}
	}

	var buf bytes.Buffer
	for i, content := range ordered {
		if i > 0 {
			buf.WriteString("\n---\n")
		}
		buf.WriteString(content)
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}

func classifyLocateChartError(chartRef, repoURL, targetRevision string, err error) error {
	if !looksLikeMissingChartVersion(err) {
		return err
	}
	return &MissingVersionError{
		ChartRef:        chartRef,
		RepoURL:         repoURL,
		TargetRevision:  targetRevision,
		UnderlyingError: err,
	}
}

func looksLikeMissingChartVersion(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	needles := []string{
		"chart version not found",
		"no chart version found",
		"could not find a version",
		"no matching version",
		"version not found",
		"tag not found",
		"manifest unknown",
		"not found",
	}
	if !strings.Contains(message, "version") && !strings.Contains(message, "tag") && !strings.Contains(message, "manifest unknown") {
		return false
	}
	for _, needle := range needles {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
