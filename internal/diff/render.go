package diff

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
)

func RenderSnippet(change Change) (string, error) {
	value := change.New
	if change.State == "removed" {
		value = change.Old
	}
	return renderSnippetForValue(change.Path, value)
}

func renderSnippetForValue(path []Segment, value interface{}) (string, error) {
	tree := value
	for i := len(path) - 1; i >= 0; i-- {
		segment := path[i]
		switch {
		case segment.Key != "":
			tree = map[string]interface{}{segment.Key: tree}
		case segment.Index != nil:
			tree = []interface{}{tree}
		case segment.MatchKey != "":
			item := ensureMap(tree)
			injectMatchKey(item, segment.MatchKey, segment.MatchValue)
			tree = []interface{}{item}
		}
	}
	data, err := yaml.Marshal(tree)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func RenderSemanticReport(changes []Change) (string, error) {
	return renderSemanticReport(changes, false)
}

func RenderSemanticConsole(changes []Change) (string, error) {
	return renderSemanticReport(changes, true)
}

func RenderSemanticMarkdown(changes []Change) (string, error) {
	return renderSemanticReportMarkdown(changes)
}

func renderSemanticReport(changes []Change, color bool) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	var parts []string
	for _, change := range changes {
		body, err := renderChangeBody(change, color)
		if err != nil {
			return "", err
		}
		part := fmt.Sprintf("Path: %s (%s)\n%s", PathString(change.Path), change.State, body)
		parts = append(parts, part)
	}
	return strings.Join(parts, "\n\n"), nil
}

func renderSemanticReportMarkdown(changes []Change) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	var parts []string
	for _, change := range changes {
		body, ok := renderCollapsedMarkdownChange(change)
		if !ok {
			var lines []string
			if change.Old != nil {
				oldSnippet, err := renderSnippetForValue(change.Path, change.Old)
				if err != nil {
					return "", err
				}
				lines = append(lines, prefixBlock(oldSnippet, "- "))
			}
			if change.New != nil {
				newSnippet, err := renderSnippetForValue(change.Path, change.New)
				if err != nil {
					return "", err
				}
				lines = append(lines, prefixBlock(newSnippet, "+ "))
			}
			body = strings.Join(lines, "\n")
		}
		parts = append(parts, fmt.Sprintf("# Path: %s (%s)\n%s", PathString(change.Path), change.State, body))
	}
	return strings.Join(parts, "\n\n"), nil
}

func renderChangeBody(change Change, color bool) (string, error) {
	if body, ok := renderCollapsedChange(change, color); ok {
		return body, nil
	}

	var body []string
	if change.Old != nil {
		oldSnippet, err := renderSnippetForValue(change.Path, change.Old)
		if err != nil {
			return "", err
		}
		label := "Old:"
		if color {
			label = colorRed + label + colorReset
			oldSnippet = colorBlock(oldSnippet, colorRed)
		}
		body = append(body, label)
		body = append(body, oldSnippet)
	}
	if change.New != nil {
		newSnippet, err := renderSnippetForValue(change.Path, change.New)
		if err != nil {
			return "", err
		}
		label := "New:"
		if color {
			label = colorGreen + label + colorReset
			newSnippet = colorBlock(newSnippet, colorGreen)
		}
		body = append(body, label)
		body = append(body, newSnippet)
	}
	return strings.Join(body, "\n"), nil
}

func renderCollapsedChange(change Change, color bool) (string, bool) {
	if len(change.Path) == 0 {
		return "", false
	}

	last := change.Path[len(change.Path)-1]
	if last.Key == "" && last.Index == nil {
		return "", false
	}

	if change.Old != nil && !isScalarValue(change.Old) {
		return "", false
	}
	if change.New != nil && !isScalarValue(change.New) {
		return "", false
	}

	var lines []string
	indent := 0
	for _, segment := range change.Path[:len(change.Path)-1] {
		switch {
		case segment.Key != "":
			lines = append(lines, strings.Repeat(" ", indent)+segment.Key+":")
			indent += 4
		case segment.Index != nil:
			lines = append(lines, strings.Repeat(" ", indent)+"-")
			indent += 4
		case segment.MatchKey != "":
			lines = append(lines, strings.Repeat(" ", indent)+"- "+segment.MatchKey+": "+segment.MatchValue)
			indent += 4
		default:
			return "", false
		}
	}

	if change.Old != nil {
		line, ok := renderLeafLine(last, indent, change.Old)
		if !ok {
			return "", false
		}
		if color {
			line = colorRed + line + colorReset
		}
		lines = append(lines, line)
	}
	if change.New != nil {
		line, ok := renderLeafLine(last, indent, change.New)
		if !ok {
			return "", false
		}
		if color {
			line = colorGreen + line + colorReset
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), true
}

func renderCollapsedMarkdownChange(change Change) (string, bool) {
	if len(change.Path) == 0 {
		return "", false
	}

	last := change.Path[len(change.Path)-1]
	if last.Key == "" && last.Index == nil {
		return "", false
	}
	if change.Old != nil && !isScalarValue(change.Old) {
		return "", false
	}
	if change.New != nil && !isScalarValue(change.New) {
		return "", false
	}

	var lines []string
	indent := 0
	for _, segment := range change.Path[:len(change.Path)-1] {
		switch {
		case segment.Key != "":
			lines = append(lines, strings.Repeat(" ", indent)+segment.Key+":")
			indent += 4
		case segment.Index != nil:
			lines = append(lines, strings.Repeat(" ", indent)+"-")
			indent += 4
		case segment.MatchKey != "":
			lines = append(lines, strings.Repeat(" ", indent)+"- "+segment.MatchKey+": "+segment.MatchValue)
			indent += 4
		default:
			return "", false
		}
	}

	if change.Old != nil {
		line, ok := renderLeafLine(last, indent, change.Old)
		if !ok {
			return "", false
		}
		lines = append(lines, "- "+line)
	}
	if change.New != nil {
		line, ok := renderLeafLine(last, indent, change.New)
		if !ok {
			return "", false
		}
		lines = append(lines, "+ "+line)
	}
	return strings.Join(lines, "\n"), true
}

func renderLeafLine(segment Segment, indent int, value interface{}) (string, bool) {
	scalar, ok := renderScalarInline(value)
	if !ok {
		return "", false
	}
	prefix := strings.Repeat(" ", indent)
	switch {
	case segment.Key != "":
		return prefix + segment.Key + ": " + scalar, true
	case segment.Index != nil:
		return prefix + "- " + scalar, true
	default:
		return "", false
	}
}

func renderScalarInline(value interface{}) (string, bool) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(data))
	if strings.Contains(text, "\n") {
		return "", false
	}
	return text, true
}

func isScalarValue(value interface{}) bool {
	switch normalize(value).(type) {
	case nil, string, bool, int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func colorBlock(block string, color string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = color + line + colorReset
	}
	return strings.Join(lines, "\n")
}

func prefixBlock(block, prefix string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func ensureMap(value interface{}) map[string]interface{} {
	if out, ok := value.(map[string]interface{}); ok {
		return out
	}
	return map[string]interface{}{"value": value}
}

func injectMatchKey(item map[string]interface{}, key string, value string) {
	parts := strings.Split(key, ".")
	current := item
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			current[part] = next
		}
		current = next
	}
}
