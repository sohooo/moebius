package validate

import "github.com/sohooo/moebius/internal/resources"

type Status string
type Source string
type Coverage string
type SchemaSource string

const (
	StatusValid   Status = "valid"
	StatusWarning Status = "warning"
	StatusError   Status = "error"

	SourceStructural Source = "structural"
	SourceSchema     Source = "schema"
	SourceSemantic   Source = "semantic"

	CoverageValidated   Coverage = "validated"
	CoverageUnvalidated Coverage = "unvalidated"

	SchemaSourceRenderedCRD SchemaSource = "rendered-crd"
	SchemaSourceEmbedded    SchemaSource = "embedded"
	SchemaSourceNone        SchemaSource = "none"
)

type Finding struct {
	Status    Status
	Source    Source
	Message   string
	Path      string
	SchemaRef string
}

type Result struct {
	Status       Status
	Coverage     Coverage
	SchemaSource SchemaSource
	Findings     []Finding
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
