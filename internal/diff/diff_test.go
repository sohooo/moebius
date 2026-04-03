package diff

import (
	"strings"
	"testing"
)

func TestCompare_KeyedArrayMatchingIgnoresOrder(t *testing.T) {
	oldValue := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "sidecar", "image": "repo/sidecar:v1"},
						map[string]interface{}{"name": "agent", "image": "repo/agent:v1"},
					},
				},
			},
		},
	}
	newValue := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "agent", "image": "repo/agent:v2"},
						map[string]interface{}{"name": "sidecar", "image": "repo/sidecar:v1"},
					},
				},
			},
		},
	}

	result, err := Compare("", "", oldValue, newValue, 3)
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 semantic change, got %d", len(result.Changes))
	}

	got := PathString(result.Changes[0].Path)
	want := "spec.template.spec.containers[name=agent].image"
	if got != want {
		t.Fatalf("unexpected path: got %q want %q", got, want)
	}
}

func TestRenderSemanticReport_CollapsesSharedContext(t *testing.T) {
	change := Change{
		State: "changed",
		Path: []Segment{
			{Key: "spec"},
			{Key: "replicas"},
		},
		Old: 2,
		New: 3,
	}

	text, err := RenderSemanticReport([]Change{change})
	if err != nil {
		t.Fatalf("RenderSemanticReport returned error: %v", err)
	}

	want := strings.TrimSpace(`
Path: spec.replicas (changed)
spec:
    replicas: 2
    replicas: 3
`)
	if strings.TrimSpace(text) != want {
		t.Fatalf("unexpected semantic report:\n%s", text)
	}
}

func TestRenderSemanticMarkdown_CollapsesSharedContext(t *testing.T) {
	change := Change{
		State: "changed",
		Path: []Segment{
			{Key: "cilium"},
			{Key: "hubble"},
			{Key: "ui"},
			{Key: "enabled"},
		},
		Old: false,
		New: true,
	}

	text, err := RenderSemanticMarkdown([]Change{change})
	if err != nil {
		t.Fatalf("RenderSemanticMarkdown returned error: %v", err)
	}

	want := strings.TrimSpace(`
# Path: cilium.hubble.ui.enabled (changed)
cilium:
    hubble:
        ui:
-             enabled: false
+             enabled: true
`)
	if strings.TrimSpace(text) != want {
		t.Fatalf("unexpected semantic markdown:\n%s", text)
	}
}
