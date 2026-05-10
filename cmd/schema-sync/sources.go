package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type sourceDocument struct {
	Name string
	Data []byte
}

type githubTag struct {
	Name string `json:"name"`
}

type githubContent struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

var plainSemverTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

func loadSourceDocuments(root string, source schemaSource, existingLock schemaLock, verify bool) ([]sourceDocument, lockedSource, error) {
	switch source.SourceType {
	case "file":
		docs, err := loadFileSourceDocuments(root, source.Paths)
		return docs, lockedSource{
			Component:       source.Component,
			SourceType:      source.SourceType,
			Version:         source.Version,
			ResolvedVersion: source.Version,
		}, err
	case "url":
		return loadURLSourceDocumentsForSource(source, existingLock, verify)
	case "github_release":
		return loadGitHubReleaseSourceDocuments(root, source, existingLock, verify)
	default:
		return nil, lockedSource{}, fmt.Errorf("unsupported source type %q", source.SourceType)
	}
}

func loadURLSourceDocumentsForSource(source schemaSource, existingLock schemaLock, verify bool) ([]sourceDocument, lockedSource, error) {
	resolvedVersion := source.Version
	if verify && source.Version == "latest" {
		locked, ok := findLockedSource(existingLock, source)
		if !ok {
			return nil, lockedSource{}, fmt.Errorf("missing lock entry for %s; run cmd/schema-sync", source.Component)
		}
		resolvedVersion = locked.ResolvedVersion
	} else if source.Version == "latest" {
		var err error
		resolvedVersion, err = resolveLatestGitHubRelease(source.Repo)
		if err != nil {
			return nil, lockedSource{}, err
		}
	}

	docs, err := loadURLSourceDocuments(expandPatterns(source.URLs, resolvedVersion))
	return docs, lockedSource{
		Component:       source.Component,
		SourceType:      source.SourceType,
		Version:         source.Version,
		ResolvedVersion: resolvedVersion,
		Repo:            source.Repo,
	}, err
}

func loadFileSourceDocuments(root string, patterns []string) ([]sourceDocument, error) {
	var documents []sourceDocument
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(resolvePath(root, pattern))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("schema source path %q matched no files", pattern)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			data, err := os.ReadFile(match)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(root, match)
			if err != nil {
				rel = match
			}
			documents = append(documents, sourceDocument{Name: filepath.ToSlash(rel), Data: data})
		}
	}
	return documents, nil
}

func loadGitHubReleaseSourceDocuments(root string, source schemaSource, existingLock schemaLock, verify bool) ([]sourceDocument, lockedSource, error) {
	resolvedVersion := source.Version
	if verify {
		locked, ok := findLockedSource(existingLock, source)
		if !ok {
			return nil, lockedSource{}, fmt.Errorf("missing lock entry for %s; run cmd/schema-sync", source.Component)
		}
		resolvedVersion = locked.ResolvedVersion
	} else if source.Version == "latest" {
		var err error
		resolvedVersion, err = resolveLatestGitHubRelease(source.Repo)
		if err != nil {
			return nil, lockedSource{}, err
		}
	}

	locked := lockedSource{
		Component:       source.Component,
		SourceType:      source.SourceType,
		Version:         source.Version,
		ResolvedVersion: resolvedVersion,
		Repo:            source.Repo,
		AssetName:       source.AssetName,
	}

	if source.AssetName != "" {
		downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", source.Repo, resolvedVersion, source.AssetName)
		docs, err := loadURLSourceDocuments([]string{downloadURL})
		return docs, locked, err
	}

	archiveURL := fmt.Sprintf("https://github.com/%s/archive/refs/tags/%s.tar.gz", source.Repo, resolvedVersion)
	documents, err := loadGitHubArchiveDocuments(root, archiveURL, expandPatterns(source.Paths, resolvedVersion))
	return documents, locked, err
}

func loadURLSourceDocuments(rawURLs []string) ([]sourceDocument, error) {
	client := &http.Client{}
	documents := make([]sourceDocument, 0, len(rawURLs))
	for _, rawURL := range rawURLs {
		resp, err := client.Get(rawURL)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("fetch %s: unexpected status %s", rawURL, resp.Status)
		}
		documents = append(documents, sourceDocument{Name: rawURL, Data: data})
	}
	return documents, nil
}

func loadGitHubArchiveDocuments(root string, rawURL string, patterns []string) ([]sourceDocument, error) {
	client := &http.Client{}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", rawURL, resp.Status)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var documents []sourceDocument
	seen := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(header.Name)
		if !matchesAnyArchivePattern(name, patterns) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}
		seen[name] = struct{}{}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			rel = name
		}
		documents = append(documents, sourceDocument{Name: filepath.ToSlash(rel), Data: data})
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("github source archive %s matched no files", rawURL)
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].Name < documents[j].Name })
	return documents, nil
}

func resolveLatestGitHubRelease(repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mobius-schema-sync")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resolveLatestGitHubTag(repo)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve latest release for %s: unexpected status %s", repo, resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("resolve latest release for %s: missing tag_name", repo)
	}
	return payload.TagName, nil
}

func resolveLatestGitHubTag(repo string) (string, error) {
	client := &http.Client{}

	var allTags []githubTag
	for page := 1; ; page++ {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/tags?per_page=100&page=%d", repo, page)
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "mobius-schema-sync")
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return "", fmt.Errorf("resolve latest tag for %s: unexpected status %s", repo, resp.Status)
		}
		var payload []githubTag
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		allTags = append(allTags, payload...)
		if len(payload) < 100 {
			break
		}
	}
	best, ok := latestPlainSemverTag(allTags)
	if !ok {
		return resolveLatestGitHubDirectory(repo)
	}
	return best, nil
}

func latestPlainSemverTag(payload []githubTag) (string, bool) {
	var best string
	var bestVersion *semver.Version
	for _, item := range payload {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		if !plainSemverTag.MatchString(item.Name) {
			continue
		}
		candidateVersion, err := semver.NewVersion(strings.TrimPrefix(item.Name, "v"))
		if err != nil {
			continue
		}
		if bestVersion == nil || candidateVersion.GreaterThan(bestVersion) {
			best = item.Name
			bestVersion = candidateVersion
		}
	}
	return best, bestVersion != nil
}

func resolveLatestGitHubDirectory(repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents", repo)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mobius-schema-sync")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve latest directory for %s: unexpected status %s", repo, resp.Status)
	}
	var payload []githubContent
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	tags := make([]githubTag, 0, len(payload))
	for _, item := range payload {
		if item.Type != "dir" {
			continue
		}
		tags = append(tags, githubTag{Name: item.Name})
	}
	best, ok := latestPlainSemverTag(tags)
	if !ok {
		return "", fmt.Errorf("resolve latest directory for %s: no version directories found", repo)
	}
	return best, nil
}

func matchesAnyArchivePattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		matchPattern := filepath.ToSlash(pattern)
		matchPattern = strings.TrimPrefix(matchPattern, "/")
		ok, err := filepath.Match(matchPattern, name)
		if err == nil && ok {
			return true
		}
		if strings.HasSuffix(matchPattern, "/*") {
			prefix := strings.TrimSuffix(matchPattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

func expandPatterns(patterns []string, resolvedVersion string) []string {
	trimmedVersion := strings.TrimPrefix(resolvedVersion, "v")
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.ReplaceAll(pattern, "{version}", resolvedVersion)
		pattern = strings.ReplaceAll(pattern, "{version_nov}", trimmedVersion)
		out = append(out, pattern)
	}
	return out
}
