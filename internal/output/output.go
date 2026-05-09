// Package output renders cluster reports for terminals, markdown, and MR notes.
package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

const StickyMarker = "<!-- mobius:mr-diff -->"

type renderTarget int

const (
	renderTargetNote renderTarget = iota
	renderTargetDescription
)

type ResourceReport struct {
	State      string
	Kind       string
	Name       string
	Namespace  string
	Result     diff.Result
	Semantic   string
	Assessment severity.Assessment
	Validation validate.Result
}

type ChartReport struct {
	Name                   string
	Namespace              string
	Resources              []ResourceReport
	RenderWarning          string
	Warnings               []string
	BaselineTargetRevision string
	CurrentTargetRevision  string
	HasRemoteSource        bool
}

type ClusterReport struct {
	Name    string
	Charts  []ChartReport
	Added   int
	Removed int
	Changed int
}

type NoteMetadata struct {
	PipelineURL string
	JobURL      string
	CommitSHA   string
	BaseRef     string
	DiffMode    string
	GeneratedAt string
}

type NoteRenderOptions struct {
	Mode                 cli.CommentMode
	IncludeArtifactsHint bool
	Status               string
	target               renderTarget
}

func PrintReports(w io.Writer, reports []ClusterReport, mode diff.Mode, format cli.OutputFormat) error {
	text, err := RenderReports(reports, mode, format)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, text)
	return err
}

func RenderReports(reports []ClusterReport, mode diff.Mode, format cli.OutputFormat) (string, error) {
	var b strings.Builder
	for i, report := range reports {
		var chunk string
		var err error
		switch format {
		case cli.OutputFormatMarkdown:
			chunk, err = renderClusterMarkdown(report, mode)
		default:
			chunk, err = renderClusterPlain(report, mode)
		}
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(chunk)
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func chartChangeCounts(chart ChartReport) (added, removed, changed int) {
	for _, resource := range chart.Resources {
		switch resource.State {
		case "added":
			added++
		case "removed":
			removed++
		default:
			changed++
		}
	}
	return added, removed, changed
}

func cloneReports(reports []ClusterReport) []ClusterReport {
	out := make([]ClusterReport, len(reports))
	for i, report := range reports {
		out[i] = report
		out[i].Charts = append([]ChartReport(nil), report.Charts...)
		for j, chart := range out[i].Charts {
			out[i].Charts[j].Resources = append([]ResourceReport(nil), chart.Resources...)
		}
	}
	return out
}

func sortReportsForComment(reports []ClusterReport) {
	for i := range reports {
		sort.SliceStable(reports[i].Charts, func(a, b int) bool {
			left, right := reports[i].Charts[a], reports[i].Charts[b]
			if severity.Rank(chartSeverity(left)) != severity.Rank(chartSeverity(right)) {
				return severity.Rank(chartSeverity(left)) > severity.Rank(chartSeverity(right))
			}
			la, lr, lc := chartChangeCounts(left)
			ra, rr, rc := chartChangeCounts(right)
			if lr != rr {
				return lr > rr
			}
			if la != ra {
				return la > ra
			}
			lTotal := la + lr + lc
			rTotal := ra + rr + rc
			if lTotal != rTotal {
				return lTotal > rTotal
			}
			return left.Name < right.Name
		})
		for j := range reports[i].Charts {
			sort.SliceStable(reports[i].Charts[j].Resources, func(a, b int) bool {
				left, right := reports[i].Charts[j].Resources[a], reports[i].Charts[j].Resources[b]
				if validateStatusRank(left.Validation.Status) != validateStatusRank(right.Validation.Status) {
					return validateStatusRank(left.Validation.Status) > validateStatusRank(right.Validation.Status)
				}
				if severity.Rank(left.Assessment.Level) != severity.Rank(right.Assessment.Level) {
					return severity.Rank(left.Assessment.Level) > severity.Rank(right.Assessment.Level)
				}
				if stateWeight(left.State) != stateWeight(right.State) {
					return stateWeight(left.State) < stateWeight(right.State)
				}
				if left.Name != right.Name {
					return left.Name < right.Name
				}
				return left.Kind < right.Kind
			})
		}
	}
}

func stateWeight(state string) int {
	switch state {
	case "removed":
		return 0
	case "changed":
		return 1
	case "added":
		return 2
	default:
		return 3
	}
}

type reportStats struct {
	clusters             int
	charts               int
	resources            int
	added                int
	removed              int
	changed              int
	validationErrors     int
	validationWarnings   int
	unvalidatedResources int
	renderWarnings       int
	missingVersions      int
	otherRenderWarnings  int
	renderNotices        int
}

func collectReportStats(reports []ClusterReport) reportStats {
	stats := reportStats{clusters: len(reports)}
	for _, report := range reports {
		stats.charts += len(report.Charts)
		for _, chart := range report.Charts {
			if chart.RenderWarning != "" {
				stats.renderWarnings++
				if isMissingVersionRenderWarning(chart.RenderWarning) {
					stats.missingVersions++
				} else {
					stats.otherRenderWarnings++
				}
			}
			stats.renderNotices += len(chart.Warnings)
			added, removed, changed := chartChangeCounts(chart)
			stats.added += added
			stats.removed += removed
			stats.changed += changed
			stats.resources += added + removed + changed
			for _, resource := range chart.Resources {
				switch resource.Validation.Status {
				case validate.StatusError:
					stats.validationErrors++
				case validate.StatusWarning:
					stats.validationWarnings++
				}
				if resource.Validation.Coverage == validate.CoverageUnvalidated {
					stats.unvalidatedResources++
				}
			}
		}
	}
	return stats
}

func renderPartialAnalysisWarnings(b *strings.Builder, stats reportStats) {
	if stats.renderWarnings > 0 || stats.renderNotices > 0 {
		fmt.Fprintln(b, "> [!important]")
		fmt.Fprintln(b, "> Analysis is partial.")
		if stats.missingVersions > 0 {
			fmt.Fprintf(b, "> %d release(s) skipped because the requested chart version is unavailable.\n", stats.missingVersions)
			fmt.Fprintf(b, "**Missing chart versions:** %d skipped release(s)\n", stats.missingVersions)
		}
		if stats.otherRenderWarnings > 0 {
			fmt.Fprintf(b, "> %d release(s) skipped due to other render warnings.\n", stats.otherRenderWarnings)
			fmt.Fprintf(b, "**Other render warnings:** %d skipped release(s)\n", stats.otherRenderWarnings)
		}
		if stats.renderWarnings > 0 {
			fmt.Fprintln(b)
		}
		if stats.renderNotices > 0 {
			fmt.Fprintf(b, "> duplicate YAML keys accepted with last-wins behavior: %d override(s).\n", stats.renderNotices)
			fmt.Fprintf(b, "**Permissive YAML warnings:** %d duplicate-key override(s)\n\n", stats.renderNotices)
		} else {
			fmt.Fprintln(b)
		}
	}
}

func chartKinds(chart ChartReport) []string {
	if chart.RenderWarning != "" {
		return []string{"<unavailable>"}
	}
	set := map[string]struct{}{}
	for _, resource := range chart.Resources {
		set[resource.Kind] = struct{}{}
	}
	kinds := make([]string, 0, len(set))
	for kind := range set {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func onlyValueTweaks(chart ChartReport) bool {
	if chart.RenderWarning != "" {
		return false
	}
	for _, resource := range chart.Resources {
		if resource.State != "changed" {
			return false
		}
	}
	return true
}

func collectChartResourceChanges(cluster string, chart ChartReport, target renderTarget, linkResources bool) []string {
	if chart.RenderWarning != "" {
		return nil
	}
	var out []string
	for _, resource := range chart.Resources {
		line := primaryResourceHighlight(resource)
		if line == "" {
			line = resource.State
		}
		label := fmt.Sprintf("`%s/%s`", resource.Kind, resource.Name)
		if linkResources {
			label = fmt.Sprintf("[%s](#%s)", label, resourceLinkAnchor(cluster, chart.Name, resource, target))
		}
		out = append(out, fmt.Sprintf("%s %s **%s** · %s", severityIcon(resource.Assessment.Level), label, resource.Assessment.Level, line))
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func topFindings(resource ResourceReport, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, finding := range resource.Assessment.Findings {
		out = append(out, finding.Reason)
		if len(out) >= limit {
			break
		}
	}
	return dedupeStrings(out)
}

func topValidationFindings(resource ResourceReport, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, finding := range resource.Validation.Findings {
		line := finding.Message
		if finding.Path != "" {
			line = fmt.Sprintf("%s (%s)", line, finding.Path)
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return dedupeStrings(out)
}

func chartSeverity(chart ChartReport) severity.Level {
	if chart.RenderWarning != "" {
		return severity.LevelInfo
	}
	level := severity.LevelInfo
	for _, resource := range chart.Resources {
		if severity.Rank(resource.Assessment.Level) > severity.Rank(level) {
			level = resource.Assessment.Level
		}
	}
	return level
}

func chartSeverityCounts(chart ChartReport) map[severity.Level]int {
	if chart.RenderWarning != "" {
		return map[severity.Level]int{}
	}
	counts := map[severity.Level]int{}
	for _, resource := range chart.Resources {
		counts[resource.Assessment.Level]++
	}
	return counts
}

func formatSeveritySummary(counts map[severity.Level]int) string {
	order := []severity.Level{
		severity.LevelCritical,
		severity.LevelHigh,
		severity.LevelMedium,
		severity.LevelLow,
		severity.LevelInfo,
	}
	var parts []string
	for _, level := range order {
		if counts[level] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", level, counts[level]))
	}
	return strings.Join(parts, ", ")
}

func chartValidationCounts(chart ChartReport) (errors int, warnings int, unvalidated int) {
	if chart.RenderWarning != "" {
		return 0, 0, 0
	}
	for _, resource := range chart.Resources {
		switch resource.Validation.Status {
		case validate.StatusError:
			errors++
		case validate.StatusWarning:
			warnings++
		}
		if resource.Validation.Coverage == validate.CoverageUnvalidated {
			unvalidated++
		}
	}
	return errors, warnings, unvalidated
}

func validationSuffix(result validate.Result) string {
	if result.Status == "" || result.Status == validate.StatusValid {
		return ""
	}
	return fmt.Sprintf(", validation: %s", result.Status)
}

func validationCoverageLine(result validate.Result) string {
	switch result.Coverage {
	case validate.CoverageValidated:
		if result.SchemaSource == validate.SchemaSourceNone || result.SchemaSource == "" {
			return "validated"
		}
		return fmt.Sprintf("validated via %s", result.SchemaSource)
	case validate.CoverageUnvalidated:
		return "unvalidated (no schema available)"
	default:
		return ""
	}
}

func resourceMetadataLine(cluster, chart string, resource ResourceReport, target renderTarget) string {
	parts := []string{
		resource.State,
		fmt.Sprintf("severity %s", severityBadge(resource.Assessment.Level)),
	}
	if detail := validationCoverageLine(resource.Validation); detail != "" {
		parts = append(parts, "validation: "+detail)
	} else if resource.Validation.Status != "" && resource.Validation.Status != validate.StatusValid {
		parts = append(parts, "validation: "+string(resource.Validation.Status))
	}
	parts = append(parts, fmt.Sprintf("[up](#%s)", chartLinkAnchor(cluster, chart, target)))
	return strings.Join(parts, " · ")
}

func validateStatusRank(status validate.Status) int {
	switch status {
	case validate.StatusError:
		return 3
	case validate.StatusWarning:
		return 2
	default:
		return 1
	}
}

func isMissingVersionRenderWarning(warning string) bool {
	_, ok := missingVersionFromRenderWarning(warning)
	return ok
}

func missingVersionFromRenderWarning(warning string) (string, bool) {
	const marker = `requested chart version "`
	start := strings.Index(warning, marker)
	if start == -1 {
		return "", false
	}
	start += len(marker)
	end := strings.Index(warning[start:], `"`)
	if end == -1 {
		return "", false
	}
	version := strings.TrimSpace(warning[start : start+end])
	if version == "" {
		return "", false
	}
	return version, true
}

func chartRenderWarningSummary(warning string) string {
	if version, ok := missingVersionFromRenderWarning(warning); ok {
		return fmt.Sprintf("requested version %s unavailable", version)
	}
	return warning
}

func chartVersionSuffix(chart ChartReport) string {
	if versionChange := chartVersionChange(chart); versionChange != "" {
		return " · version " + versionChange
	}
	return ""
}

func chartVersionChange(chart ChartReport) string {
	if !chart.HasRemoteSource {
		return ""
	}
	if chart.BaselineTargetRevision == "" || chart.CurrentTargetRevision == "" || chart.BaselineTargetRevision == chart.CurrentTargetRevision {
		return ""
	}
	return fmt.Sprintf("%s → %s", chart.BaselineTargetRevision, chart.CurrentTargetRevision)
}

func renderChartSignalTable(b *strings.Builder, chart ChartReport, added, removed, changed int) {
	fmt.Fprintln(b, "| Signal | Details |")
	fmt.Fprintln(b, "| --- | --- |")
	fmt.Fprintf(b, "| **Summary** | %s |\n", escapeTable(chartSummaryDetails(chart, added, removed, changed)))
	if kinds := formatChartKinds(chart); kinds != "" {
		fmt.Fprintf(b, "| **Kinds** | %s |\n", escapeTable(kinds))
	}
	if chart.RenderWarning == "" {
		fmt.Fprintf(b, "| **Change mix** | %s |\n", escapeTable(formatChangeMix(added, removed, changed)))
		if surfaces := formatChartSurfaces(chart); surfaces != "" {
			fmt.Fprintf(b, "| **Surface** | %s |\n", escapeTable(surfaces))
		}
	}
	if onlyValueTweaks(chart) {
		fmt.Fprintln(b, "| **Scope** | value-level tweaks only |")
	}
	if summary := formatSeveritySummaryWithBadges(chartSeverityCounts(chart)); summary != "" {
		fmt.Fprintf(b, "| **Severity** | %s |\n", escapeTable(summary))
	}
	errors, warnings, unvalidated := chartValidationCounts(chart)
	if errors > 0 || warnings > 0 || unvalidated > 0 {
		fmt.Fprintf(b, "| **Validation** | %d errors · %d warnings · %d unvalidated |\n", errors, warnings, unvalidated)
	}
	fmt.Fprintln(b)
}

func chartSummaryLine(chart ChartReport, added, removed, changed int) string {
	parts := []string{fmt.Sprintf("Chart `%s`", chart.Name)}
	if versionChange := chartVersionChange(chart); versionChange != "" {
		parts = append(parts, "version "+versionChange)
	}
	parts = append(parts,
		fmt.Sprintf("namespace `%s`", emptyToNone(chart.Namespace)),
		fmt.Sprintf("severity `%s`", chartSeverity(chart)),
		fmt.Sprintf("added %d", added),
		fmt.Sprintf("removed %d", removed),
		fmt.Sprintf("changed %d", changed),
	)
	return strings.Join(parts, " · ")
}

func chartSummaryDetails(chart ChartReport, added, removed, changed int) string {
	var parts []string
	if versionChange := chartVersionChange(chart); versionChange != "" {
		parts = append(parts, "version "+versionChange)
	}
	total := added + removed + changed
	if chart.RenderWarning != "" {
		if version, ok := missingVersionFromRenderWarning(chart.RenderWarning); ok {
			parts = append(parts, fmt.Sprintf("requested version %s unavailable", version))
		} else {
			parts = append(parts, "render skipped")
		}
	} else if total > 0 {
		if total == 1 {
			parts = append(parts, "1 resource affected")
		} else {
			parts = append(parts, fmt.Sprintf("%d resources affected", total))
		}
	}
	parts = append(parts, "highest severity "+severityBadge(chartSeverity(chart)))
	if chart.RenderWarning != "" || len(chart.Warnings) > 0 {
		parts = append(parts, "analysis partial")
	}
	return strings.Join(parts, " · ")
}

func formatChartKinds(chart ChartReport) string {
	kinds := chartKinds(chart)
	if len(kinds) == 0 {
		return ""
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, fmt.Sprintf("`%s`", kind))
	}
	return strings.Join(out, ", ")
}

func formatSeveritySummaryWithBadges(counts map[severity.Level]int) string {
	order := []severity.Level{
		severity.LevelCritical,
		severity.LevelHigh,
		severity.LevelMedium,
		severity.LevelLow,
		severity.LevelInfo,
	}
	var parts []string
	for _, level := range order {
		if counts[level] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", severityBadge(level), counts[level]))
	}
	return strings.Join(parts, " · ")
}

func formatChangeFingerprint(added, removed, changed int, severityCounts map[severity.Level]int, schemaGaps int) string {
	if added == 0 && removed == 0 && changed == 0 && len(severityCounts) == 0 && schemaGaps == 0 {
		return ""
	}
	parts := []string{formatChangeMix(added, removed, changed)}
	if summary := formatSeveritySummaryWithBadges(severityCounts); summary != "" {
		parts = append(parts, summary)
	}
	if schemaGaps > 0 {
		parts = append(parts, fmt.Sprintf("schema gaps %d", schemaGaps))
	}
	return strings.Join(parts, " · ")
}

func formatChangeMix(added, removed, changed int) string {
	return fmt.Sprintf("+%d · -%d · ~%d", added, removed, changed)
}

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

func severityIcon(level severity.Level) string {
	return strings.Fields(severityBadge(level))[0]
}

func severityBadge(level severity.Level) string {
	switch level {
	case severity.LevelCritical:
		return "🔴 critical"
	case severity.LevelHigh:
		return "🟠 high"
	case severity.LevelMedium:
		return "🟡 medium"
	case severity.LevelLow:
		return "🟢 low"
	default:
		return "🔵 info"
	}
}

func escapeTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func primaryResourceHighlight(resource ResourceReport) string {
	if len(resource.Validation.Findings) > 0 && resource.Validation.Status != validate.StatusValid {
		line := resource.Validation.Findings[0].Message
		if resource.Validation.Findings[0].Path != "" {
			line = fmt.Sprintf("%s (%s)", line, resource.Validation.Findings[0].Path)
		}
		return fmt.Sprintf("validation %s: %s", resource.Validation.Status, line)
	}
	if len(resource.Assessment.Findings) > 0 {
		return resource.Assessment.Findings[0].Reason
	}
	return ""
}

func emptyToNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}
