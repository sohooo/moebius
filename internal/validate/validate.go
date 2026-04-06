// Package validate performs offline validation of rendered Kubernetes resources.
package validate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xeipuuv/gojsonschema"

	"mobius/internal/resources"
)

type Status string
type Source string

const (
	StatusValid   Status = "valid"
	StatusWarning Status = "warning"
	StatusError   Status = "error"

	SourceStructural Source = "structural"
	SourceSchema     Source = "schema"
	SourceSemantic   Source = "semantic"
)

type Finding struct {
	Status    Status
	Source    Source
	Message   string
	Path      string
	SchemaRef string
}

type Result struct {
	Status   Status
	Findings []Finding
}

type Input struct {
	Resource   resources.Resource
	Siblings   map[string]resources.Resource
	Duplicates map[string]int
	Resolver   *SchemaResolver
}

type SemanticValidator interface {
	Supports(GVK) bool
	Validate(Input, GVK) []Finding
}

func Validate(in Input) Result {
	gvk := GVKFromResource(in.Resource)
	var findings []Finding

	findings = append(findings, structuralFindings(in, gvk)...)

	if in.Resource.Value != nil && gvk.Kind != "" && gvk.Version != "" {
		if schemaBytes, ref, ok := in.Resolver.Resolve(gvk); ok {
			findings = append(findings, schemaFindings(schemaBytes, ref, in.Resource.Value)...)
		}
	}

	for _, validator := range semanticValidators() {
		if validator.Supports(gvk) {
			findings = append(findings, validator.Validate(in, gvk)...)
		}
	}

	findings = dedupeFindings(findings)
	sort.SliceStable(findings, func(i, j int) bool {
		if statusRank(findings[i].Status) != statusRank(findings[j].Status) {
			return statusRank(findings[i].Status) > statusRank(findings[j].Status)
		}
		if findings[i].Source != findings[j].Source {
			return findings[i].Source < findings[j].Source
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Message < findings[j].Message
	})

	result := Result{Status: StatusValid, Findings: findings}
	for _, finding := range findings {
		if statusRank(finding.Status) > statusRank(result.Status) {
			result.Status = finding.Status
		}
	}
	return result
}

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

func schemaFindings(schemaBytes []byte, schemaRef string, value interface{}) []Finding {
	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	docLoader := gojsonschema.NewGoLoader(value)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return []Finding{{
			Status:    StatusError,
			Source:    SourceSchema,
			Message:   err.Error(),
			SchemaRef: schemaRef,
		}}
	}
	if result.Valid() {
		return nil
	}
	findings := make([]Finding, 0, len(result.Errors()))
	for _, issue := range result.Errors() {
		path := issue.Field()
		if path == "(root)" {
			path = ""
		}
		findings = append(findings, Finding{
			Status:    StatusError,
			Source:    SourceSchema,
			Message:   issue.Description(),
			Path:      path,
			SchemaRef: schemaRef,
		})
	}
	return findings
}

func dedupeFindings(in []Finding) []Finding {
	seen := map[string]struct{}{}
	out := make([]Finding, 0, len(in))
	for _, finding := range in {
		keyBytes, _ := json.Marshal(finding)
		key := string(keyBytes)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, finding)
	}
	return out
}

func statusRank(status Status) int {
	switch status {
	case StatusError:
		return 3
	case StatusWarning:
		return 2
	default:
		return 1
	}
}
