package validate

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"mobius/internal/resources"
)

//go:embed schemas/index.json schemas/kubernetes/*/*.json schemas/platform/*/*/*.json
var schemaFS embed.FS

type GVK struct {
	Group   string
	Version string
	Kind    string
}

func (g GVK) Canonical() string {
	if g.Group == "" {
		return fmt.Sprintf("core/%s/%s", g.Version, g.Kind)
	}
	return fmt.Sprintf("%s/%s/%s", g.Group, g.Version, g.Kind)
}

type schemaIndex struct {
	Schemas map[string]string `json:"schemas"`
}

type SchemaResolver struct {
	embeddedOnce sync.Once
	embedded     map[string]string
	crdSchemas   map[string][]byte
}

func NewSchemaResolver(currentResources map[string]resources.Resource) *SchemaResolver {
	return &SchemaResolver{
		crdSchemas: extractCRDSchemas(currentResources),
	}
}

func (r *SchemaResolver) Resolve(gvk GVK) ([]byte, string, bool) {
	if schema, ok := r.crdSchemas[gvk.Canonical()]; ok {
		return schema, "rendered-crd:" + gvk.Canonical(), true
	}
	r.embeddedOnce.Do(func() {
		data, err := schemaFS.ReadFile("schemas/index.json")
		if err != nil {
			r.embedded = map[string]string{}
			return
		}
		var idx schemaIndex
		if err := json.Unmarshal(data, &idx); err != nil {
			r.embedded = map[string]string{}
			return
		}
		r.embedded = idx.Schemas
	})
	if path, ok := r.embedded[gvk.Canonical()]; ok {
		data, err := schemaFS.ReadFile(filepath.ToSlash(path))
		if err == nil {
			return data, "embedded:" + gvk.Canonical(), true
		}
	}
	return nil, "", false
}

func GVKFromResource(resource resources.Resource) GVK {
	group, version := parseAPIVersion(resource.APIVersion)
	return GVK{
		Group:   group,
		Version: version,
		Kind:    resource.Kind,
	}
}

func parseAPIVersion(apiVersion string) (string, string) {
	if apiVersion == "" {
		return "", ""
	}
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func extractCRDSchemas(currentResources map[string]resources.Resource) map[string][]byte {
	out := map[string][]byte{}
	for _, resource := range currentResources {
		if resource.Kind != "CustomResourceDefinition" {
			continue
		}
		root, ok := resource.Value.(map[string]interface{})
		if !ok {
			continue
		}
		spec, _ := root["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		group, _ := spec["group"].(string)
		names, _ := spec["names"].(map[string]interface{})
		kind, _ := names["kind"].(string)
		versions, _ := spec["versions"].([]interface{})
		for _, versionItem := range versions {
			versionMap, _ := versionItem.(map[string]interface{})
			if versionMap == nil {
				continue
			}
			served, _ := versionMap["served"].(bool)
			if !served {
				continue
			}
			version, _ := versionMap["name"].(string)
			schemaRoot, _ := versionMap["schema"].(map[string]interface{})
			openAPISchema, _ := schemaRoot["openAPIV3Schema"]
			if group == "" || version == "" || kind == "" || openAPISchema == nil {
				continue
			}
			schemaBytes, err := json.Marshal(openAPISchema)
			if err != nil {
				continue
			}
			out[GVK{Group: group, Version: version, Kind: kind}.Canonical()] = schemaBytes
		}
	}
	return out
}
