// Package validate performs offline validation of rendered Kubernetes resources.
package validate

import (
	"sort"
)

func Validate(in Input) Result {
	gvk := GVKFromResource(in.Resource)
	var findings []Finding
	coverage := CoverageUnvalidated
	schemaSource := SchemaSourceNone

	findings = append(findings, structuralFindings(in, gvk)...)

	if in.Resource.Value != nil && gvk.Kind != "" && gvk.Version != "" {
		if schema, ref, source, ok := in.Resolver.Resolve(gvk); ok {
			coverage = CoverageValidated
			schemaSource = source
			findings = append(findings, schemaFindings(schema, ref, in.Resource.Value)...)
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

	result := Result{
		Status:       StatusValid,
		Coverage:     coverage,
		SchemaSource: schemaSource,
		Findings:     findings,
	}
	for _, finding := range findings {
		if statusRank(finding.Status) > statusRank(result.Status) {
			result.Status = finding.Status
		}
	}
	return result
}
