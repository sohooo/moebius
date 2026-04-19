package gitrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repo struct {
	root string
	repo *git.Repository
}

func Open(path string) (*Repo, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	root, err := repoRoot(path)
	if err != nil {
		return nil, err
	}
	return &Repo{root: root, repo: repo}, nil
}

func repoRoot(path string) (string, error) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("no git repository found from %s", path)
		}
		dir = next
	}
}

func (r *Repo) Root() string { return r.root }

func (r *Repo) ResolveCommit(rev string) (*object.Commit, error) {
	if rev == "HEAD" {
		head, err := r.repo.Head()
		if err != nil {
			return nil, err
		}
		return r.repo.CommitObject(head.Hash())
	}

	candidates := []string{
		rev,
		"refs/heads/" + rev,
		"refs/remotes/origin/" + rev,
	}
	for _, candidate := range candidates {
		hash, err := r.repo.ResolveRevision(plumbing.Revision(candidate))
		if err == nil && hash != nil {
			return r.repo.CommitObject(*hash)
		}
	}
	return nil, fmt.Errorf("could not resolve git revision %q", rev)
}

func (r *Repo) ResolveBaseRef(rev string) (string, *object.Commit, error) {
	if rev != "" {
		commit, err := r.ResolveCommit(rev)
		if err != nil {
			return "", nil, err
		}
		return rev, commit, nil
	}

	candidates := make([]string, 0, 3)
	if ref, err := r.repo.Reference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), true); err == nil {
		target := ref.Target().String()
		if strings.HasPrefix(target, "refs/remotes/origin/") {
			candidates = append(candidates, strings.TrimPrefix(target, "refs/remotes/origin/"))
		}
	}
	candidates = append(candidates, "main", "master")

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		commit, err := r.ResolveCommit(candidate)
		if err == nil {
			return candidate, commit, nil
		}
	}
	return "", nil, fmt.Errorf("could not auto-detect a base ref; tried origin/HEAD, main, and master")
}

func (r *Repo) MergeBase(head, base *object.Commit) (*object.Commit, error) {
	commits, err := head.MergeBase(base)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("no merge-base found between %s and %s", head.Hash, base.Hash)
	}
	return commits[0], nil
}

func (r *Repo) ChangedClusters(clustersDir string, base, head *object.Commit) ([]string, error) {
	baseTree, err := base.Tree()
	if err != nil {
		return nil, err
	}
	headTree, err := head.Tree()
	if err != nil {
		return nil, err
	}
	patch, err := baseTree.Patch(headTree)
	if err != nil {
		return nil, err
	}

	prefix := strings.TrimSuffix(filepath.ToSlash(clustersDir), "/") + "/"
	set := map[string]struct{}{}
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		for _, file := range []interface{ Path() string }{from, to} {
			if file == nil {
				continue
			}
			path := file.Path()
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			rest := strings.TrimPrefix(path, prefix)
			parts := strings.SplitN(rest, "/", 2)
			if parts[0] != "" {
				set[parts[0]] = struct{}{}
			}
		}
	}
	return mapKeys(set), nil
}

func (r *Repo) AllClusters(clustersDir, appsFile string) ([]string, error) {
	root := filepath.Join(r.root, clustersDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var clusters []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), filepath.FromSlash(appsFile))); err == nil {
			clusters = append(clusters, entry.Name())
		}
	}
	sort.Strings(clusters)
	return clusters, nil
}

func (r *Repo) AllClustersAtCommit(commit *object.Commit, clustersDir, appsFile string) ([]string, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(clustersDir), "/") + "/"
	appsFile = filepath.ToSlash(appsFile)
	set := map[string]struct{}{}
	iter := tree.Files()
	defer iter.Close()
	if err := iter.ForEach(func(file *object.File) error {
		if !strings.HasPrefix(file.Name, prefix) {
			return nil
		}
		rest := strings.TrimPrefix(file.Name, prefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil
		}
		if parts[1] == appsFile {
			set[parts[0]] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return mapKeys(set), nil
}

func (r *Repo) PathExistsAtCommit(commit *object.Commit, relPath string) (bool, error) {
	tree, err := commit.Tree()
	if err != nil {
		return false, err
	}
	_, err = tree.File(filepath.ToSlash(relPath))
	if err == nil {
		return true, nil
	}
	if err == object.ErrFileNotFound {
		_, dirErr := tree.Tree(filepath.ToSlash(relPath))
		if dirErr == nil {
			return true, nil
		}
		if dirErr == object.ErrDirectoryNotFound {
			return false, nil
		}
		return false, dirErr
	}
	return false, err
}

func (r *Repo) WriteFileAtCommit(commit *object.Commit, relPath, destRoot string) error {
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	file, err := tree.File(filepath.ToSlash(relPath))
	if err != nil {
		return err
	}
	reader, err := file.Reader()
	if err != nil {
		return err
	}
	defer reader.Close()

	destPath := filepath.Join(destRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(reader)
	return err
}

func (r *Repo) WriteDirAtCommit(commit *object.Commit, prefix, destRoot string) error {
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	prefix = strings.TrimSuffix(filepath.ToSlash(prefix), "/") + "/"
	iter := tree.Files()
	defer iter.Close()
	return iter.ForEach(func(file *object.File) error {
		if !strings.HasPrefix(file.Name, prefix) {
			return nil
		}
		if file.Mode != filemode.Regular && file.Mode != filemode.Executable {
			return nil
		}
		return r.WriteFileAtCommit(commit, file.Name, destRoot)
	})
}

func mapKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
