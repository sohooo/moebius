package severity

import (
	"fmt"
	"strings"

	"github.com/sohooo/moebius/internal/diff"
)

func componentBaselineFinding(in Input) (Finding, bool) {
	switch in.Kind {
	case "BackupTarget":
		return finding(LevelCritical, "storage", "Longhorn BackupTarget changed", "", "", ""), true
	case "Setting":
		return finding(LevelCritical, "platform", "Longhorn Setting changed", "", "", ""), true
	case "SystemBackup":
		return finding(LevelCritical, "platform", "Longhorn SystemBackup changed", "", "", ""), true
	case "SystemRestore":
		return finding(LevelCritical, "platform", "Longhorn SystemRestore changed", "", "", ""), true
	case "Volume":
		if in.State == "removed" {
			return finding(LevelCritical, "storage", "Longhorn Volume removed", "", "", ""), true
		}
		return finding(LevelHigh, "storage", "Longhorn Volume changed", "", "", ""), true
	case "EngineImage":
		return finding(LevelHigh, "storage", "Longhorn EngineImage changed", "", "", ""), true
	case "RecurringJob":
		return finding(LevelHigh, "storage", "Longhorn RecurringJob changed", "", "", ""), true
	case "Snapshot":
		return finding(LevelHigh, "storage", "Longhorn Snapshot changed", "", "", ""), true
	case "BackingImage", "BackupBackingImage", "BackupVolume", "Engine", "InstanceManager", "Orphan", "Replica", "ShareManager", "SupportBundle":
		return finding(LevelMedium, "storage", fmt.Sprintf("Longhorn %s changed", in.Kind), "", "", ""), true

	case "CiliumClusterwideNetworkPolicy":
		return finding(LevelCritical, "network", "Cilium clusterwide policy changed", "", "", ""), true
	case "CiliumBGPClusterConfig":
		return finding(LevelCritical, "network", "Cilium BGP cluster config changed", "", "", ""), true
	case "CiliumLoadBalancerIPPool":
		return finding(LevelCritical, "network", "Cilium load balancer IP pool changed", "", "", ""), true
	case "CiliumNetworkPolicy":
		return finding(LevelHigh, "network", "Cilium network policy changed", "", "", ""), true
	case "CiliumEnvoyConfig":
		return finding(LevelHigh, "network", "Cilium Envoy config changed", "", "", ""), true
	case "CiliumNodeConfig":
		return finding(LevelHigh, "network", "Cilium node config changed", "", "", ""), true
	case "CiliumEgressGatewayPolicy":
		return finding(LevelHigh, "network", "Cilium egress gateway policy changed", "", "", ""), true
	case "CiliumL2AnnouncementPolicy":
		return finding(LevelHigh, "network", "Cilium L2 announcement policy changed", "", "", ""), true
	case "CiliumCIDRGroup", "CiliumBGPPeerConfig", "CiliumBGPAdvertisement":
		return finding(LevelHigh, "network", fmt.Sprintf("Cilium %s changed", in.Kind), "", "", ""), true
	case "CiliumEndpointSlice":
		return finding(LevelMedium, "network", "Cilium endpoint slice changed", "", "", ""), true

	case "GatewayClass":
		return finding(LevelCritical, "network", "GatewayClass changed", "", "", ""), true
	case "SecurityPolicy":
		return finding(LevelCritical, "security", "Envoy Gateway security policy changed", "", "", ""), true
	case "ReferenceGrant":
		return finding(LevelCritical, "security", "Gateway API ReferenceGrant changed", "", "", ""), true
	case "Gateway", "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute", "EnvoyProxy", "BackendTrafficPolicy", "ClientTrafficPolicy", "EnvoyPatchPolicy", "BackendTLSPolicy":
		return finding(LevelHigh, categoryForKind(in.Kind), fmt.Sprintf("%s changed", in.Kind), "", "", ""), true

	case "VaultConnection":
		return finding(LevelCritical, "security", "OpenBao connection changed", "", "", ""), true
	case "VaultAuth":
		return finding(LevelCritical, "security", "OpenBao auth backend changed", "", "", ""), true
	case "VaultPolicy":
		return finding(LevelCritical, "security", "OpenBao policy changed", "", "", ""), true
	case "VaultRole":
		return finding(LevelCritical, "security", "OpenBao role changed", "", "", ""), true
	case "VaultDynamicSecret", "VaultPKISecret", "VaultPKISecretRole", "VaultTransitSecret", "VaultDatabaseSecret", "VaultWrite", "VaultTransformSecret":
		return finding(LevelHigh, "security", fmt.Sprintf("OpenBao %s changed", in.Kind), "", "", ""), true
	case "VaultStaticSecret":
		return finding(LevelMedium, "security", "OpenBao static secret sync changed", "", "", ""), true

	case "ImageCatalog":
		return finding(LevelCritical, "platform", "CloudNativePG ImageCatalog changed", "", "", ""), true
	case "ClusterImageCatalog":
		return finding(LevelCritical, "platform", "CloudNativePG ClusterImageCatalog changed", "", "", ""), true
	case "Pooler":
		return finding(LevelHigh, "workload", "CloudNativePG Pooler changed", "", "", ""), true

	case "Keycloak":
		return finding(LevelCritical, "security", "Keycloak core config changed", "", "", ""), true
	case "KeycloakBackup":
		return finding(LevelCritical, "security", "Keycloak backup changed", "", "", ""), true
	case "KeycloakRestore":
		return finding(LevelCritical, "security", "Keycloak restore changed", "", "", ""), true
	case "KeycloakClient":
		return finding(LevelCritical, "security", "Keycloak client config changed", "", "", ""), true
	case "KeycloakRealm":
		return finding(LevelCritical, "security", "Keycloak realm config changed", "", "", ""), true
	case "KeycloakRealmImport":
		return finding(LevelHigh, "security", "Keycloak realm import changed", "", "", ""), true
	case "KeycloakUser":
		return finding(LevelHigh, "security", "Keycloak user config changed", "", "", ""), true
	}
	return Finding{}, false
}

func componentPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case in.Kind == "Cluster" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case in.Kind == "Backup" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case in.Kind == "ScheduledBackup" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case in.Kind == "Database" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case in.Kind == "Publication" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case in.Kind == "Subscription" && isCloudNativePGPath(path):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case isLonghornKind(in.Kind):
		return longhornPathFindings(in, change, path, oldStr, newStr)
	case isCiliumKind(in.Kind):
		return ciliumPathFindings(in, change, path, oldStr, newStr)
	case isGatewayKind(in.Kind):
		return gatewayPathFindings(in, change, path, oldStr, newStr)
	case isOpenBaoKind(in.Kind):
		return openBaoPathFindings(in, change, path, oldStr, newStr)
	case isCloudNativePGKind(in.Kind):
		return cloudNativePGPathFindings(in, change, path, oldStr, newStr)
	case isKeycloakKind(in.Kind):
		return keycloakPathFindings(in, change, path, oldStr, newStr)
	default:
		return nil
	}
}

func longhornPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case path == "spec.backupTargetName" || strings.Contains(path, "backupTarget"):
		return []Finding{finding(LevelCritical, "storage", "Longhorn backup target changed", path, oldStr, newStr)}
	case strings.HasPrefix(path, "spec.numberOfReplicas"):
		level := LevelHigh
		reason := fmt.Sprintf("Longhorn replica count changed %s -> %s", oldStr, newStr)
		if oldInt, okOld := intValue(change.Old); okOld {
			if newInt, okNew := intValue(change.New); okNew && newInt >= oldInt {
				level = LevelMedium
			}
		}
		return []Finding{finding(level, "storage", reason, path, oldStr, newStr)}
	case strings.Contains(path, "replicaAutoBalance"):
		return []Finding{finding(LevelHigh, "storage", "Longhorn replica auto-balance setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "dataLocality"):
		return []Finding{finding(LevelHigh, "storage", "Longhorn data locality changed", path, oldStr, newStr)}
	case strings.Contains(path, "accessMode"):
		return []Finding{finding(LevelHigh, "storage", "Longhorn access mode changed", path, oldStr, newStr)}
	case strings.Contains(path, "migratable"):
		return []Finding{finding(LevelHigh, "storage", "Longhorn migratable setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "fromBackup"), strings.Contains(path, "snapshot"):
		return []Finding{finding(LevelHigh, "storage", fmt.Sprintf("Longhorn backup or snapshot setting changed at `%s`", path), path, oldStr, newStr)}
	case strings.Contains(path, "engineImage"):
		return []Finding{finding(LevelHigh, "storage", "Longhorn engine image changed", path, oldStr, newStr)}
	default:
		return nil
	}
}

func ciliumPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case strings.Contains(path, "endpointSelector"), strings.Contains(path, ".ingress"), strings.Contains(path, ".egress"), strings.Contains(path, "toCIDR"), strings.Contains(path, "toEndpoints"), strings.Contains(path, "fromEndpoints"), strings.Contains(path, "l7Rules"):
		level := LevelHigh
		reason := fmt.Sprintf("Cilium policy scope changed at `%s`", path)
		if in.Kind == "CiliumClusterwideNetworkPolicy" {
			level = LevelCritical
			reason = fmt.Sprintf("Cilium clusterwide policy scope changed at `%s`", path)
		}
		return []Finding{finding(level, "network", reason, path, oldStr, newStr)}
	case strings.Contains(path, "encryption"):
		return []Finding{finding(LevelCritical, "network", "Cilium encryption setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "bgp"):
		return []Finding{finding(LevelCritical, "network", "Cilium BGP configuration changed", path, oldStr, newStr)}
	case strings.Contains(path, "ipam"):
		return []Finding{finding(LevelHigh, "network", "Cilium IPAM setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "egressGateway"):
		return []Finding{finding(LevelHigh, "network", "Cilium egress gateway setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "kubeProxyReplacement"):
		return []Finding{finding(LevelHigh, "network", "Cilium kube-proxy replacement changed", path, oldStr, newStr)}
	case strings.Contains(path, "tunnel"):
		return []Finding{finding(LevelHigh, "network", "Cilium tunnel mode changed", path, oldStr, newStr)}
	case strings.Contains(path, "loadBalancer"), strings.Contains(path, "l2Announcements"):
		level := LevelHigh
		reason := fmt.Sprintf("Cilium load balancer setting changed at `%s`", path)
		if in.Kind == "CiliumLoadBalancerIPPool" {
			level = LevelCritical
			reason = "Cilium load balancer IP pool changed"
		}
		return []Finding{finding(level, "network", reason, path, oldStr, newStr)}
	default:
		return nil
	}
}

func gatewayPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case in.Kind == "GatewayClass" && strings.Contains(path, "controllerName"):
		return []Finding{finding(LevelCritical, "network", "GatewayClass controller changed", path, oldStr, newStr)}
	case in.Kind == "ReferenceGrant":
		return []Finding{finding(LevelCritical, "security", "Gateway API ReferenceGrant changed", path, oldStr, newStr)}
	case in.Kind == "SecurityPolicy":
		return []Finding{finding(LevelCritical, "security", "Envoy Gateway security policy changed", path, oldStr, newStr)}
	case strings.Contains(path, "listeners"):
		level := LevelHigh
		reason := fmt.Sprintf("Gateway listener changed at `%s`", path)
		if strings.Contains(path, "tls") || strings.Contains(path, "certificateRefs") {
			reason = fmt.Sprintf("Gateway listener TLS changed at `%s`", path)
		}
		return []Finding{finding(level, "network", reason, path, oldStr, newStr)}
	case strings.Contains(path, "hostnames"), strings.Contains(path, "hostname"):
		return []Finding{finding(LevelHigh, "network", fmt.Sprintf("Gateway hostname changed at `%s`", path), path, oldStr, newStr)}
	case strings.Contains(path, "matches"), strings.Contains(path, "filters"), strings.Contains(path, "backendRefs"):
		return []Finding{finding(LevelHigh, "network", fmt.Sprintf("Gateway routing changed at `%s`", path), path, oldStr, newStr)}
	case strings.Contains(path, "port"):
		return []Finding{finding(LevelHigh, "network", fmt.Sprintf("Gateway exposed listener or port changed at `%s`", path), path, oldStr, newStr)}
	default:
		return nil
	}
}

func openBaoPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case strings.Contains(path, "auth"), strings.Contains(path, "method"), strings.Contains(path, "mount"):
		return []Finding{finding(LevelCritical, "security", "OpenBao auth method changed", path, oldStr, newStr)}
	case path == "rules" || strings.Contains(path, ".rules"):
		return []Finding{finding(LevelCritical, "security", "OpenBao policy rules changed", path, oldStr, newStr)}
	case strings.Contains(path, "role"):
		return []Finding{finding(LevelCritical, "security", "OpenBao role binding changed", path, oldStr, newStr)}
	case strings.Contains(path, "address"), strings.Contains(path, "tls"):
		return []Finding{finding(LevelCritical, "security", "OpenBao connection or TLS setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "engine"), strings.Contains(path, "path"), strings.Contains(path, "mountPath"):
		return []Finding{finding(LevelHigh, "security", "OpenBao secret engine path changed", path, oldStr, newStr)}
	case strings.Contains(path, "lease"), strings.Contains(path, "rotation"):
		return []Finding{finding(LevelHigh, "security", "OpenBao lease or rotation setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "pki"), strings.Contains(path, "database"), strings.Contains(path, "transit"):
		return []Finding{finding(LevelHigh, "security", fmt.Sprintf("OpenBao secret backend setting changed at `%s`", path), path, oldStr, newStr)}
	default:
		return nil
	}
}

func cloudNativePGPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case in.Kind == "Backup":
		return []Finding{finding(LevelHigh, "storage", "CloudNativePG Backup changed", path, oldStr, newStr)}
	case in.Kind == "ScheduledBackup":
		return []Finding{finding(LevelHigh, "storage", "CloudNativePG ScheduledBackup changed", path, oldStr, newStr)}
	case in.Kind == "Publication", in.Kind == "Subscription":
		return []Finding{finding(LevelHigh, "platform", fmt.Sprintf("CloudNativePG %s changed", in.Kind), path, oldStr, newStr)}
	case in.Kind == "Database":
		return []Finding{finding(LevelMedium, "platform", "CloudNativePG Database changed", path, oldStr, newStr)}
	case strings.Contains(path, "bootstrap"):
		return []Finding{finding(LevelCritical, "platform", "CloudNativePG bootstrap mode changed", path, oldStr, newStr)}
	case strings.Contains(path, "externalClusters"):
		return []Finding{finding(LevelCritical, "platform", "CloudNativePG external cluster or replication source changed", path, oldStr, newStr)}
	case in.Kind == "ImageCatalog" || in.Kind == "ClusterImageCatalog":
		return []Finding{finding(LevelCritical, "platform", fmt.Sprintf("CloudNativePG %s changed", in.Kind), path, oldStr, newStr)}
	case strings.HasPrefix(path, "spec.instances"):
		level := LevelMedium
		reason := fmt.Sprintf("CloudNativePG instance count changed %s -> %s", oldStr, newStr)
		if oldInt, okOld := intValue(change.Old); okOld {
			if newInt, okNew := intValue(change.New); okNew && (newInt == 0 || newInt < oldInt) {
				level = LevelHigh
			}
		}
		return []Finding{finding(level, "capacity", reason, path, oldStr, newStr)}
	case strings.Contains(path, "storage"), strings.Contains(path, "walStorage"):
		return []Finding{finding(LevelHigh, "storage", "CloudNativePG storage configuration changed", path, oldStr, newStr)}
	case strings.Contains(path, "backup"):
		return []Finding{finding(LevelHigh, "storage", "CloudNativePG backup setting changed", path, oldStr, newStr)}
	case path == "spec.imageName" || strings.Contains(path, "imageName"):
		return []Finding{finding(LevelHigh, "workload", "CloudNativePG image changed", path, oldStr, newStr)}
	case strings.Contains(path, "primaryUpdateStrategy"), strings.Contains(path, "managed.roles"), strings.Contains(path, "monitoring"), strings.Contains(path, "affinity"):
		return []Finding{finding(LevelHigh, "platform", fmt.Sprintf("CloudNativePG operational setting changed at `%s`", path), path, oldStr, newStr)}
	default:
		return nil
	}
}

func keycloakPathFindings(in Input, change diff.Change, path, oldStr, newStr string) []Finding {
	switch {
	case strings.Contains(path, "restore"), strings.Contains(path, "backup"):
		return []Finding{finding(LevelCritical, "security", "Keycloak backup or restore setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "realm"), strings.Contains(path, "client"), strings.Contains(path, "authFlow"), strings.Contains(path, "provider"):
		return []Finding{finding(LevelCritical, "security", "Keycloak realm or client auth setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "hostname"), strings.Contains(path, "publicUrl"), strings.Contains(path, "ingress"), strings.Contains(path, "tls"):
		return []Finding{finding(LevelHigh, "network", "Keycloak hostname or TLS setting changed", path, oldStr, newStr)}
	case strings.Contains(path, "redirectUri"), strings.Contains(path, "webOrigins"):
		return []Finding{finding(LevelHigh, "security", "Keycloak client redirect or origin changed", path, oldStr, newStr)}
	case strings.Contains(path, "database"), strings.Contains(path, "ha"), strings.Contains(path, "replicas"):
		return []Finding{finding(LevelHigh, "workload", "Keycloak database or HA topology changed", path, oldStr, newStr)}
	default:
		return nil
	}
}

func isLonghornKind(kind string) bool {
	switch kind {
	case "BackingImage", "Backup", "BackupBackingImage", "BackupTarget", "BackupVolume", "Engine", "EngineImage", "InstanceManager", "Node", "Orphan", "RecurringJob", "Replica", "Setting", "ShareManager", "Snapshot", "SupportBundle", "SystemBackup", "SystemRestore", "Volume":
		return true
	default:
		return false
	}
}

func isCiliumKind(kind string) bool {
	switch kind {
	case "CiliumClusterwideNetworkPolicy", "CiliumNetworkPolicy", "CiliumCIDRGroup", "CiliumEgressGatewayPolicy", "CiliumEndpointSlice", "CiliumEnvoyConfig", "CiliumNodeConfig", "CiliumBGPClusterConfig", "CiliumBGPPeerConfig", "CiliumBGPAdvertisement", "CiliumLoadBalancerIPPool", "CiliumL2AnnouncementPolicy":
		return true
	default:
		return false
	}
}

func isGatewayKind(kind string) bool {
	switch kind {
	case "GatewayClass", "Gateway", "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute", "ReferenceGrant", "EnvoyProxy", "BackendTrafficPolicy", "ClientTrafficPolicy", "SecurityPolicy", "EnvoyPatchPolicy", "BackendTLSPolicy":
		return true
	default:
		return false
	}
}

func isOpenBaoKind(kind string) bool {
	switch kind {
	case "VaultConnection", "VaultAuth", "VaultStaticSecret", "VaultDynamicSecret", "VaultPKISecret", "VaultPKISecretRole", "VaultTransitSecret", "VaultPolicy", "VaultRole", "VaultDatabaseSecret", "VaultWrite", "VaultTransformSecret":
		return true
	default:
		return false
	}
}

func isCloudNativePGKind(kind string) bool {
	switch kind {
	case "Cluster", "Backup", "ScheduledBackup", "Pooler", "Database", "Publication", "Subscription", "ImageCatalog", "ClusterImageCatalog":
		return true
	default:
		return false
	}
}

func isCloudNativePGPath(path string) bool {
	return strings.Contains(path, "bootstrap") ||
		strings.Contains(path, "storage") ||
		strings.Contains(path, "walStorage") ||
		strings.Contains(path, "backup") ||
		strings.Contains(path, "imageName") ||
		strings.Contains(path, "externalClusters") ||
		strings.Contains(path, "instances") ||
		strings.Contains(path, "primaryUpdateStrategy") ||
		strings.Contains(path, "managed.roles") ||
		strings.Contains(path, "monitoring") ||
		strings.Contains(path, "affinity")
}

func isKeycloakKind(kind string) bool {
	switch kind {
	case "Keycloak", "KeycloakRealmImport", "KeycloakClient", "KeycloakRealm", "KeycloakUser", "KeycloakBackup", "KeycloakRestore":
		return true
	default:
		return false
	}
}
