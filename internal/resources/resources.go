// Package resources loads and splits rendered Kubernetes manifests.
package resources

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Resource struct {
	Key        string
	Identity   string
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Path       string
	Value      interface{}
}

type DuplicateKeyMode string

const (
	DuplicateKeyModeError        DuplicateKeyMode = "error"
	DuplicateKeyModeWarnLastWins DuplicateKeyMode = "warn-last-wins"
)

type SplitOptions struct {
	DuplicateKeyMode DuplicateKeyMode
}

type DuplicateKeyError struct {
	Document    int
	Key         string
	Line        int
	PreviousLine int
	Path        string
}

func (e DuplicateKeyError) Error() string {
	path := e.Path
	if path == "" {
		path = e.Key
	}
	return fmt.Sprintf("document %d: mapping key %q already defined at line %d (line %d, path %s)", e.Document, e.Key, e.PreviousLine, e.Line, path)
}

func LoadFile(path string) (Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Resource{}, err
	}
	var value interface{}
	if err := yaml.Unmarshal(data, &value); err != nil {
		return Resource{}, err
	}
	value = normalize(value)
	apiVersion, kind, name, namespace := metadata(value)
	key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return Resource{
		Key:        key,
		Identity:   canonicalIdentity(apiVersion, kind, namespace, name),
		APIVersion: apiVersion,
		Kind:       fallback(kind, "Unknown"),
		Name:       fallback(name, "unnamed"),
		Namespace:  namespace,
		Path:       path,
		Value:      value,
	}, nil
}

func LoadDir(resourceDir string) (map[string]Resource, error) {
	out := map[string]Resource{}
	entries, err := os.ReadDir(resourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(resourceDir, entry.Name())
		resource, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		out[resource.Key] = resource
	}
	return out, nil
}

func SplitRendered(rendered string, resourceDir string, opts SplitOptions) ([]Resource, []string, error) {
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		return nil, nil, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(rendered))
	var out []Resource
	var warnings []string
	index := 0
	identityCounts := map[string]int{}
	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}
		if len(doc.Content) == 0 {
			continue
		}
		value, docWarnings, err := nodeValue(doc.Content[0], opts.DuplicateKeyMode, "", index+1)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, docWarnings...)
		if isZeroYAMLDocument(value) {
			continue
		}
		value = normalize(value)
		apiVersion, kind, name, namespace := metadata(value)
		if kind == "" {
			kind = "Unknown"
		}
		if name == "" {
			name = fmt.Sprintf("doc-%d", index)
		}
		nsToken := namespace
		if nsToken == "" {
			nsToken = "cluster"
		}
		identity := canonicalIdentity(apiVersion, kind, namespace, name)
		key := fmt.Sprintf("%s--%s--%s", kind, nsToken, name)
		identityCounts[identity]++
		if identityCounts[identity] > 1 {
			key = fmt.Sprintf("%s--dup-%d", key, identityCounts[identity])
		}
		path := filepath.Join(resourceDir, key+".yaml")
		data, err := yaml.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, nil, err
		}
		out = append(out, Resource{
			Key:        key,
			Identity:   identity,
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
			Path:       path,
			Value:      value,
		})
		index++
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, warnings, nil
}

func metadata(value interface{}) (apiVersion string, kind string, name string, namespace string) {
	root, ok := value.(map[string]interface{})
	if !ok {
		return "", "", "", ""
	}
	apiVersion, _ = root["apiVersion"].(string)
	kind, _ = root["kind"].(string)
	meta, _ := root["metadata"].(map[string]interface{})
	if meta != nil {
		name, _ = meta["name"].(string)
		namespace, _ = meta["namespace"].(string)
	}
	return apiVersion, kind, name, namespace
}

func canonicalIdentity(apiVersion, kind, namespace, name string) string {
	return fmt.Sprintf("%s|%s|%s|%s", fallback(apiVersion, "unknown"), fallback(kind, "Unknown"), namespace, fallback(name, "unnamed"))
}

func isZeroYAMLDocument(value interface{}) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return len(typed) == 0
	case []interface{}:
		return len(typed) == 0
	default:
		return false
	}
}

func normalize(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = normalize(item)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = normalize(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = normalize(item)
		}
		return out
	default:
		return typed
	}
}

func nodeValue(node *yaml.Node, mode DuplicateKeyMode, path string, document int) (interface{}, []string, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil, nil
		}
		return nodeValue(node.Content[0], mode, path, document)
	case yaml.MappingNode:
		out := map[string]interface{}{}
		seenLines := map[string]int{}
		var warnings []string
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := keyNode.Value
			childPath := joinPath(path, key)
			value, childWarnings, err := nodeValue(valueNode, mode, childPath, document)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, childWarnings...)
			if previousLine, ok := seenLines[key]; ok {
				dupErr := DuplicateKeyError{
					Document:     document,
					Key:          key,
					Line:         keyNode.Line,
					PreviousLine: previousLine,
					Path:         childPath,
				}
				if mode == DuplicateKeyModeError {
					return nil, nil, dupErr
				}
				warnings = append(warnings, fmt.Sprintf("document %d: duplicate key %q at line %d overrides line %d (%s)", document, key, keyNode.Line, previousLine, childPath))
			}
			seenLines[key] = keyNode.Line
			out[key] = value
		}
		return out, warnings, nil
	case yaml.SequenceNode:
		out := make([]interface{}, 0, len(node.Content))
		var warnings []string
		for i, child := range node.Content {
			value, childWarnings, err := nodeValue(child, mode, joinPath(path, "["+strconv.Itoa(i)+"]"), document)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, childWarnings...)
			out = append(out, value)
		}
		return out, warnings, nil
	case yaml.AliasNode:
		if node.Alias == nil {
			return nil, nil, errors.New("yaml alias without target")
		}
		return nodeValue(node.Alias, mode, path, document)
	default:
		var out interface{}
		if err := node.Decode(&out); err != nil {
			return nil, nil, err
		}
		return normalize(out), nil, nil
	}
}

func joinPath(base, part string) string {
	if base == "" {
		return part
	}
	if strings.HasPrefix(part, "[") {
		return base + part
	}
	return base + "." + part
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
