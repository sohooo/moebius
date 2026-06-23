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

func TestCompare_UnkeyedSliceAddRemoveAndChange(t *testing.T) {
	oldValue := map[string]interface{}{"ports": []interface{}{80, 443, 9443}}
	newValue := map[string]interface{}{"ports": []interface{}{80, 8443}}

	result, err := Compare("", "", oldValue, newValue, 1)
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	got := changeSummaries(result.Changes)
	want := []string{
		"changed ports[1]",
		"removed ports[2]",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected changes:\ngot  %v\nwant %v", got, want)
	}
}

func TestCompare_KeyedSliceFallsBackToIndexesWhenItemLacksMatchKey(t *testing.T) {
	oldValue := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"name": "a", "value": "old"},
		map[string]interface{}{"value": "unnamed"},
	}}
	newValue := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"name": "a", "value": "new"},
		map[string]interface{}{"value": "unnamed"},
	}}

	result, err := Compare("", "", oldValue, newValue, 1)
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if got, want := PathString(result.Changes[0].Path), "items[0].value"; got != want {
		t.Fatalf("expected index fallback path %q want %q", got, want)
	}
}

func TestCompare_KeyedSliceDuplicateMatchKeysUsesLastValue(t *testing.T) {
	oldValue := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"name": "a", "image": "old-first"},
		map[string]interface{}{"name": "a", "image": "old-last"},
	}}
	newValue := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"name": "a", "image": "new-last"},
	}}

	result, err := Compare("", "", oldValue, newValue, 1)
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("expected one change, got %#v", result.Changes)
	}
	if got, want := PathString(result.Changes[0].Path), "items[name=a].image"; got != want {
		t.Fatalf("unexpected path %q want %q", got, want)
	}
	if result.Changes[0].Old != "old-last" || result.Changes[0].New != "new-last" {
		t.Fatalf("expected duplicate key comparison to use last old value, got %#v", result.Changes[0])
	}
}

func TestRenderSnippetAndSemanticConsole(t *testing.T) {
	change := Change{
		State: "added",
		Path: []Segment{
			{Key: "spec"},
			{Key: "containers"},
			{MatchKey: "name", MatchValue: "app"},
			{Key: "image"},
		},
		New: "repo/app:v2",
	}

	snippet, err := RenderSnippet(change)
	if err != nil {
		t.Fatalf("RenderSnippet returned error: %v", err)
	}
	for _, needle := range []string{"containers:", "name: app", "image: repo/app:v2"} {
		if !strings.Contains(snippet, needle) {
			t.Fatalf("expected snippet to contain %q:\n%s", needle, snippet)
		}
	}
	console, err := RenderSemanticConsole([]Change{change})
	if err != nil {
		t.Fatalf("RenderSemanticConsole returned error: %v", err)
	}
	if !strings.Contains(console, "\033[32m") || !strings.Contains(console, "Path: spec.containers[name=app].image (added)") {
		t.Fatalf("expected colored console semantic output:\n%q", console)
	}
}

func TestFilterIgnoredChanges_DefaultHelmMetadata(t *testing.T) {
	changes := []Change{
		{State: "changed", Path: path("metadata", "labels", "app.kubernetes.io/version"), Old: "1.8.0", New: "1.8.1"},
		{State: "changed", Path: path("metadata", "labels", "helm.sh/chart"), Old: "outline-0.8.0", New: "outline-0.9.0"},
		{State: "changed", Path: path("spec", "template", "metadata", "annotations", "checksum/config"), Old: "old", New: "new"},
		{State: "changed", Path: path("spec", "template", "metadata", "annotations", "checksum/secret"), Old: "old", New: "new"},
		{State: "changed", Path: path("metadata", "labels", "team"), Old: "platform", New: "product"},
		{State: "changed", Path: path("spec", "replicas"), Old: 2, New: 3},
	}

	filtered := FilterIgnoredChanges(changes, IgnoreOptions{UseDefaults: true})

	got := changeSummaries(filtered)
	want := []string{
		"changed metadata.labels.team",
		"changed spec.replicas",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected filtered changes:\ngot  %v\nwant %v", got, want)
	}
}

func TestFilterIgnoredChanges_CustomMetadataRules(t *testing.T) {
	changes := []Change{
		{State: "changed", Path: path("spec", "jobTemplate", "spec", "template", "metadata", "annotations", "rollme"), Old: "a", New: "b"},
		{State: "changed", Path: path("metadata", "annotations", "rollme"), Old: "a", New: "b"},
	}

	filtered := FilterIgnoredChanges(changes, IgnoreOptions{
		Metadata: []MetadataIgnoreRule{{
			Locations:   []string{"spec.jobTemplate.spec.template.metadata"},
			Annotations: []string{"roll*"},
		}},
	})

	got := changeSummaries(filtered)
	want := []string{"changed metadata.annotations.rollme"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected filtered changes:\ngot  %v\nwant %v", got, want)
	}
}

func TestFilterIgnoredChanges_DoesNotHideWholeResourceAdds(t *testing.T) {
	changes := []Change{
		{State: "added", Path: path("metadata", "labels", "helm.sh/chart"), New: "outline-0.9.0"},
	}

	filtered := FilterIgnoredChanges(changes, IgnoreOptions{UseDefaults: true})

	if len(filtered) != 1 {
		t.Fatalf("expected added changes to remain visible, got %#v", filtered)
	}
}

func path(keys ...string) []Segment {
	out := make([]Segment, 0, len(keys))
	for _, key := range keys {
		out = append(out, Segment{Key: key})
	}
	return out
}

func changeSummaries(changes []Change) []string {
	out := make([]string, 0, len(changes))
	for _, change := range changes {
		out = append(out, change.State+" "+PathString(change.Path))
	}
	return out
}
