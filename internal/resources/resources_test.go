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

	out, err := SplitRendered(rendered, dir)
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

	out, err := SplitRendered(rendered, dir)
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
