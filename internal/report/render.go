package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/helmrender"
	"github.com/sohooo/moebius/internal/resources"
)

func renderCluster(root string, layout config.LayoutConfig, cluster, state, outputRoot string, selection releaseSelection, renderer *helmrender.Renderer, mode cli.RenderErrorMode, duplicateKeyMode cli.DuplicateKeyMode) error {
	if !anyAppsFileExists(root, layout, cluster) {
		return nil
	}
	releases, releaseWarnings, err := config.LoadReleasesWithWarnings(root, layout, cluster)
	if err != nil {
		return err
	}
	warningsByRelease := map[string][]string{}
	for _, warning := range releaseWarnings {
		warningsByRelease[warning.ReleaseName] = append(warningsByRelease[warning.ReleaseName], warning.Message)
	}
	clusterDir := filepath.Join(outputRoot, cluster)
	clusterDirCreated := false

	for _, release := range releases {
		if !selection.includes(release.Name) {
			continue
		}
		if !clusterDirCreated {
			if err := os.MkdirAll(clusterDir, 0o755); err != nil {
				return err
			}
			clusterDirCreated = true
		}
		valuesFiles := config.ResolveOverrideValueFiles(root, layout, cluster, release)
		chartRef := release.ChartReference()
		rendered, err := renderer.RenderWithValuesFiles(root, chartRef, release.RepoURL, release.TargetRevision, release.Name, release.Namespace, valuesFiles)
		if err != nil {
			warning := renderFailureWarning(cluster, release, chartRef, state, err)
			if writeErr := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "warnings"), state, cluster, release.Name, []string{warning}); writeErr != nil {
				return writeErr
			}
			if mode == cli.RenderErrorModeWarnSkipRelease {
				if err := writeSkippedReleaseWarning(clusterDir, release, warning); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("render cluster %q release %q: %w", cluster, release.Name, err)
		}

		chartDir := filepath.Join(clusterDir, release.Name)
		resourceDir := filepath.Join(chartDir, "resources")
		if err := os.MkdirAll(resourceDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(chartDir, "namespace.txt"), []byte(release.Namespace+"\n"), 0o644); err != nil {
			return err
		}
		renderedPath := filepath.Join(chartDir, "rendered.yaml")
		if err := os.WriteFile(renderedPath, []byte(rendered), 0o644); err != nil {
			return err
		}
		_, notices, err := resources.SplitRendered(rendered, resourceDir, resources.SplitOptions{
			DuplicateKeyMode: optsDuplicateMode(duplicateKeyMode),
		})
		if err != nil {
			message := fmt.Sprintf("cluster %q release %q chart %q produced invalid %s rendered YAML (rendered manifest: %s): %v", cluster, release.Name, chartRef, state, renderedPath, err)
			if writeErr := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "errors"), state, cluster, release.Name, []string{message}); writeErr != nil {
				return writeErr
			}
			if mode == cli.RenderErrorModeWarnSkipRelease {
				if err := os.WriteFile(filepath.Join(chartDir, renderWarningFilename), []byte(message+"\n"), 0o644); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("%s", message)
		}
		notices = append(warningsByRelease[release.Name], notices...)
		if len(notices) > 0 {
			lines := make([]string, 0, len(notices))
			for _, notice := range notices {
				lines = append(lines, fmt.Sprintf("cluster %q release %q chart %q %s render warning: %s", cluster, release.Name, chartRef, state, notice))
			}
			if err := os.WriteFile(filepath.Join(chartDir, renderNoticeFilename), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				return err
			}
			if err := writeArtifactMessage(filepath.Join(filepath.Dir(outputRoot), "warnings"), state, cluster, release.Name, lines); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderFailureWarning(cluster string, release config.Release, chartRef, state string, err error) string {
	if warning, ok := missingVersionRenderWarning(cluster, release, chartRef, err); ok {
		return warning
	}
	return fmt.Sprintf("cluster %q release %q chart %q failed to render %s manifests: %v", cluster, release.Name, chartRef, state, err)
}

func writeSkippedReleaseWarning(clusterDir string, release config.Release, warning string) error {
	chartDir := filepath.Join(clusterDir, release.Name)
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(chartDir, "namespace.txt"), []byte(release.Namespace+"\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(chartDir, renderWarningFilename), []byte(warning+"\n"), 0o644)
}

func missingVersionRenderWarning(cluster string, release config.Release, chartRef string, err error) (string, bool) {
	var versionErr *helmrender.MissingVersionError
	if !helmrender.IsMissingVersionError(err) {
		return "", false
	}
	if !errors.As(err, &versionErr) || versionErr == nil {
		return "", false
	}
	message := fmt.Sprintf(
		"cluster %q release %q chart %q requested chart version %q is unavailable",
		cluster,
		release.Name,
		chartRef,
		versionErr.TargetRevision,
	)
	if versionErr.RepoURL != "" {
		message += fmt.Sprintf(" (repo %q)", versionErr.RepoURL)
	}
	message += fmt.Sprintf(": %v", versionErr.Unwrap())
	return message, true
}

func optsDuplicateMode(mode cli.DuplicateKeyMode) resources.DuplicateKeyMode {
	if mode == cli.DuplicateKeyModeWarnLastWins {
		return resources.DuplicateKeyModeWarnLastWins
	}
	return resources.DuplicateKeyModeError
}
