package report

import (
	"path/filepath"
	"testing"

	"github.com/sohooo/moebius/internal/config"
)

func TestPlanAffectedReleases(t *testing.T) {
	layout := config.Default().Layout
	cluster := "kube-bravo"

	baseline := map[string]config.Release{
		"outline": {
			Name:      "outline",
			Namespace: "demo",
			Project:   "default",
			Chart:     "charts/outline",
		},
		"keycloak": {
			Name:      "keycloak",
			Namespace: "demo",
			Project:   "default",
			Chart:     "charts/keycloak",
		},
	}
	current := map[string]config.Release{
		"outline": baseline["outline"],
		"keycloak": {
			Name:      "keycloak",
			Namespace: "demo",
			Project:   "default",
			Chart:     "charts/keycloak",
		},
	}

	root := t.TempDir()
	baselineRoot := t.TempDir()
	writeReportTestFile(t, filepath.Join(baselineRoot, "clusters/kube-bravo/overrides/default/outline.yaml"), "message: base\n")
	writeReportTestFile(t, filepath.Join(root, "clusters/kube-bravo/overrides/default/outline.yaml"), "message: current\n")
	selection, err := planAffectedReleases(root, baselineRoot, layout, cluster, true, true, []string{"clusters/kube-bravo/overrides/default/outline.yaml"}, baseline, current)
	if err != nil {
		t.Fatalf("planAffectedReleases returned error: %v", err)
	}
	if !selection.includes("outline") || selection.includes("keycloak") {
		t.Fatalf("expected only outline affected, got %#v", selection)
	}

	root = t.TempDir()
	baselineRoot = t.TempDir()
	selection, err = planAffectedReleases(root, baselineRoot, layout, cluster, true, true, []string{"charts/keycloak/templates/deployment.yaml"}, baseline, current)
	if err != nil {
		t.Fatalf("planAffectedReleases returned error: %v", err)
	}
	if selection.includes("outline") || !selection.includes("keycloak") {
		t.Fatalf("expected only keycloak affected by local chart change, got %#v", selection)
	}

	current = map[string]config.Release{
		"outline": baseline["outline"],
		"keycloak": {
			Name:      "keycloak",
			Namespace: "demo",
			Project:   "default",
			Chart:     "charts/keycloak",
		},
	}
	current["outline"] = config.Release{
		Name:           "outline",
		Namespace:      "demo",
		Project:        "default",
		Chart:          "outline",
		RepoURL:        "https://charts.example.test",
		TargetRevision: "1.2.4",
	}
	selection, err = planAffectedReleases(root, baselineRoot, layout, cluster, true, true, []string{"clusters/kube-bravo/apps.yaml"}, baseline, current)
	if err != nil {
		t.Fatalf("planAffectedReleases returned error: %v", err)
	}
	if !selection.includes("outline") || selection.includes("keycloak") {
		t.Fatalf("expected only outline affected by release attributes, got %#v", selection)
	}
}

func TestPlanAffectedReleasesFallsBackOnUnmappedClusterChange(t *testing.T) {
	layout := config.Default().Layout
	release := config.Release{Name: "outline", Namespace: "demo", Project: "default", Chart: "charts/outline"}
	releases := map[string]config.Release{"outline": release}
	selection, err := planAffectedReleaseDetails(t.TempDir(), t.TempDir(), layout, "kube-bravo", true, true, []string{"clusters/kube-bravo/unknown/input.yaml"}, releases, releases)
	if err != nil {
		t.Fatalf("planAffectedReleases returned error: %v", err)
	}
	if !selection.all {
		t.Fatalf("expected full-cluster fallback, got %#v", selection)
	}
	if selection.FallbackReason != "unmapped_cluster_change_full_cluster_fallback" {
		t.Fatalf("unexpected fallback reason %q", selection.FallbackReason)
	}
}
