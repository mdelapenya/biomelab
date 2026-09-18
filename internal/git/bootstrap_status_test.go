package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListWorktreesAgentBootstrapStatus(t *testing.T) {
	t.Run("generated excluded instructions are clean", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktree(t, false)
		files := []string{
			"AGENTS.md",
			"CLAUDE.md",
			"GEMINI.md",
			filepath.Join(".kiro", "steering", "biomelab-task.md"),
		}
		for _, path := range files {
			writeBootstrapStatusFile(t, wtPath, path, "generated\n")
			if err := EnsureExcluded(wtPath, "/"+filepath.ToSlash(path)); err != nil {
				t.Fatalf("EnsureExcluded(%q): %v", path, err)
			}
		}
		assertLinkedDirty(t, repo, false)
	})

	t.Run("tracked instruction modification is dirty", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktree(t, true)
		if err := EnsureExcluded(wtPath, "/AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		writeBootstrapStatusFile(t, wtPath, "AGENTS.md", "modified\n")
		assertLinkedDirty(t, repo, true)
	})

	t.Run("untracked instruction without matching exclusion is dirty", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktree(t, false)
		writeBootstrapStatusFile(t, wtPath, "GEMINI.md", "generated\n")
		if err := EnsureExcluded(wtPath, "/AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		assertLinkedDirty(t, repo, true)
	})

	t.Run("later common exclusion negation is dirty", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktree(t, false)
		writeBootstrapStatusFile(t, wtPath, "AGENTS.md", "user owned\n")
		if err := EnsureExcluded(wtPath, "/AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		if err := EnsureExcluded(wtPath, "!/AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		assertNativeGitReportsUntracked(t, wtPath, "AGENTS.md")
		assertLinkedDirty(t, repo, true)
	})

	t.Run("repository gitignore negation is dirty", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktreeWithTracked(t, map[string]string{
			".gitignore": "!/AGENTS.md\n",
		})
		writeBootstrapStatusFile(t, wtPath, "AGENTS.md", "user owned\n")
		if err := EnsureExcluded(wtPath, "/AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		assertNativeGitReportsUntracked(t, wtPath, "AGENTS.md")
		assertLinkedDirty(t, repo, true)
	})

	t.Run("unrelated Kiro file is dirty", func(t *testing.T) {
		repo, wtPath := setupBootstrapStatusWorktree(t, false)
		path := filepath.Join(".kiro", "steering", "biomelab-task.md")
		writeBootstrapStatusFile(t, wtPath, path, "generated\n")
		if err := EnsureExcluded(wtPath, "/"+filepath.ToSlash(path)); err != nil {
			t.Fatal(err)
		}
		writeBootstrapStatusFile(t, wtPath, filepath.Join(".kiro", "steering", "other.md"), "user owned\n")
		assertLinkedDirty(t, repo, true)
	})
}

func setupBootstrapStatusWorktree(t *testing.T, trackAgents bool) (*Repository, string) {
	t.Helper()
	tracked := make(map[string]string)
	if trackAgents {
		tracked["AGENTS.md"] = "original\n"
	}
	return setupBootstrapStatusWorktreeWithTracked(t, tracked)
}

func setupBootstrapStatusWorktreeWithTracked(t *testing.T, tracked map[string]string) (*Repository, string) {
	t.Helper()
	dir, _ := setupTestRepo(t)
	for path, content := range tracked {
		writeBootstrapStatusFile(t, dir, path, content)
		runGit(t, dir, "add", path)
	}
	if len(tracked) > 0 {
		runGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "instructions")
	}
	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorktree("bootstrap-status"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	return repo, filepath.Join(dir, ".biomelab-worktrees", "bootstrap-status")
}

func writeBootstrapStatusFile(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertLinkedDirty(t *testing.T, repo *Repository, want bool) {
	t.Helper()
	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	for _, wt := range wts {
		if !wt.IsMain && wt.Branch == "bootstrap-status" {
			if wt.IsDirty != want {
				t.Fatalf("linked worktree dirty = %v, want %v", wt.IsDirty, want)
			}
			return
		}
	}
	t.Fatal("bootstrap-status linked worktree not found")
}

func assertNativeGitReportsUntracked(t *testing.T, worktreePath, path string) {
	t.Helper()
	status := runGit(t, worktreePath, "status", "--porcelain", "--untracked-files=all")
	if !strings.Contains(status, "?? "+path) {
		t.Fatalf("native git status does not report %q as untracked: %q", path, status)
	}
}
