package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

// setupTestRepo creates a temporary git repository with an initial commit.
func setupTestRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create an initial commit so HEAD exists.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	f, err := wt.Filesystem.Create("README.md")
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, _ = f.Write([]byte("# Test\n"))
	_ = f.Close()

	_, err = wt.Add("README.md")
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	return dir, repo
}

func TestOpenRepository(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	if repo.Root() != dir {
		t.Errorf("got root %q, want %q", repo.Root(), dir)
	}
}

func TestListWorktrees_MainOnly(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}

	if !wts[0].IsMain {
		t.Error("expected main worktree")
	}

	if wts[0].Branch != "master" && wts[0].Branch != "main" {
		t.Errorf("unexpected branch %q", wts[0].Branch)
	}

	if wts[0].IsDirty {
		t.Error("expected clean worktree")
	}
}

func TestListWorktrees_WithLinked(t *testing.T) {
	dir, repo := setupTestRepo(t)

	// Create a linked worktree using go-git's x/plumbing/worktree.
	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		t.Fatalf("failed to create worktree manager: %v", err)
	}

	linkedPath := filepath.Join(filepath.Dir(dir), "test-branch")
	t.Cleanup(func() { _ = os.RemoveAll(linkedPath) })

	linkedFS := osfs.New(linkedPath)
	err = wt.Add(linkedFS, "test-branch")
	if err != nil {
		t.Fatalf("failed to add worktree: %v", err)
	}

	// Open with our wrapper and list.
	gwaim, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := gwaim.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	found := false
	for _, w := range wts {
		if w.Branch == "test-branch" {
			found = true
			if w.IsMain {
				t.Error("linked worktree should not be main")
			}
		}
	}
	if !found {
		t.Error("linked worktree not found in list")
	}
}

func TestCreateAndRemoveWorktree(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	branchName := "feature-test"
	linkedPath := filepath.Join(filepath.Dir(dir), branchName)
	t.Cleanup(func() { _ = os.RemoveAll(linkedPath) })

	err = repo.CreateWorktree(branchName)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	// Verify it exists.
	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees after create, got %d", len(wts))
	}

	// Remove it.
	err = repo.RemoveWorktree(branchName)
	if err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	wts, err = repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(wts))
	}
}

func TestDirtyDetection(t *testing.T) {
	dir, _ := setupTestRepo(t)

	// Modify a file to make it dirty.
	err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644)
	if err != nil {
		t.Fatalf("failed to create dirty file: %v", err)
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}

	if !wts[0].IsDirty {
		t.Error("expected dirty worktree after creating untracked file")
	}
}

func TestRepoRoot(t *testing.T) {
	dir, _ := setupTestRepo(t)

	// Create a subdirectory and test detection from there.
	subDir := filepath.Join(dir, "sub", "dir")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	root, err := RepoRoot(subDir)
	if err != nil {
		t.Fatalf("RepoRoot failed: %v", err)
	}

	if root != dir {
		t.Errorf("got root %q, want %q", root, dir)
	}
}

func TestWorktreePaths(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	paths, err := repo.WorktreePaths()
	if err != nil {
		t.Fatalf("WorktreePaths failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	if paths[0] != dir {
		t.Errorf("got path %q, want %q", paths[0], dir)
	}
}
