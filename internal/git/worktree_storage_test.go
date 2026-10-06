package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

func TestGeneratedWorktreeStorageUsesLocalExclude(t *testing.T) {
	dir, _ := setupTestRepo(t)
	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludePath, []byte("# user exclusions\n/user-cache/"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBootstrapStatusFile(t, dir, ".gitignore", "# tracked ignore remains untouched\n")
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "ignore fixture")
	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"scratch-one", "scratch-two"} {
		if err := repo.CreateWorktree(branch); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(excludePath)
	if err != nil || !strings.HasPrefix(string(data), "# user exclusions\n/user-cache/\n") || strings.Count(string(data), "/.biomelab-worktrees/\n") != 1 {
		t.Fatalf("exclude lost user content or duplicated registration: %q error=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || string(data) != "# tracked ignore remains untouched\n" {
		t.Fatal("creation changed tracked .gitignore")
	}
	assertMainDirtyMatchesNative(t, repo, false)
}

func TestGeneratedWorktreeExcludeFailurePreventsCreation(t *testing.T) {
	dir, raw := setupTestRepo(t)
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.Remove(exclude); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.MkdirAll(exclude, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	generation := repo.Generation()
	err = repo.CreateWorktree("scratch")
	if err == nil || !strings.Contains(err.Error(), "exclude generated worktrees directory") {
		t.Fatalf("missing actionable exclusion error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".biomelab-worktrees")); !os.IsNotExist(err) {
		t.Fatalf("creation proceeded without exclusion: %v", err)
	}
	if _, err := raw.Reference(plumbing.NewBranchReferenceName("scratch"), false); err == nil || repo.Generation() != generation {
		t.Fatal("failed exclusion created a branch or advanced generation")
	}
}

func TestGeneratedWorktreeStatusNeverHidesUserChanges(t *testing.T) {
	for _, scenario := range []string{"excluded", "not excluded", "later exclude negation", "tracked generated-path file", "repository negation", "unrelated user directory"} {
		t.Run(scenario, func(t *testing.T) {
			dir, _ := setupTestRepo(t)
			path := ".biomelab-worktrees/generated.txt"
			if scenario == "unrelated user directory" {
				path = "user-worktrees/draft.txt"
			}
			if scenario == "tracked generated-path file" {
				writeBootstrapStatusFile(t, dir, path, "tracked original\n")
				runGit(t, dir, "add", path)
				runGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "tracked fixture")
			}
			if scenario == "repository negation" {
				writeBootstrapStatusFile(t, dir, ".gitignore", "!/.biomelab-worktrees/\n!/.biomelab-worktrees/generated.txt\n")
				runGit(t, dir, "add", ".gitignore")
				runGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "negation fixture")
			}
			writeBootstrapStatusFile(t, dir, path, "new user content\n")
			if scenario != "not excluded" {
				if err := EnsureExcluded(dir, "/.biomelab-worktrees/"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "later exclude negation" {
				if err := EnsureExcluded(dir, "!/.biomelab-worktrees/"); err != nil {
					t.Fatal(err)
				}
			}
			repo, err := OpenRepository(dir)
			if err != nil {
				t.Fatal(err)
			}
			assertMainDirtyMatchesNative(t, repo, scenario != "excluded")
		})
	}
}

func assertMainDirtyMatchesNative(t *testing.T, repo *Repository, want bool) {
	t.Helper()
	nativeDirty := runGit(t, repo.Root(), "status", "--porcelain", "--untracked-files=all") != ""
	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	for _, wt := range wts {
		if wt.IsMain {
			if wt.IsDirty != want || nativeDirty != want {
				t.Fatalf("main dirty: app=%v native=%v want=%v", wt.IsDirty, nativeDirty, want)
			}
			return
		}
	}
	t.Fatal("main checkout missing")
}
