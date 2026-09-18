package notes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueContextPaths(t *testing.T) {
	root := "/some/worktree"
	if got, want := IssuePath(root), filepath.Join(root, ".biomelab", "issue.md"); got != want {
		t.Fatalf("IssuePath() = %q, want %q", got, want)
	}
	if got, want := ProgressPath(root), filepath.Join(root, ".biomelab", "progress.md"); got != want {
		t.Fatalf("ProgressPath() = %q, want %q", got, want)
	}
}

func TestInitializeIssueContextCreatesIgnoredSnapshotAndProgress(t *testing.T) {
	repo := initRepo(t)
	issue := "# Issue 82: Context\n\nSource: https://example.test/issues/82\n\nRequirements\n"
	if err := InitializeIssueContext(repo, issue); err != nil {
		t.Fatalf("InitializeIssueContext: %v", err)
	}
	data, err := os.ReadFile(IssuePath(repo))
	if err != nil || string(data) != issue {
		t.Fatalf("issue snapshot = %q, %v", data, err)
	}
	progress, err := os.ReadFile(ProgressPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"## Completed work", "## Decisions", "## Remaining tasks or blockers", "## Validation", "## Current commit and uncommitted state", "No progress recorded yet. Inspect current code and Git state before planning; do not assume the issue is untouched."} {
		if !strings.Contains(string(progress), section) {
			t.Errorf("progress template missing %q", section)
		}
	}
	for _, rel := range []string{".biomelab/issue.md", ".biomelab/progress.md"} {
		cmd := exec.Command("git", "check-ignore", "-q", rel)
		cmd.Dir = repo
		if err := cmd.Run(); err != nil {
			t.Errorf("%s is not ignored: %v", rel, err)
		}
	}
}

func TestInitializeIssueContextPreservesExistingFilesOnRetry(t *testing.T) {
	repo := initRepo(t)
	if err := InitializeIssueContext(repo, "first issue"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProgressPath(repo), []byte("agent handoff\n"), filePerm); err != nil {
		t.Fatal(err)
	}
	if err := InitializeIssueContext(repo, "replacement issue"); err != nil {
		t.Fatal(err)
	}
	issue, _ := os.ReadFile(IssuePath(repo))
	progress, _ := os.ReadFile(ProgressPath(repo))
	if string(issue) != "first issue" || string(progress) != "agent handoff\n" {
		t.Fatalf("retry overwrote context: issue=%q progress=%q", issue, progress)
	}
}

func TestInitializeIssueContextRejectsUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, repo string)
	}{
		{"non-directory parent", func(t *testing.T, repo string) {
			if err := os.WriteFile(filepath.Join(repo, ".biomelab"), []byte("x"), filePerm); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink parent", func(t *testing.T, repo string) {
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.Mkdir(target, dirPerm); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(repo, ".biomelab")); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink issue target", func(t *testing.T, repo string) {
			if err := os.Mkdir(filepath.Join(repo, ".biomelab"), dirPerm); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, []byte("keep"), filePerm); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, IssuePath(repo)); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink progress target", func(t *testing.T, repo string) {
			if err := os.Mkdir(filepath.Join(repo, ".biomelab"), dirPerm); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, []byte("keep"), filePerm); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, ProgressPath(repo)); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			tc.setup(t, repo)
			if err := InitializeIssueContext(repo, "issue"); err == nil {
				t.Fatal("InitializeIssueContext succeeded")
			}
		})
	}
}
