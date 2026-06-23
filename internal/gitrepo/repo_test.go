package gitrepo

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestAllClustersUsesAnyConfiguredAppsFile(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "clusters/kube-bravo/apps-dev.yaml", "- name: app\n  namespace: default\n  chart: charts/app\n")
	_, _ = repo, root

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	clusters, err := r.AllClustersForAppsFiles("clusters", []string{"apps.yaml", "apps-dev.yaml"})
	if err != nil {
		t.Fatalf("AllClustersForAppsFiles returned error: %v", err)
	}
	if len(clusters) != 1 || clusters[0] != "kube-bravo" {
		t.Fatalf("unexpected clusters: %v", clusters)
	}
}

func TestAllClustersAtCommitUsesAnyConfiguredAppsFile(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "clusters/kube-bravo/apps-dev.yaml", "- name: app\n  namespace: default\n  chart: charts/app\n")
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	clusters, err := r.AllClustersAtCommitForAppsFiles(commit, "clusters", []string{"apps.yaml", "apps-dev.yaml"})
	if err != nil {
		t.Fatalf("AllClustersAtCommitForAppsFiles returned error: %v", err)
	}
	if len(clusters) != 1 || clusters[0] != "kube-bravo" {
		t.Fatalf("unexpected clusters: %v", clusters)
	}
}

func TestChangedClustersAndMergeBase(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "clusters/kube-alpha/apps.yaml", "- name: app\n")
	baseHash := commitFiles(t, repo, root, "base", map[string]string{
		"clusters/kube-bravo/apps.yaml": "- name: app\n",
		"README.md":                     "base\n",
	})
	headHash := commitFiles(t, repo, root, "head", map[string]string{
		"clusters/kube-bravo/apps.yaml":   "- name: changed\n",
		"clusters/kube-charlie/apps.yaml": "- name: new\n",
		"docs/ignored.md":                 "ignored\n",
	})

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		t.Fatalf("base commit: %v", err)
	}
	headCommit, err := repo.CommitObject(headHash)
	if err != nil {
		t.Fatalf("head commit: %v", err)
	}
	mergeBase, err := r.MergeBase(headCommit, baseCommit)
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if mergeBase.Hash != baseHash {
		t.Fatalf("unexpected merge-base %s want %s", mergeBase.Hash, baseHash)
	}
	clusters, err := r.ChangedClusters("clusters", baseCommit, headCommit)
	if err != nil {
		t.Fatalf("ChangedClusters: %v", err)
	}
	if got, want := strings.Join(clusters, ","), "kube-bravo,kube-charlie"; got != want {
		t.Fatalf("unexpected changed clusters %q want %q", got, want)
	}
	paths, err := r.ChangedPaths(baseCommit, headCommit)
	if err != nil {
		t.Fatalf("ChangedPaths: %v", err)
	}
	if got, want := strings.Join(paths, ","), "clusters/kube-bravo/apps.yaml,clusters/kube-charlie/apps.yaml,docs/ignored.md"; got != want {
		t.Fatalf("unexpected changed paths %q want %q", got, want)
	}
}

func TestPathExistsAndWriteAtCommit(t *testing.T) {
	root := t.TempDir()
	repo := initRepoWithCommit(t, root, "clusters/kube-bravo/apps.yaml", "- name: app\n")
	commitHash := commitFiles(t, repo, root, "add chart", map[string]string{
		"charts/app/Chart.yaml":               "apiVersion: v2\nname: app\nversion: 0.1.0\n",
		"charts/app/templates/configmap.yaml": "kind: ConfigMap\n",
		"charts/app/templates/run.sh":         "#!/bin/sh\n",
	})
	execPath := filepath.Join(root, "charts/app/templates/run.sh")
	if err := os.Chmod(execPath, 0o755); err != nil {
		t.Fatalf("chmod executable: %v", err)
	}
	commitHash = commitWorktree(t, repo, "mark executable")

	r, err := Open(root)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	commit, err := repo.CommitObject(commitHash)
	if err != nil {
		t.Fatalf("commit object: %v", err)
	}
	for _, rel := range []string{"charts/app/Chart.yaml", "charts/app"} {
		exists, err := r.PathExistsAtCommit(commit, rel)
		if err != nil {
			t.Fatalf("PathExistsAtCommit(%s): %v", rel, err)
		}
		if !exists {
			t.Fatalf("expected %s to exist", rel)
		}
	}
	exists, err := r.PathExistsAtCommit(commit, "charts/missing")
	if err != nil {
		t.Fatalf("PathExistsAtCommit missing: %v", err)
	}
	if exists {
		t.Fatal("did not expect missing path to exist")
	}

	dest := t.TempDir()
	if err := r.WriteFileAtCommit(commit, "charts/app/Chart.yaml", dest); err != nil {
		t.Fatalf("WriteFileAtCommit: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "charts/app/Chart.yaml")); err != nil || !strings.Contains(string(data), "name: app") {
		t.Fatalf("unexpected written file data %q err %v", string(data), err)
	}
	if err := r.WriteDirAtCommit(commit, "charts/app", dest); err != nil {
		t.Fatalf("WriteDirAtCommit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "charts/app/templates/configmap.yaml")); err != nil {
		t.Fatalf("expected nested file copied: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dest, "charts/app/templates/run.sh")); err != nil || fileMode(info) == 0 {
		t.Fatalf("expected executable source file to be included in copy, mode=%v err=%v", fileMode(info), err)
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

func commitFiles(t *testing.T, repo *git.Repository, root, message string, files map[string]string) plumbing.Hash {
	t.Helper()
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return commitWorktree(t, repo, message)
}

func commitWorktree(t *testing.T, repo *git.Repository, message string) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatalf("AddGlob: %v", err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		All:    true,
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit %q: %v", message, err)
	}
	return hash
}

func fileMode(info fs.FileInfo) fs.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode()
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
