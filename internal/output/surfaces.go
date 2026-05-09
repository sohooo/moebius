package output

import (
	"strings"

	"github.com/sohooo/moebius/internal/severity"
)

var surfaceOrder = []string{
	"security",
	"database",
	"ci/cd",
	"networking",
	"workload",
	"configuration",
	"storage",
	"policy",
	"platform",
	"observability",
	"custom",
}

func formatChartSurfaces(chart ChartReport) string {
	if chart.RenderWarning != "" {
		return ""
	}
	set := map[string]struct{}{}
	for _, resource := range chart.Resources {
		for _, surface := range resourceSurfaces(resource) {
			set[surface] = struct{}{}
		}
	}
	if len(set) == 0 {
		return ""
	}
	out := make([]string, 0, len(set))
	for _, surface := range surfaceOrder {
		if _, ok := set[surface]; ok {
			out = append(out, surface)
		}
	}
	return strings.Join(out, " · ")
}

func resourceSurfaces(resource ResourceReport) []string {
	primary := surfaceForKind(resource.Kind)
	set := map[string]struct{}{}
	if primary != "" {
		set[primary] = struct{}{}
	}
	for _, finding := range resource.Assessment.Findings {
		if surface := surfaceForFinding(finding); surface != "" {
			set[surface] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for _, surface := range surfaceOrder {
		if _, ok := set[surface]; ok {
			out = append(out, surface)
		}
	}
	return out
}

func surfaceForFinding(finding severity.Finding) string {
	reason := strings.ToLower(finding.Reason)
	switch {
	case strings.Contains(reason, "cloudnativepg"):
		return "database"
	case strings.Contains(reason, "argo cd") || strings.Contains(reason, "argocd"):
		return "ci/cd"
	case strings.Contains(reason, "keycloak") || strings.Contains(reason, "openbao") || strings.Contains(reason, "vault"):
		return "security"
	case strings.Contains(reason, "cilium") || strings.Contains(reason, "gateway"):
		return "networking"
	case strings.Contains(reason, "longhorn"):
		return "storage"
	}
	switch finding.Category {
	case "security":
		return "security"
	case "network":
		return "networking"
	case "workload", "capacity":
		return "workload"
	case "storage":
		return "storage"
	case "policy":
		return "policy"
	case "platform":
		return "platform"
	case "metadata":
		return "configuration"
	default:
		return ""
	}
}

func surfaceForKind(kind string) string {
	switch kind {
	case "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding", "ServiceAccount", "PodSecurityPolicy", "SecurityPolicy", "ReferenceGrant", "AuthorizationPolicy", "PeerAuthentication", "VaultConnection", "VaultAuth", "VaultStaticSecret", "VaultDynamicSecret", "VaultPKISecret", "VaultPKISecretRole", "VaultTransitSecret", "VaultPolicy", "VaultRole", "VaultDatabaseSecret", "VaultWrite", "VaultTransformSecret", "Keycloak", "KeycloakRealmImport", "KeycloakClient", "KeycloakRealm", "KeycloakUser", "KeycloakBackup", "KeycloakRestore", "Certificate", "CertificateRequest", "Issuer", "ClusterIssuer":
		return "security"
	case "Database", "Backup", "ScheduledBackup", "Pooler", "Publication", "Subscription", "ImageCatalog", "ClusterImageCatalog":
		return "database"
	case "Application", "ApplicationSet", "AppProject", "Rollout", "AnalysisRun", "AnalysisTemplate", "ClusterAnalysisTemplate", "Experiment":
		return "ci/cd"
	case "Service", "Ingress", "GatewayClass", "Gateway", "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute", "VirtualService", "DestinationRule", "NetworkPolicy", "CiliumClusterwideNetworkPolicy", "CiliumNetworkPolicy", "CiliumCIDRGroup", "CiliumEgressGatewayPolicy", "CiliumEndpointSlice", "CiliumEnvoyConfig", "CiliumNodeConfig", "CiliumBGPClusterConfig", "CiliumBGPPeerConfig", "CiliumBGPAdvertisement", "CiliumLoadBalancerIPPool", "CiliumL2AnnouncementPolicy", "EnvoyProxy", "BackendTrafficPolicy", "ClientTrafficPolicy", "EnvoyPatchPolicy", "BackendTLSPolicy":
		return "networking"
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "ReplicaSet":
		return "workload"
	case "ConfigMap", "Secret":
		return "configuration"
	case "PersistentVolume", "PersistentVolumeClaim", "StorageClass", "BackingImage", "BackupBackingImage", "BackupTarget", "BackupVolume", "Engine", "EngineImage", "InstanceManager", "Node", "Orphan", "RecurringJob", "Replica", "Setting", "ShareManager", "Snapshot", "SupportBundle", "SystemBackup", "SystemRestore", "Volume":
		return "storage"
	case "PodDisruptionBudget", "ResourceQuota", "LimitRange", "HorizontalPodAutoscaler", "VerticalPodAutoscaler", "PriorityClass", "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration":
		return "policy"
	case "Namespace", "CustomResourceDefinition", "APIService", "RuntimeClass", "ControllerRevision", "Lease":
		return "platform"
	case "ServiceMonitor", "PodMonitor", "PrometheusRule", "Probe", "AlertmanagerConfig", "Prometheus", "Alertmanager", "ThanosRuler", "GrafanaDashboard", "OpenTelemetryCollector", "Instrumentation":
		return "observability"
	default:
		return "custom"
	}
}
