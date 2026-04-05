package severity

import (
	"testing"

	"mobius/internal/diff"
)

func TestAssess_BaselineKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input Input
		want  Level
	}{
		{
			name:  "cluster role is critical",
			input: Input{Kind: "ClusterRole", State: "changed"},
			want:  LevelCritical,
		},
		{
			name:  "role is high",
			input: Input{Kind: "Role", Namespace: "demo", State: "changed"},
			want:  LevelHigh,
		},
		{
			name:  "configmap metadata only is low",
			input: Input{Kind: "ConfigMap", Namespace: "demo", State: "changed", Changes: []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "metadata"}, {Key: "annotations"}, {Key: "team"}}, Old: "a", New: "b"}}},
			want:  LevelLow,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Assess(tc.input)
			if got.Level != tc.want {
				t.Fatalf("expected %s, got %s (%v)", tc.want, got.Level, got.Findings)
			}
		})
	}
}

func TestAssess_PathAwareRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      Input
		wantLevel  Level
		wantReason string
	}{
		{
			name: "replicas change is medium",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "replicas"}},
					Old:   3,
					New:   5,
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "replicas changed 3 -> 5",
		},
		{
			name: "image tag change is high",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "image"}},
					Old:   "ghcr.io/acme/app:v1",
					New:   "ghcr.io/acme/app:v2",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "image changed ghcr.io/acme/app:v1 -> ghcr.io/acme/app:v2",
		},
		{
			name: "service type escalation is high",
			input: Input{
				Kind:      "Service",
				Namespace: "demo",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "type"}},
					Old:   "ClusterIP",
					New:   "LoadBalancer",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "service type changed ClusterIP -> LoadBalancer",
		},
		{
			name: "ingress tls change is high",
			input: Input{
				Kind:      "Ingress",
				Namespace: "demo",
				State:     "changed",
				Changes: []diff.Change{{
					State: "removed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "tls"}, {Index: intPtr(0)}},
					Old:   map[string]interface{}{"hosts": []interface{}{"example.com"}},
					New:   nil,
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "ingress TLS changed at `spec.tls[0]`",
		},
		{
			name: "probe removal is high",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes: []diff.Change{{
					State: "removed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "livenessProbe"}},
					Old:   map[string]interface{}{"httpGet": map[string]interface{}{"path": "/healthz"}},
					New:   nil,
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "probe configuration removed at `spec.template.spec.containers[name=app].livenessProbe`",
		},
		{
			name: "cluster role rules are critical",
			input: Input{
				Kind:  "ClusterRole",
				State: "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "rules"}},
					Old:   []interface{}{"get"},
					New:   []interface{}{"get", "list"},
				}},
			},
			wantLevel:  LevelCritical,
			wantReason: "RBAC rules changed at `rules`",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Assess(tc.input)
			if got.Level != tc.wantLevel {
				t.Fatalf("expected level %s, got %s (%v)", tc.wantLevel, got.Level, got.Findings)
			}
			if len(got.Findings) == 0 || got.Findings[0].Reason != tc.wantReason {
				t.Fatalf("expected top finding %q, got %#v", tc.wantReason, got.Findings)
			}
		})
	}
}

func TestAssess_ValueAwareRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     Input
		wantLevel Level
	}{
		{
			name: "scale to zero is high",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes:   []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "spec"}, {Key: "replicas"}}, Old: 3, New: 0}},
			},
			wantLevel: LevelHigh,
		},
		{
			name: "resource limit removal is high",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes:   []diff.Change{{State: "removed", Path: []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "resources"}, {Key: "limits"}, {Key: "cpu"}}, Old: "500m", New: nil}},
			},
			wantLevel: LevelHigh,
		},
		{
			name: "request reduction is medium",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes:   []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "resources"}, {Key: "requests"}, {Key: "memory"}}, Old: "512Mi", New: "256Mi"}},
			},
			wantLevel: LevelMedium,
		},
		{
			name: "image registry change is high",
			input: Input{
				Kind:      "Deployment",
				Namespace: "demo",
				State:     "changed",
				Changes:   []diff.Change{{State: "changed", Path: []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "image"}}, Old: "ghcr.io/acme/app:v1", New: "docker.io/library/app:v1"}},
			},
			wantLevel: LevelHigh,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Assess(tc.input); got.Level != tc.wantLevel {
				t.Fatalf("expected %s, got %s (%v)", tc.wantLevel, got.Level, got.Findings)
			}
		})
	}
}

func TestAssess_AggregatesHighestSeverity(t *testing.T) {
	t.Parallel()

	assessment := Assess(Input{
		Kind:      "Deployment",
		Namespace: "demo",
		State:     "changed",
		Changes: []diff.Change{
			{State: "changed", Path: []diff.Segment{{Key: "metadata"}, {Key: "labels"}, {Key: "app"}}, Old: "v1", New: "v2"},
			{State: "changed", Path: []diff.Segment{{Key: "spec"}, {Key: "template"}, {Key: "spec"}, {Key: "containers"}, {MatchKey: "name", MatchValue: "app"}, {Key: "image"}}, Old: "ghcr.io/acme/app:v1", New: "docker.io/library/app:v1"},
		},
	})

	if assessment.Level != LevelHigh {
		t.Fatalf("expected highest severity high, got %s", assessment.Level)
	}
	if len(assessment.Findings) < 2 {
		t.Fatalf("expected multiple findings, got %#v", assessment.Findings)
	}
	if assessment.Findings[0].Level != LevelHigh {
		t.Fatalf("expected highest-severity finding first, got %#v", assessment.Findings)
	}
}

func intPtr(v int) *int {
	return &v
}
