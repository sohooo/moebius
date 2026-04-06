package validate

import (
	"fmt"
	"strings"
)

type validatorFunc struct {
	supports func(GVK) bool
	validate func(Input, GVK) []Finding
}

func (v validatorFunc) Supports(gvk GVK) bool                { return v.supports(gvk) }
func (v validatorFunc) Validate(in Input, gvk GVK) []Finding { return v.validate(in, gvk) }

func semanticValidators() []SemanticValidator {
	return []SemanticValidator{
		validatorFunc{supports: isGatewayGVK, validate: validateGatewaySemantic},
		validatorFunc{supports: isCiliumGVK, validate: validateCiliumSemantic},
		validatorFunc{supports: isCloudNativePGGVK, validate: validateCloudNativePGSemantic},
		validatorFunc{supports: isOpenBaoGVK, validate: validateOpenBaoSemantic},
		validatorFunc{supports: isLonghornGVK, validate: validateLonghornSemantic},
		validatorFunc{supports: isKeycloakGVK, validate: validateKeycloakSemantic},
	}
}

func validateGatewaySemantic(in Input, gvk GVK) []Finding {
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	switch gvk.Kind {
	case "Gateway":
		listeners, _ := spec["listeners"].([]interface{})
		var findings []Finding
		for i, item := range listeners {
			listener := asMap(item)
			protocol, _ := listener["protocol"].(string)
			if strings.EqualFold(protocol, "HTTPS") && listener["tls"] == nil {
				findings = append(findings, Finding{
					Status:  StatusWarning,
					Source:  SourceSemantic,
					Path:    fmt.Sprintf("spec.listeners[%d].tls", i),
					Message: "HTTPS listener is missing TLS configuration",
				})
			}
		}
		return findings
	case "HTTPRoute":
		rules, _ := spec["rules"].([]interface{})
		if len(rules) == 0 {
			return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec.rules", Message: "HTTPRoute has no routing rules"}}
		}
	}
	return nil
}

func validateCiliumSemantic(in Input, gvk GVK) []Finding {
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	if gvk.Kind == "CiliumNetworkPolicy" || gvk.Kind == "CiliumClusterwideNetworkPolicy" {
		if len(spec) == 0 {
			return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec", Message: "Cilium policy has no selector or rule content"}}
		}
	}
	return nil
}

func validateCloudNativePGSemantic(in Input, gvk GVK) []Finding {
	if gvk.Kind != "Cluster" {
		return nil
	}
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	var findings []Finding
	if bootstrap := asMap(spec["bootstrap"]); len(bootstrap) == 0 {
		findings = append(findings, Finding{Status: StatusWarning, Source: SourceSemantic, Path: "spec.bootstrap", Message: "CloudNativePG cluster has no explicit bootstrap configuration"})
	}
	if storage := asMap(spec["storage"]); len(storage) == 0 {
		findings = append(findings, Finding{Status: StatusWarning, Source: SourceSemantic, Path: "spec.storage", Message: "CloudNativePG cluster has no explicit storage configuration"})
	}
	return findings
}

func validateOpenBaoSemantic(in Input, gvk GVK) []Finding {
	if gvk.Kind != "VaultConnection" {
		return nil
	}
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	if skip, _ := spec["skipTLSVerify"].(bool); skip {
		return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec.skipTLSVerify", Message: "TLS verification is disabled for the OpenBao connection"}}
	}
	return nil
}

func validateLonghornSemantic(in Input, gvk GVK) []Finding {
	if gvk.Kind != "Volume" {
		return nil
	}
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	if _, ok := spec["numberOfReplicas"]; !ok {
		return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec.numberOfReplicas", Message: "Longhorn volume does not declare numberOfReplicas"}}
	}
	return nil
}

func validateKeycloakSemantic(in Input, gvk GVK) []Finding {
	root := asMap(in.Resource.Value)
	spec := asMap(root["spec"])
	switch gvk.Kind {
	case "Keycloak":
		if hostname := asMap(spec["hostname"]); len(hostname) == 0 && spec["hostname"] == nil {
			return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec.hostname", Message: "Keycloak does not declare a hostname"}}
		}
	case "KeycloakClient":
		if spec["redirectUri"] == nil && spec["redirectUris"] == nil {
			return []Finding{{Status: StatusWarning, Source: SourceSemantic, Path: "spec.redirectUri", Message: "Keycloak client has no redirect URI configured"}}
		}
	}
	return nil
}

func isGatewayGVK(gvk GVK) bool {
	return gvk.Group == "gateway.networking.k8s.io" ||
		(gvk.Group == "gateway.envoyproxy.io" && (gvk.Kind == "EnvoyProxy" || gvk.Kind == "BackendTrafficPolicy" || gvk.Kind == "ClientTrafficPolicy" || gvk.Kind == "SecurityPolicy" || gvk.Kind == "EnvoyPatchPolicy" || gvk.Kind == "BackendTLSPolicy"))
}

func isCiliumGVK(gvk GVK) bool {
	return gvk.Group == "cilium.io"
}

func isCloudNativePGGVK(gvk GVK) bool {
	return gvk.Group == "postgresql.cnpg.io"
}

func isOpenBaoGVK(gvk GVK) bool {
	return gvk.Group == "secrets.hashicorp.com"
}

func isLonghornGVK(gvk GVK) bool {
	return gvk.Group == "longhorn.io"
}

func isKeycloakGVK(gvk GVK) bool {
	return gvk.Group == "k8s.keycloak.org"
}

func asMap(value interface{}) map[string]interface{} {
	m, _ := value.(map[string]interface{})
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}
