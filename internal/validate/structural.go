package validate

import (
	"fmt"
	"strings"
)

func structuralFindings(in Input, gvk GVK) []Finding {
	var findings []Finding
	root, ok := in.Resource.Value.(map[string]interface{})
	if !ok {
		return []Finding{{
			Status:  StatusError,
			Source:  SourceStructural,
			Message: "resource is not a Kubernetes object",
		}}
	}
	if strings.TrimSpace(gvk.Version) == "" {
		findings = append(findings, Finding{Status: StatusError, Source: SourceStructural, Path: "apiVersion", Message: "required field missing"})
	}
	if strings.TrimSpace(gvk.Kind) == "" {
		findings = append(findings, Finding{Status: StatusError, Source: SourceStructural, Path: "kind", Message: "required field missing"})
	}
	meta, _ := root["metadata"].(map[string]interface{})
	if meta == nil {
		findings = append(findings, Finding{Status: StatusError, Source: SourceStructural, Path: "metadata", Message: "required field missing"})
	} else {
		if name, _ := meta["name"].(string); strings.TrimSpace(name) == "" {
			findings = append(findings, Finding{Status: StatusError, Source: SourceStructural, Path: "metadata.name", Message: "required field missing"})
		}
	}
	if count := in.Duplicates[in.Resource.Identity]; count > 1 {
		findings = append(findings, Finding{
			Status:  StatusError,
			Source:  SourceStructural,
			Message: fmt.Sprintf("duplicate resource identity detected (%d instances)", count),
		})
	}
	return findings
}
