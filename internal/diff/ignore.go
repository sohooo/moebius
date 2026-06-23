package diff

import (
	"regexp"
	"strings"
)

type IgnoreOptions struct {
	UseDefaults bool
	Metadata    []MetadataIgnoreRule
}

type MetadataIgnoreRule struct {
	Locations   []string
	Labels      []string
	Annotations []string
}

func FilterIgnoredChanges(changes []Change, opts IgnoreOptions) []Change {
	if len(changes) == 0 {
		return nil
	}
	matcher := newIgnoreMatcher(opts)
	if len(matcher.rules) == 0 {
		return changes
	}
	out := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change.State != "changed" || !matcher.ignored(change.Path) {
			out = append(out, change)
		}
	}
	return out
}

type ignoreMatcher struct {
	rules []compiledMetadataIgnoreRule
}

type compiledMetadataIgnoreRule struct {
	locations   map[string]struct{}
	labels      []globPattern
	annotations []globPattern
}

type globPattern struct {
	raw string
	re  *regexp.Regexp
}

func newIgnoreMatcher(opts IgnoreOptions) ignoreMatcher {
	var rules []compiledMetadataIgnoreRule
	if opts.UseDefaults {
		rules = append(rules, compileMetadataIgnoreRule(MetadataIgnoreRule{
			Locations: []string{
				"metadata",
				"spec.template.metadata",
				"spec.jobTemplate.spec.template.metadata",
			},
			Labels: []string{
				"app.kubernetes.io/version",
				"helm.sh/chart",
			},
			Annotations: []string{
				"checksum/*",
			},
		}))
	}
	for _, rule := range opts.Metadata {
		rules = append(rules, compileMetadataIgnoreRule(rule))
	}
	return ignoreMatcher{rules: rules}
}

func compileMetadataIgnoreRule(rule MetadataIgnoreRule) compiledMetadataIgnoreRule {
	out := compiledMetadataIgnoreRule{
		locations: map[string]struct{}{},
	}
	for _, location := range rule.Locations {
		out.locations[strings.TrimSpace(location)] = struct{}{}
	}
	for _, pattern := range rule.Labels {
		out.labels = append(out.labels, compileGlob(pattern))
	}
	for _, pattern := range rule.Annotations {
		out.annotations = append(out.annotations, compileGlob(pattern))
	}
	return out
}

func (m ignoreMatcher) ignored(path []Segment) bool {
	location, field, key, ok := metadataPathParts(path)
	if !ok {
		return false
	}
	for _, rule := range m.rules {
		if _, ok := rule.locations[location]; !ok {
			continue
		}
		switch field {
		case "labels":
			if anyGlobMatches(rule.labels, key) {
				return true
			}
		case "annotations":
			if anyGlobMatches(rule.annotations, key) {
				return true
			}
		}
	}
	return false
}

func metadataPathParts(path []Segment) (location string, field string, key string, ok bool) {
	if len(path) < 3 {
		return "", "", "", false
	}
	for _, segment := range path {
		if segment.Key == "" {
			return "", "", "", false
		}
	}
	field = path[len(path)-2].Key
	if field != "labels" && field != "annotations" {
		return "", "", "", false
	}
	key = path[len(path)-1].Key
	location = PathString(path[:len(path)-2])
	return location, field, key, true
}

func compileGlob(pattern string) globPattern {
	var b strings.Builder
	b.WriteByte('^')
	for _, r := range pattern {
		if r == '*' {
			b.WriteString(".*")
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	b.WriteByte('$')
	return globPattern{raw: pattern, re: regexp.MustCompile(b.String())}
}

func anyGlobMatches(patterns []globPattern, value string) bool {
	for _, pattern := range patterns {
		if pattern.re.MatchString(value) {
			return true
		}
	}
	return false
}
