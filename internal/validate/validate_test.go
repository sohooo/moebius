package validate

import (
	"testing"

	"github.com/sohooo/moebius/internal/resources"
)

func TestValidate_StructuralFindings(t *testing.T) {
	t.Parallel()

	resource := resources.Resource{
		Kind:       "Service",
		APIVersion: "v1",
		Identity:   "v1|Service||unnamed",
		Value: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{},
			"spec":       map[string]interface{}{"ports": []interface{}{map[string]interface{}{"port": 80}}},
		},
	}

	result := Validate(Input{
		Resource:   resource,
		Duplicates: map[string]int{resource.Identity: 2},
		Resolver:   NewSchemaResolver(nil),
	})
	if result.Status != StatusError {
		t.Fatalf("expected structural error, got %s (%v)", result.Status, result.Findings)
	}
	if len(result.Findings) < 2 {
		t.Fatalf("expected multiple findings, got %#v", result.Findings)
	}
	if result.Coverage != CoverageValidated {
		t.Fatalf("expected validated coverage, got %s", result.Coverage)
	}
	if result.SchemaSource != SchemaSourceEmbedded {
		t.Fatalf("expected embedded schema source, got %s", result.SchemaSource)
	}
}

func TestValidate_BuiltinSchemaValidation(t *testing.T) {
	t.Parallel()

	resource := resources.Resource{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "hello",
		Identity:   "apps/v1|Deployment|demo|hello",
		Value: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "hello", "namespace": "demo"},
			"spec":       map[string]interface{}{"replicas": "3"},
		},
	}

	result := Validate(Input{
		Resource:   resource,
		Duplicates: map[string]int{resource.Identity: 1},
		Resolver:   NewSchemaResolver(nil),
	})
	if result.Status != StatusError {
		t.Fatalf("expected schema error, got %s (%v)", result.Status, result.Findings)
	}
	assertHasSource(t, result, SourceSchema)
	if result.Coverage != CoverageValidated {
		t.Fatalf("expected validated coverage, got %s", result.Coverage)
	}
	if result.SchemaSource != SchemaSourceEmbedded {
		t.Fatalf("expected embedded schema source, got %s", result.SchemaSource)
	}
}

func TestValidate_RenderedCRDSchemaOverridesEmbedded(t *testing.T) {
	t.Parallel()

	crd := resources.Resource{
		APIVersion: "apiextensions.k8s.io/v1",
		Kind:       "CustomResourceDefinition",
		Name:       "widgets.example.com",
		Identity:   "apiextensions.k8s.io/v1|CustomResourceDefinition||widgets.example.com",
		Value: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": "widgets.example.com"},
			"spec": map[string]interface{}{
				"group": "example.com",
				"names": map[string]interface{}{"kind": "Widget"},
				"versions": []interface{}{
					map[string]interface{}{
						"name":   "v1",
						"served": true,
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"type":     "object",
								"required": []interface{}{"spec"},
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{"const": "example.com/v1"},
									"kind":       map[string]interface{}{"const": "Widget"},
									"metadata": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"name": map[string]interface{}{"type": "string"},
										},
									},
									"spec": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"size": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	resource := resources.Resource{
		APIVersion: "example.com/v1",
		Kind:       "Widget",
		Name:       "demo",
		Identity:   "example.com/v1|Widget|demo|demo",
		Value: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Widget",
			"metadata":   map[string]interface{}{"name": "demo", "namespace": "demo"},
			"spec":       map[string]interface{}{"size": "big"},
		},
	}

	resolver := NewSchemaResolver(map[string]resources.Resource{"crd": crd})
	result := Validate(Input{
		Resource:   resource,
		Duplicates: map[string]int{resource.Identity: 1},
		Resolver:   resolver,
	})
	if result.Status != StatusError {
		t.Fatalf("expected rendered CRD schema error, got %s (%v)", result.Status, result.Findings)
	}
	if len(result.Findings) == 0 || result.Findings[0].SchemaRef != "rendered-crd:example.com/v1/Widget" {
		t.Fatalf("expected rendered CRD schema ref, got %#v", result.Findings)
	}
	if result.Coverage != CoverageValidated {
		t.Fatalf("expected validated coverage, got %s", result.Coverage)
	}
	if result.SchemaSource != SchemaSourceRenderedCRD {
		t.Fatalf("expected rendered CRD source, got %s", result.SchemaSource)
	}
}

func TestValidate_EmbeddedPlatformSchemas(t *testing.T) {
	t.Parallel()

	tests := []resources.Resource{
		{
			APIVersion: "postgresql.cnpg.io/v1",
			Kind:       "Cluster",
			Name:       "db",
			Identity:   "postgresql.cnpg.io/v1|Cluster|data|db",
			Value: map[string]interface{}{
				"apiVersion": "postgresql.cnpg.io/v1",
				"kind":       "Cluster",
				"metadata":   map[string]interface{}{"name": "db", "namespace": "data"},
				"spec":       map[string]interface{}{"instances": "3"},
			},
		},
		{
			APIVersion: "cilium.io/v2",
			Kind:       "CiliumNetworkPolicy",
			Name:       "policy",
			Identity:   "cilium.io/v2|CiliumNetworkPolicy|net|policy",
			Value: map[string]interface{}{
				"apiVersion": "cilium.io/v2",
				"kind":       "CiliumNetworkPolicy",
				"metadata":   map[string]interface{}{"name": "policy", "namespace": "net"},
				"spec":       map[string]interface{}{"endpointSelector": "all"},
			},
		},
		{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "Gateway",
			Name:       "gw",
			Identity:   "gateway.networking.k8s.io/v1|Gateway|gw|gw",
			Value: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata":   map[string]interface{}{"name": "gw", "namespace": "gw"},
				"spec":       map[string]interface{}{"listeners": []interface{}{map[string]interface{}{"name": "https", "port": "443", "protocol": "HTTPS"}}},
			},
		},
		{
			APIVersion: "secrets.hashicorp.com/v1beta1",
			Kind:       "VaultConnection",
			Name:       "bao",
			Identity:   "secrets.hashicorp.com/v1beta1|VaultConnection|sec|bao",
			Value: map[string]interface{}{
				"apiVersion": "secrets.hashicorp.com/v1beta1",
				"kind":       "VaultConnection",
				"metadata":   map[string]interface{}{"name": "bao", "namespace": "sec"},
				"spec":       map[string]interface{}{"address": true},
			},
		},
		{
			APIVersion: "k8s.keycloak.org/v2alpha1",
			Kind:       "KeycloakClient",
			Name:       "client",
			Identity:   "k8s.keycloak.org/v2alpha1|KeycloakClient|sso|client",
			Value: map[string]interface{}{
				"apiVersion": "k8s.keycloak.org/v2alpha1",
				"kind":       "KeycloakClient",
				"metadata":   map[string]interface{}{"name": "client", "namespace": "sso"},
				"spec":       map[string]interface{}{"redirectUris": "https://example.com"},
			},
		},
		{
			APIVersion: "longhorn.io/v1beta2",
			Kind:       "Volume",
			Name:       "data",
			Identity:   "longhorn.io/v1beta2|Volume|longhorn-system|data",
			Value: map[string]interface{}{
				"apiVersion": "longhorn.io/v1beta2",
				"kind":       "Volume",
				"metadata":   map[string]interface{}{"name": "data", "namespace": "longhorn-system"},
				"spec":       map[string]interface{}{"numberOfReplicas": "3"},
			},
		},
	}

	for _, resource := range tests {
		resource := resource
		t.Run(resource.Kind, func(t *testing.T) {
			t.Parallel()
			result := Validate(Input{
				Resource:   resource,
				Duplicates: map[string]int{resource.Identity: 1},
				Resolver:   NewSchemaResolver(nil),
			})
			if result.Status != StatusError {
				t.Fatalf("expected schema error for %s, got %s (%v)", resource.Kind, result.Status, result.Findings)
			}
			if result.Coverage != CoverageValidated {
				t.Fatalf("expected validated coverage for %s, got %s", resource.Kind, result.Coverage)
			}
			if result.SchemaSource != SchemaSourceEmbedded {
				t.Fatalf("expected embedded schema source for %s, got %s", resource.Kind, result.SchemaSource)
			}
		})
	}
}

func TestValidate_SemanticValidators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource resources.Resource
	}{
		{
			name: "gateway https listener without tls",
			resource: resources.Resource{
				APIVersion: "gateway.networking.k8s.io/v1",
				Kind:       "Gateway",
				Name:       "gw",
				Identity:   "gateway.networking.k8s.io/v1|Gateway|gw|gw",
				Value: map[string]interface{}{
					"apiVersion": "gateway.networking.k8s.io/v1",
					"kind":       "Gateway",
					"metadata":   map[string]interface{}{"name": "gw", "namespace": "gw"},
					"spec": map[string]interface{}{
						"gatewayClassName": "envoy",
						"listeners":        []interface{}{map[string]interface{}{"name": "https", "port": 443, "protocol": "HTTPS"}},
					},
				},
			},
		},
		{
			name: "openbao skip tls verify",
			resource: resources.Resource{
				APIVersion: "secrets.hashicorp.com/v1beta1",
				Kind:       "VaultConnection",
				Name:       "bao",
				Identity:   "secrets.hashicorp.com/v1beta1|VaultConnection|sec|bao",
				Value: map[string]interface{}{
					"apiVersion": "secrets.hashicorp.com/v1beta1",
					"kind":       "VaultConnection",
					"metadata":   map[string]interface{}{"name": "bao", "namespace": "sec"},
					"spec":       map[string]interface{}{"address": "https://bao", "skipTLSVerify": true},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := Validate(Input{
				Resource:   tc.resource,
				Duplicates: map[string]int{tc.resource.Identity: 1},
				Resolver:   NewSchemaResolver(nil),
			})
			if result.Status != StatusWarning {
				t.Fatalf("expected warning, got %s (%v)", result.Status, result.Findings)
			}
			assertHasSource(t, result, SourceSemantic)
			if result.Coverage != CoverageValidated {
				t.Fatalf("expected validated coverage, got %s", result.Coverage)
			}
		})
	}
}

func TestValidate_MissingSchemaIsExplicitlyUnvalidated(t *testing.T) {
	t.Parallel()

	resource := resources.Resource{
		APIVersion: "example.com/v1",
		Kind:       "UnknownThing",
		Name:       "demo",
		Identity:   "example.com/v1|UnknownThing|demo|demo",
		Value: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "UnknownThing",
			"metadata":   map[string]interface{}{"name": "demo", "namespace": "demo"},
			"spec":       map[string]interface{}{"enabled": true},
		},
	}

	result := Validate(Input{
		Resource:   resource,
		Duplicates: map[string]int{resource.Identity: 1},
		Resolver:   NewSchemaResolver(nil),
	})

	if result.Coverage != CoverageUnvalidated {
		t.Fatalf("expected unvalidated coverage, got %s", result.Coverage)
	}
	if result.SchemaSource != SchemaSourceNone {
		t.Fatalf("expected no schema source, got %s", result.SchemaSource)
	}
}

func assertHasSource(t *testing.T, result Result, source Source) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.Source == source {
			return
		}
	}
	t.Fatalf("expected finding from %s, got %#v", source, result.Findings)
}
