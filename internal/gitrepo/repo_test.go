package gitrepo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestResolveBaseRefUsesOriginHEAD(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "README.md", "hello")
	hash := branchCommit(t, repo, "main")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/remotes/origin/main"), hash)); err != nil {
		t.Fatalf("set origin/main ref: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), plumbing.ReferenceName("refs/remotes/origin/main"))); err != nil {
		t.Fatalf("set origin/HEAD ref: %v", err)
	}

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	name, _, err := r.ResolveBaseRef("")
	if err != nil {
		t.Fatalf("ResolveBaseRef returned error: %v", err)
	}
	if name != "main" {
		t.Fatalf("expected main, got %q", name)
	}
}

func TestAllClustersUsesConfiguredAppsFile(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "clusters/kube-bravo/releases.yaml", "- name: app\n  namespace: default\n  chart: charts/app\n")
	_, _ = repo, root

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	clusters, err := r.AllClusters("clusters", "releases.yaml")
	if err != nil {
		t.Fatalf("AllClusters returned error: %v", err)
	}
	if len(clusters) != 1 || clusters[0] != "kube-bravo" {
		t.Fatalf("unexpected clusters: %v", clusters)
	}
}

func initRepoWithCommit(t *testing.T, root, path, contents string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add(path); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return repo
}

func branchCommit(t *testing.T, repo *git.Repository, name string) plumbing.Hash {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash())); err != nil {
		t.Fatalf("set branch ref: %v", err)
	}
	_ = repo.CreateBranch(&config.Branch{Name: name})
	return head.Hash()
}
