package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

func TestRemoveWorktreeRefusesChangedDataWithoutMutations(t *testing.T) {
	for _, scenario := range []string{"tracked", "staged", "untracked", "tracked ignored-path"} {
		t.Run(scenario, func(t *testing.T) {
			dir, _ := setupTestRepo(t)
			repo, err := OpenRepository(dir)
			if err != nil {
				t.Fatal(err)
			}
			const branch = "preserve-work"
			if err := repo.CreateWorktree(branch); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, ".biomelab-worktrees", branch)
			file := filepath.Join(path, "README.md")
			if scenario == "untracked" {
				file = filepath.Join(path, "untracked-plan.md")
			}
			if scenario == "tracked ignored-path" {
				file = filepath.Join(path, ".biomelab", "tracked.md")
				if err := os.WriteFile(file, []byte("original tracked\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, path, "add", "--force", ".biomelab/tracked.md")
				runGit(t, path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "tracked ignored-path fixture")
			}
			if err := os.WriteFile(file, []byte("user data to preserve\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if scenario == "staged" {
				runGit(t, path, "add", "README.md")
			}
			before := runGit(t, path, "rev-parse", "HEAD")
			generation := repo.Generation()
			metadata := filepath.Join(dir, ".git", "worktrees", branch)
			indexBefore, err := os.ReadFile(filepath.Join(metadata, "index"))
			if err != nil {
				t.Fatal(err)
			}
			err = repo.RemoveWorktree(branch)
			if err == nil || !strings.Contains(err.Error(), "modified or untracked files") || !strings.Contains(err.Error(), path) {
				t.Fatalf("missing actionable refusal: %v", err)
			}
			data, err := os.ReadFile(file)
			if err != nil || string(data) != "user data to preserve\n" {
				t.Fatalf("user data changed: %q %v", data, err)
			}
			indexAfter, err := os.ReadFile(filepath.Join(metadata, "index"))
			if err != nil || string(indexAfter) != string(indexBefore) {
				t.Fatal("refused status check changed index/metadata")
			}
			if got := runGit(t, path, "rev-parse", "HEAD"); got != before || repo.Generation() != generation {
				t.Fatal("refused deletion changed HEAD or generation")
			}
			ref, err := repo.repo.Reference(plumbing.NewBranchReferenceName(branch), false)
			if err != nil || ref.Hash().String() != before {
				t.Fatal("refused deletion removed branch")
			}
		})
	}
}

func TestRemoveWorktreeStatusFailurePreservesData(t *testing.T) {
	for _, scenario := range []string{"invalid index", "missing git", "missing path metadata", "locked clean worktree"} {
		t.Run(scenario, func(t *testing.T) {
			dir, _ := setupTestRepo(t)
			repo, err := OpenRepository(dir)
			if err != nil {
				t.Fatal(err)
			}
			const branch = "preserve-on-failure"
			if err := repo.CreateWorktree(branch); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, ".biomelab-worktrees", branch)
			metadata := filepath.Join(dir, ".git", "worktrees", branch)
			switch scenario {
			case "invalid index":
				if err := os.WriteFile(filepath.Join(metadata, "index"), []byte("invalid index"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "missing git":
				t.Setenv("PATH", t.TempDir())
			case "missing path metadata":
				if err := os.Remove(filepath.Join(metadata, "gitdir")); err != nil {
					t.Fatal(err)
				}
			default:
				runGit(t, dir, "worktree", "lock", path)
			}
			generation := repo.Generation()
			err = repo.RemoveWorktree(branch)
			if err == nil {
				t.Fatal("unsafe verification/removal failure was ignored")
			}
			if data, err := os.ReadFile(filepath.Join(path, "README.md")); err != nil || string(data) != "# Test\n" {
				t.Fatalf("refusal lost checkout: %q error=%v", data, err)
			}
			if _, err := os.Stat(metadata); err != nil || repo.Generation() != generation {
				t.Fatal("refusal lost metadata or changed generation")
			}
			if _, err := repo.repo.Reference(plumbing.NewBranchReferenceName(branch), false); err != nil {
				t.Fatal("refusal lost branch reference")
			}
		})
	}
}

func TestRemoveWorktreeAllowsExplicitCleanRemovalWithIgnoredNotes(t *testing.T) {
	dir, _ := setupTestRepo(t)
	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	const branch = "clean-removal"
	if err := repo.CreateWorktree(branch); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".biomelab-worktrees", branch)
	if err := os.WriteFile(filepath.Join(path, ".biomelab", "note.md"), []byte("ignored note explicitly covered by confirmation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveWorktree(branch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("clean checkout was not removed")
	}
}
