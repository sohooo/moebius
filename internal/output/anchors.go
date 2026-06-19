package output

import (
	"fmt"
	"strings"
)

func clusterAnchor(cluster string) string {
	return "cluster-" + anchorSlug(cluster)
}

func chartAnchor(cluster, chart string) string {
	return "chart-" + anchorSlug(cluster) + "-" + anchorSlug(chart)
}

func chartLinkAnchor(cluster, chart string, target renderTarget) string {
	if target == renderTargetDescription {
		return descriptionAnchor(descriptionChartHeading(cluster, chart))
	}
	return chartAnchor(cluster, chart)
}

func resourceAnchor(cluster, kind, name string) string {
	return "resource-" + anchorSlug(cluster) + "-" + anchorSlug(kind) + "-" + anchorSlug(name)
}

func resourceLinkAnchor(cluster, chart string, resource ResourceReport, target renderTarget) string {
	if target == renderTargetDescription {
		return descriptionAnchor(descriptionResourceHeading(cluster, chart, resource.Namespace, resource.Kind, resource.Name))
	}
	return resourceAnchor(cluster, resource.Kind, resource.Name)
}

func descriptionClusterHeading(cluster string) string {
	return fmt.Sprintf(":computer: %s", cluster)
}

func descriptionChartHeading(cluster, chart string) string {
	return fmt.Sprintf(":package: %s %s", cluster, chart)
}

func descriptionResourceHeading(cluster, chart, namespace, kind, name string) string {
	return fmt.Sprintf("`%s` · %s · %s/%s %s", cluster, chart, namespace, kind, name)
}

func descriptionAnchor(heading string) string {
	return "user-content-" + gitlabHeadingSlug(heading)
}

func anchorSlug(parts ...string) string {
	raw := strings.ToLower(strings.Join(parts, "-"))
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func gitlabHeadingSlug(heading string) string {
	raw := strings.ToLower(heading)
	var b strings.Builder
	for _, r := range raw {
		if r == ':' || r == '.' || r == '`' || r == '·' || r == '/' {
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-")
}
