package resources

import "testing"

func TestSplitRendered_UsesStableResourceKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
  namespace: demo
spec:
  replicas: 2
---
apiVersion: v1
kind: Namespace
metadata:
  name: kube-bravo
`

	out, _, err := SplitRendered(rendered, dir, SplitOptions{})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(out))
	}
	if out[0].Key != "Deployment--demo--hello-world" {
		t.Fatalf("unexpected namespaced key: %q", out[0].Key)
	}
	if out[1].Key != "Namespace--cluster--kube-bravo" {
		t.Fatalf("unexpected cluster-scoped key: %q", out[1].Key)
	}
}

func TestSplitRendered_UniquifiesDuplicateResourceKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: shared
  namespace: demo
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: shared
  namespace: demo
`

	out, _, err := SplitRendered(rendered, dir, SplitOptions{})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(out))
	}
	if out[0].Identity != out[1].Identity {
		t.Fatalf("expected duplicate resources to share identity, got %q and %q", out[0].Identity, out[1].Identity)
	}
	if out[0].Key == out[1].Key {
		t.Fatalf("expected duplicate resources to use distinct keys")
	}
}

func TestSplitRendered_WarnLastWinsForDuplicateMappingKeys(t *testing.T) {
	dir := t.TempDir()
	rendered := `apiVersion: v1
kind: Service
metadata:
  name: demo
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/scrape: "false"
`

	out, warnings, err := SplitRendered(rendered, dir, SplitOptions{DuplicateKeyMode: DuplicateKeyModeWarnLastWins})
	if err != nil {
		t.Fatalf("SplitRendered returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one resource, got %d", len(out))
	}
	metadata := out[0].Value.(map[string]interface{})["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	if annotations["prometheus.io/scrape"] != "false" {
		t.Fatalf("expected last value to win, got %#v", annotations["prometheus.io/scrape"])
	}
	if len(warnings) == 0 {
		t.Fatal("expected duplicate-key warning")
	}
}
