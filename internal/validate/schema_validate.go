package validate

import (
	"encoding/json"

	"github.com/xeipuuv/gojsonschema"
)

func schemaFindings(schema *gojsonschema.Schema, schemaRef string, value interface{}) []Finding {
	result, err := schema.Validate(gojsonschema.NewGoLoader(value))
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
		path := normalizeValidationPath(issue.Field())
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

func normalizeValidationPath(path string) string {
	if path == "(root)" {
		return ""
	}
	return path
}
