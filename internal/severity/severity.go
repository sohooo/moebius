// Package severity classifies Kubernetes manifest changes by criticality.
package severity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/diff"
)

type Level string

const (
	LevelCritical Level = "critical"
	LevelHigh     Level = "high"
	LevelMedium   Level = "medium"
	LevelLow      Level = "low"
	LevelInfo     Level = "info"
)

type Finding struct {
	Level    Level
	Reason   string
	Category string
	Path     string
	Old      string
	New      string
}

type Assessment struct {
	Level    Level
	Findings []Finding
}

type Input struct {
	Kind      string
	Name      string
	Namespace string
	State     string
	Changes   []diff.Change
}

func Assess(in Input) Assessment {
	var findings []Finding

	pathFindings := assessPathFindings(in)
	findings = append(findings, pathFindings...)

	if len(pathFindings) == 0 {
		findings = append(findings, baselineFindings(in)...)
	}

	findings = dedupeFindings(findings)
	sort.SliceStable(findings, func(i, j int) bool {
		if Rank(findings[i].Level) != Rank(findings[j].Level) {
			return Rank(findings[i].Level) > Rank(findings[j].Level)
		}
		if findings[i].Category != findings[j].Category {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].Reason < findings[j].Reason
	})

	level := LevelInfo
	if len(findings) > 0 {
		level = findings[0].Level
	}
	return Assessment{
		Level:    level,
		Findings: findings,
	}
}

func Rank(level Level) int {
	switch level {
	case LevelCritical:
		return 5
	case LevelHigh:
		return 4
	case LevelMedium:
		return 3
	case LevelLow:
		return 2
	default:
		return 1
	}
}

func baselineFindings(in Input) []Finding {
	var findings []Finding
	if in.Namespace == "" && in.State == "removed" {
		findings = append(findings, finding(LevelCritical, "platform", "cluster-scoped resource removed", "", "", ""))
	}
	if componentFinding, ok := componentBaselineFinding(in); ok {
		findings = append(findings, componentFinding)
		return findings
	}

	switch in.Kind {
	case "Namespace":
		findings = append(findings, finding(LevelCritical, "platform", "Namespace changed", "", "", ""))
	case "CustomResourceDefinition":
		findings = append(findings, finding(LevelCritical, "platform", "CustomResourceDefinition changed", "", "", ""))
	case "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration":
		findings = append(findings, finding(LevelCritical, "policy", fmt.Sprintf("%s changed", in.Kind), "", "", ""))
	case "ClusterRole", "ClusterRoleBinding":
		findings = append(findings, finding(LevelCritical, "security", fmt.Sprintf("%s changed", in.Kind), "", "", ""))
	case "StorageClass", "PriorityClass", "APIService", "PersistentVolume":
		findings = append(findings, finding(LevelCritical, "platform", fmt.Sprintf("%s changed", in.Kind), "", "", ""))
	case "NetworkPolicy":
		if in.State == "removed" {
			findings = append(findings, finding(LevelCritical, "network", "NetworkPolicy removed", "", "", ""))
		}
	case "PodSecurityPolicy":
		findings = append(findings, finding(LevelCritical, "security", "PodSecurityPolicy changed", "", "", ""))
	case "Role", "RoleBinding", "ServiceAccount", "Ingress", "Gateway", "HTTPRoute", "VirtualService", "DestinationRule", "AuthorizationPolicy", "PeerAuthentication", "PodDisruptionBudget", "HorizontalPodAutoscaler":
		findings = append(findings, finding(LevelHigh, categoryForKind(in.Kind), fmt.Sprintf("%s changed", in.Kind), "", "", ""))
	case "PersistentVolumeClaim":
		findings = append(findings, finding(LevelHigh, "storage", "PersistentVolumeClaim changed", "", "", ""))
	case "ConfigMap", "Secret", "Deployment", "StatefulSet", "DaemonSet", "Service", "CronJob", "Job":
		findings = append(findings, finding(LevelMedium, categoryForKind(in.Kind), fmt.Sprintf("%s changed", in.Kind), "", "", ""))
	default:
		if in.State == "added" && in.Namespace != "" {
			findings = append(findings, finding(LevelInfo, "metadata", "supporting resource added", "", "", ""))
		}
	}
	return findings
}

func assessPathFindings(in Input) []Finding {
	var findings []Finding
	for _, change := range in.Changes {
		path := diff.PathString(change.Path)
		oldStr := scalarString(change.Old)
		newStr := scalarString(change.New)

		if componentFindings := componentPathFindings(in, change, path, oldStr, newStr); len(componentFindings) > 0 {
			findings = append(findings, componentFindings...)
			continue
		}

		switch {
		case isMetadataOnlyPath(path):
			findings = append(findings, finding(LevelLow, "metadata", fmt.Sprintf("metadata changed at `%s`", path), path, oldStr, newStr))
		case path == "spec.replicas":
			level := LevelMedium
			if oldInt, okOld := intValue(change.Old); okOld {
				if newInt, okNew := intValue(change.New); okNew {
					if oldInt == 0 || newInt == 0 {
						level = LevelHigh
					} else {
						delta := percentDelta(oldInt, newInt)
						if delta >= 50 {
							level = LevelHigh
						}
					}
				}
			}
			findings = append(findings, finding(level, "capacity", fmt.Sprintf("replicas changed %s -> %s", oldStr, newStr), path, oldStr, newStr))
		case isImagePath(path):
			level := LevelHigh
			reason := fmt.Sprintf("image changed %s -> %s", oldStr, newStr)
			if registryChanged(oldStr, newStr) {
				reason = fmt.Sprintf("image registry changed %s -> %s", oldStr, newStr)
			}
			findings = append(findings, finding(level, "workload", reason, path, oldStr, newStr))
		case strings.HasSuffix(path, "imagePullPolicy"):
			findings = append(findings, finding(LevelMedium, "workload", fmt.Sprintf("image pull policy changed %s -> %s", oldStr, newStr), path, oldStr, newStr))
		case strings.Contains(path, ".resources.limits."):
			level := LevelMedium
			reason := fmt.Sprintf("resource limits changed at `%s`", path)
			if change.New == nil || newStr == "" {
				level = LevelHigh
				reason = fmt.Sprintf("resource limits removed at `%s`", path)
			}
			findings = append(findings, finding(level, "capacity", reason, path, oldStr, newStr))
		case strings.Contains(path, ".resources.requests."):
			level := LevelMedium
			reason := fmt.Sprintf("resource requests changed at `%s`", path)
			if requestReduced(oldStr, newStr) {
				reason = fmt.Sprintf("resource requests reduced at `%s`", path)
			}
			findings = append(findings, finding(level, "capacity", reason, path, oldStr, newStr))
		case path == "spec.type":
			level := LevelMedium
			reason := fmt.Sprintf("service type changed %s -> %s", oldStr, newStr)
			if (oldStr == "ClusterIP" || oldStr == "") && (newStr == "LoadBalancer" || newStr == "NodePort") {
				level = LevelHigh
			}
			findings = append(findings, finding(level, "network", reason, path, oldStr, newStr))
		case isIngressHostPath(path):
			findings = append(findings, finding(LevelHigh, "network", fmt.Sprintf("ingress host changed at `%s`", path), path, oldStr, newStr))
		case isIngressTLSPath(path):
			findings = append(findings, finding(LevelHigh, "network", fmt.Sprintf("ingress TLS changed at `%s`", path), path, oldStr, newStr))
		case isIngressPathPath(path):
			findings = append(findings, finding(LevelHigh, "network", fmt.Sprintf("ingress path changed at `%s`", path), path, oldStr, newStr))
		case strings.Contains(path, "securityContext"), path == "spec.serviceAccountName", path == "spec.hostNetwork", path == "spec.hostPID", path == "spec.hostIPC", strings.Contains(path, "privileged"), strings.Contains(path, "capabilities"), strings.Contains(path, "runAsUser"), strings.Contains(path, "runAsNonRoot"), strings.Contains(path, "automountServiceAccountToken"):
			findings = append(findings, finding(LevelHigh, "security", fmt.Sprintf("security-sensitive setting changed at `%s`", path), path, oldStr, newStr))
		case isProbePath(path):
			level := LevelMedium
			reason := fmt.Sprintf("probe configuration changed at `%s`", path)
			if change.New == nil || newStr == "" {
				level = LevelHigh
				reason = fmt.Sprintf("probe configuration removed at `%s`", path)
			}
			findings = append(findings, finding(level, "workload", reason, path, oldStr, newStr))
		case strings.Contains(path, "volumeClaimTemplates"), path == "spec.storageClassName", strings.Contains(path, "resources.requests.storage"), strings.Contains(path, "accessModes"):
			findings = append(findings, finding(LevelHigh, "storage", fmt.Sprintf("storage configuration changed at `%s`", path), path, oldStr, newStr))
		case path == "rules" || strings.HasPrefix(path, "rules[") || strings.Contains(path, ".rules"):
			level := LevelHigh
			if in.Kind == "ClusterRole" || in.Kind == "ClusterRoleBinding" {
				level = LevelCritical
			}
			findings = append(findings, finding(level, "security", fmt.Sprintf("RBAC rules changed at `%s`", path), path, oldStr, newStr))
		case strings.Contains(path, "clientConfig") || strings.Contains(path, "failurePolicy"):
			if in.Kind == "MutatingWebhookConfiguration" || in.Kind == "ValidatingWebhookConfiguration" {
				findings = append(findings, finding(LevelCritical, "policy", fmt.Sprintf("webhook policy changed at `%s`", path), path, oldStr, newStr))
			}
		case strings.Contains(path, ".env") || strings.Contains(path, ".command") || strings.Contains(path, ".args") || strings.Contains(path, ".tolerations") || strings.Contains(path, ".affinity") || strings.Contains(path, ".nodeSelector"):
			findings = append(findings, finding(LevelMedium, "workload", fmt.Sprintf("workload behavior changed at `%s`", path), path, oldStr, newStr))
		}
	}
	return findings
}

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

func finding(level Level, category, reason, path, old, new string) Finding {
	return Finding{
		Level:    level,
		Category: category,
		Reason:   reason,
		Path:     path,
		Old:      old,
		New:      new,
	}
}

func dedupeFindings(in []Finding) []Finding {
	seen := map[string]struct{}{}
	out := make([]Finding, 0, len(in))
	for _, item := range in {
		key := string(item.Level) + "|" + item.Category + "|" + item.Reason + "|" + item.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func isMetadataOnlyPath(path string) bool {
	return strings.HasPrefix(path, "metadata.labels") ||
		strings.HasPrefix(path, "metadata.annotations") ||
		path == "metadata.namespace" ||
		path == "metadata.name"
}

func isImagePath(path string) bool {
	return strings.Contains(path, ".image") && !strings.Contains(path, "imagePullPolicy")
}

func isIngressHostPath(path string) bool {
	return strings.Contains(path, ".host") || strings.Contains(path, ".hosts")
}

func isIngressTLSPath(path string) bool {
	return strings.Contains(path, ".tls")
}

func isIngressPathPath(path string) bool {
	return strings.Contains(path, ".paths") || strings.Contains(path, ".path")
}

func isProbePath(path string) bool {
	return strings.Contains(path, "livenessProbe") ||
		strings.Contains(path, "readinessProbe") ||
		strings.Contains(path, "startupProbe")
}

func categoryForKind(kind string) string {
	switch kind {
	case "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding", "ServiceAccount", "PodSecurityPolicy":
		return "security"
	case "Ingress", "Gateway", "HTTPRoute", "VirtualService", "DestinationRule", "AuthorizationPolicy", "PeerAuthentication", "Service", "NetworkPolicy":
		return "network"
	case "PersistentVolume", "PersistentVolumeClaim", "StorageClass":
		return "storage"
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "HorizontalPodAutoscaler", "PodDisruptionBudget":
		return "workload"
	case "Namespace", "CustomResourceDefinition", "PriorityClass", "APIService":
		return "platform"
	case "ConfigMap", "Secret":
		return "metadata"
	default:
		return "policy"
	}
}

func scalarString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func percentDelta(old, new int) int {
	if old == 0 {
		return 100
	}
	delta := old - new
	if delta < 0 {
		delta = -delta
	}
	return delta * 100 / old
}

func registryChanged(oldImage, newImage string) bool {
	return imageRegistry(oldImage) != imageRegistry(newImage)
}

func imageRegistry(image string) string {
	if image == "" {
		return ""
	}
	parts := strings.Split(image, "/")
	if len(parts) <= 1 {
		return "docker.io"
	}
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		return parts[0]
	}
	return "docker.io"
}

func requestReduced(oldStr, newStr string) bool {
	if oldStr == "" || newStr == "" {
		return false
	}
	if oldStr == newStr {
		return false
	}
	return len(newStr) < len(oldStr) || newStr < oldStr
}
