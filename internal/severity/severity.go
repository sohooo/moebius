// Package severity classifies Kubernetes manifest changes by criticality.
package severity

import (
	"fmt"
	"sort"
	"strings"

	"mobius/internal/diff"
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
