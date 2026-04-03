package diff

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeRaw      Mode = "raw"
	ModeSemantic Mode = "semantic"
	ModeBoth     Mode = "both"
)

type Segment struct {
	Key        string
	Index      *int
	MatchKey   string
	MatchValue string
}

type Change struct {
	State string
	Path  []Segment
	Old   interface{}
	New   interface{}
}

type Result struct {
	Changes    []Change
	RawDiff    string
	HasChanges bool
}

func Compare(oldPath, newPath string, oldValue, newValue interface{}, contextLines int) (Result, error) {
	rawDiff, changed, err := rawUnifiedDiff(oldPath, newPath, contextLines)
	if err != nil {
		return Result{}, err
	}
	changes := compareValues(nil, oldValue, newValue)
	return Result{Changes: changes, RawDiff: rawDiff, HasChanges: changed || len(changes) > 0}, nil
}

func rawUnifiedDiff(oldPath, newPath string, contextLines int) (string, bool, error) {
	oldData, err := readOptionalFile(oldPath)
	if err != nil {
		return "", false, err
	}
	newData, err := readOptionalFile(newPath)
	if err != nil {
		return "", false, err
	}
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(oldData)),
		B:        difflib.SplitLines(string(newData)),
		FromFile: oldPath,
		ToFile:   newPath,
		Context:  contextLines,
	}
	diffText, err := difflib.GetUnifiedDiffString(ud)
	if err != nil {
		return "", false, err
	}
	return diffText, diffText != "", nil
}

func readOptionalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func compareValues(path []Segment, oldValue, newValue interface{}) []Change {
	oldValue = normalize(oldValue)
	newValue = normalize(newValue)

	switch oldTyped := oldValue.(type) {
	case map[string]interface{}:
		newTyped, ok := newValue.(map[string]interface{})
		if !ok {
			return emitChange(path, oldValue, newValue)
		}
		return compareMaps(path, oldTyped, newTyped)
	case []interface{}:
		newTyped, ok := newValue.([]interface{})
		if !ok {
			return emitChange(path, oldValue, newValue)
		}
		return compareSlices(path, oldTyped, newTyped)
	default:
		if equalScalars(oldValue, newValue) {
			return nil
		}
		return emitChange(path, oldValue, newValue)
	}
}

func compareMaps(path []Segment, oldMap, newMap map[string]interface{}) []Change {
	keySet := map[string]struct{}{}
	for key := range oldMap {
		keySet[key] = struct{}{}
	}
	for key := range newMap {
		keySet[key] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var changes []Change
	for _, key := range keys {
		nextPath := appendCopy(path, Segment{Key: key})
		oldVal, oldOK := oldMap[key]
		newVal, newOK := newMap[key]
		switch {
		case !oldOK:
			changes = append(changes, Change{State: "added", Path: nextPath, New: newVal})
		case !newOK:
			changes = append(changes, Change{State: "removed", Path: nextPath, Old: oldVal})
		default:
			changes = append(changes, compareValues(nextPath, oldVal, newVal)...)
		}
	}
	return changes
}

func compareSlices(path []Segment, oldSlice, newSlice []interface{}) []Change {
	if matchKey, ok := detectMatchKey(oldSlice, newSlice); ok {
		return compareKeyedSlices(path, oldSlice, newSlice, matchKey)
	}

	var changes []Change
	max := len(oldSlice)
	if len(newSlice) > max {
		max = len(newSlice)
	}
	for i := 0; i < max; i++ {
		index := i
		nextPath := appendCopy(path, Segment{Index: &index})
		switch {
		case i >= len(oldSlice):
			changes = append(changes, Change{State: "added", Path: nextPath, New: normalize(newSlice[i])})
		case i >= len(newSlice):
			changes = append(changes, Change{State: "removed", Path: nextPath, Old: normalize(oldSlice[i])})
		default:
			changes = append(changes, compareValues(nextPath, oldSlice[i], newSlice[i])...)
		}
	}
	return changes
}

func compareKeyedSlices(path []Segment, oldSlice, newSlice []interface{}, matchKey string) []Change {
	oldMap := map[string]interface{}{}
	newMap := map[string]interface{}{}
	oldOrder := make([]string, 0, len(oldSlice))
	newOrder := make([]string, 0, len(newSlice))

	for _, item := range oldSlice {
		key := mustKey(item, matchKey)
		oldMap[key] = item
		oldOrder = append(oldOrder, key)
	}
	for _, item := range newSlice {
		key := mustKey(item, matchKey)
		newMap[key] = item
		newOrder = append(newOrder, key)
	}

	keySet := map[string]struct{}{}
	for _, key := range oldOrder {
		keySet[key] = struct{}{}
	}
	for _, key := range newOrder {
		keySet[key] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var changes []Change
	for _, key := range keys {
		nextPath := appendCopy(path, Segment{MatchKey: matchKey, MatchValue: key})
		oldVal, oldOK := oldMap[key]
		newVal, newOK := newMap[key]
		switch {
		case !oldOK:
			changes = append(changes, Change{State: "added", Path: nextPath, New: normalize(newVal)})
		case !newOK:
			changes = append(changes, Change{State: "removed", Path: nextPath, Old: normalize(oldVal)})
		default:
			changes = append(changes, compareValues(nextPath, oldVal, newVal)...)
		}
	}
	return changes
}

func detectMatchKey(oldSlice, newSlice []interface{}) (string, bool) {
	candidates := []string{"name", "key", "id", "metadata.name"}
	for _, candidate := range candidates {
		if sliceSupportsMatchKey(oldSlice, candidate) && sliceSupportsMatchKey(newSlice, candidate) {
			return candidate, true
		}
	}
	return "", false
}

func sliceSupportsMatchKey(items []interface{}, key string) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		if _, ok := extractScalarKey(item, key); !ok {
			return false
		}
	}
	return true
}

func extractScalarKey(value interface{}, key string) (string, bool) {
	current, ok := normalize(value).(map[string]interface{})
	if !ok {
		return "", false
	}
	parts := strings.Split(key, ".")
	var currentValue interface{} = current
	for _, part := range parts {
		nextMap, ok := currentValue.(map[string]interface{})
		if !ok {
			return "", false
		}
		currentValue, ok = nextMap[part]
		if !ok {
			return "", false
		}
	}
	switch typed := currentValue.(type) {
	case string:
		return typed, true
	case int, int64, float64, bool:
		return fmt.Sprint(typed), true
	default:
		return "", false
	}
}

func mustKey(value interface{}, key string) string {
	out, _ := extractScalarKey(value, key)
	return out
}

func emitChange(path []Segment, oldValue, newValue interface{}) []Change {
	state := "changed"
	switch {
	case oldValue == nil:
		state = "added"
	case newValue == nil:
		state = "removed"
	}
	return []Change{{State: state, Path: appendCopy(nil, path...), Old: normalize(oldValue), New: normalize(newValue)}}
}

func equalScalars(a, b interface{}) bool {
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

func appendCopy(path []Segment, segment ...Segment) []Segment {
	out := make([]Segment, len(path), len(path)+len(segment))
	copy(out, path)
	out = append(out, segment...)
	return out
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

func PathString(path []Segment) string {
	var b strings.Builder
	for i, segment := range path {
		switch {
		case segment.Key != "":
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(segment.Key)
		case segment.Index != nil:
			fmt.Fprintf(&b, "[%d]", *segment.Index)
		case segment.MatchKey != "":
			fmt.Fprintf(&b, "[%s=%s]", segment.MatchKey, segment.MatchValue)
		}
	}
	return b.String()
}

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
	if len(changes) == 0 {
		return "", nil
	}
	var parts []string
	for _, change := range changes {
		var body []string
		if change.Old != nil {
			oldSnippet, err := renderSnippetForValue(change.Path, change.Old)
			if err != nil {
				return "", err
			}
			body = append(body, "Old:")
			body = append(body, oldSnippet)
		}
		if change.New != nil {
			newSnippet, err := renderSnippetForValue(change.Path, change.New)
			if err != nil {
				return "", err
			}
			body = append(body, "New:")
			body = append(body, newSnippet)
		}
		part := fmt.Sprintf("Path: %s (%s)\n%s", PathString(change.Path), change.State, strings.Join(body, "\n"))
		parts = append(parts, part)
	}
	return strings.Join(parts, "\n\n"), nil
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
