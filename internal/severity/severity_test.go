package severity

import (
	"testing"

	"github.com/sohooo/moebius/internal/diff"
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

func TestAssess_ComponentBaselineKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      Input
		wantLevel  Level
		wantReason string
	}{
		{
			name:       "longhorn backup target is critical",
			input:      Input{Kind: "BackupTarget", Namespace: "longhorn-system", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "Longhorn BackupTarget changed",
		},
		{
			name:       "cilium clusterwide policy is critical",
			input:      Input{Kind: "CiliumClusterwideNetworkPolicy", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "Cilium clusterwide policy changed",
		},
		{
			name:       "gateway class is critical",
			input:      Input{Kind: "GatewayClass", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "GatewayClass changed",
		},
		{
			name:       "openbao auth is critical",
			input:      Input{Kind: "VaultAuth", Namespace: "openbao-system", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "OpenBao auth backend changed",
		},
		{
			name:       "cloudnativepg image catalog is critical",
			input:      Input{Kind: "ImageCatalog", Namespace: "database", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "CloudNativePG ImageCatalog changed",
		},
		{
			name:       "keycloak core config is critical",
			input:      Input{Kind: "Keycloak", Namespace: "sso", State: "changed"},
			wantLevel:  LevelCritical,
			wantReason: "Keycloak core config changed",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Assess(tc.input)
			if got.Level != tc.wantLevel {
				t.Fatalf("expected %s, got %s (%v)", tc.wantLevel, got.Level, got.Findings)
			}
			if len(got.Findings) == 0 || got.Findings[0].Reason != tc.wantReason {
				t.Fatalf("expected top finding %q, got %#v", tc.wantReason, got.Findings)
			}
		})
	}
}

func TestAssess_ComponentPathAwareRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      Input
		wantLevel  Level
		wantReason string
	}{
		{
			name: "longhorn replica reduction is high",
			input: Input{
				Kind:      "Volume",
				Namespace: "longhorn-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "numberOfReplicas"}},
					Old:   3,
					New:   2,
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Longhorn replica count changed 3 -> 2",
		},
		{
			name: "longhorn metadata stays low",
			input: Input{
				Kind:      "Volume",
				Namespace: "longhorn-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "metadata"}, {Key: "annotations"}, {Key: "team"}},
					Old:   "a",
					New:   "b",
				}},
			},
			wantLevel:  LevelLow,
			wantReason: "metadata changed at `metadata.annotations.team`",
		},
		{
			name: "cilium namespaced policy rule is high",
			input: Input{
				Kind:      "CiliumNetworkPolicy",
				Namespace: "networking",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "egress"}, {Index: intPtr(0)}, {Key: "toCIDR"}, {Index: intPtr(0)}},
					Old:   "10.0.0.0/24",
					New:   "0.0.0.0/0",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Cilium policy scope changed at `spec.egress[0].toCIDR[0]`",
		},
		{
			name: "cilium encryption is critical",
			input: Input{
				Kind:      "CiliumNodeConfig",
				Namespace: "kube-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "defaults"}, {Key: "encryption"}, {Key: "enabled"}},
					Old:   true,
					New:   false,
				}},
			},
			wantLevel:  LevelCritical,
			wantReason: "Cilium encryption setting changed",
		},
		{
			name: "gateway listener tls is high",
			input: Input{
				Kind:      "Gateway",
				Namespace: "gateway-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "listeners"}, {Index: intPtr(0)}, {Key: "tls"}, {Key: "certificateRefs"}},
					Old:   []interface{}{"old-cert"},
					New:   []interface{}{"new-cert"},
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Gateway listener TLS changed at `spec.listeners[0].tls.certificateRefs`",
		},
		{
			name: "httproute backend change is high",
			input: Input{
				Kind:      "HTTPRoute",
				Namespace: "gateway-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "rules"}, {Index: intPtr(0)}, {Key: "backendRefs"}, {Index: intPtr(0)}, {Key: "name"}},
					Old:   "api-v1",
					New:   "api-v2",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Gateway routing changed at `spec.rules[0].backendRefs[0].name`",
		},
		{
			name: "openbao policy rule is critical",
			input: Input{
				Kind:      "VaultPolicy",
				Namespace: "openbao-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "rules"}},
					Old:   "path \"secret/*\" { capabilities = [\"read\"] }",
					New:   "path \"secret/*\" { capabilities = [\"create\", \"read\"] }",
				}},
			},
			wantLevel:  LevelCritical,
			wantReason: "OpenBao policy rules changed",
		},
		{
			name: "openbao static secret sync is medium",
			input: Input{
				Kind:      "VaultStaticSecret",
				Namespace: "openbao-system",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "refreshAfter"}},
					Old:   "1h",
					New:   "30m",
				}},
			},
			wantLevel:  LevelMedium,
			wantReason: "OpenBao static secret sync changed",
		},
		{
			name: "cloudnativepg bootstrap change is critical",
			input: Input{
				Kind:      "Cluster",
				Namespace: "database",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "bootstrap"}, {Key: "recovery"}},
					Old:   nil,
					New:   map[string]interface{}{"source": "standby"},
				}},
			},
			wantLevel:  LevelCritical,
			wantReason: "CloudNativePG bootstrap mode changed",
		},
		{
			name: "cloudnativepg storage change is high",
			input: Input{
				Kind:      "Cluster",
				Namespace: "database",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "storage"}, {Key: "size"}},
					Old:   "100Gi",
					New:   "200Gi",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "CloudNativePG storage configuration changed",
		},
		{
			name: "cloudnativepg instance scaling is medium",
			input: Input{
				Kind:      "Cluster",
				Namespace: "database",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "instances"}},
					Old:   3,
					New:   4,
				}},
			},
			wantLevel:  LevelMedium,
			wantReason: "CloudNativePG instance count changed 3 -> 4",
		},
		{
			name: "keycloak hostname change is high",
			input: Input{
				Kind:      "Keycloak",
				Namespace: "sso",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "hostname"}, {Key: "hostname"}},
					Old:   "sso.example.com",
					New:   "login.example.com",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Keycloak hostname or TLS setting changed",
		},
		{
			name: "keycloak redirect uri is high",
			input: Input{
				Kind:      "KeycloakClient",
				Namespace: "sso",
				State:     "changed",
				Changes: []diff.Change{{
					State: "changed",
					Path:  []diff.Segment{{Key: "spec"}, {Key: "redirectUri"}},
					Old:   "https://app.example.com/callback",
					New:   "https://preview.example.com/callback",
				}},
			},
			wantLevel:  LevelHigh,
			wantReason: "Keycloak client redirect or origin changed",
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

func intPtr(v int) *int {
	return &v
}
