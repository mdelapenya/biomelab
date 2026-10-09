package notes

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// initRepo creates a fresh repo with one commit and returns its absolute path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	// A global commit.gpgSign=true would make go-git refuse the commit.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg.Commit.GpgSign = config.NewOptBool(false)
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("set config: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	f, err := wt.Filesystem().Create("README.md")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _ = f.Write([]byte("# Test\n"))
	_ = f.Close()
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t"},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

// addWorktree creates a linked worktree at <repo>/<rel> on a new branch.
// Returns the absolute worktree path.
func addWorktree(t *testing.T, repo, branch, rel string) string {
	t.Helper()
	wtPath := filepath.Join(repo, rel)
	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return wtPath
}

func TestPath(t *testing.T) {
	got := Path("/some/worktree")
	want := filepath.Join("/some/worktree", ".biomelab", "note.md")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestTitlePath(t *testing.T) {
	got := TitlePath("/some/worktree")
	want := filepath.Join("/some/worktree", ".biomelab", "pr-title.md")
	if got != want {
		t.Errorf("TitlePath = %q, want %q", got, want)
	}
}

func TestDraftWriteDoesNotCreateUnexcludedFile(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		write      func(string) error
	}{
		{"note", noteFile, func(dir string) error { return Write(dir, "draft") }},
		{"title", prTitleFile, func(dir string) error { return WriteTitle(dir, "draft") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir() // No .git: exclusion must fail.
			if err := tc.write(root); err == nil {
				t.Fatal("write succeeded without an exclusion")
			}
			if _, err := os.Lstat(filepath.Join(root, noteDir, tc.file)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexcluded draft exists: %v", err)
			}
		})
	}
}

func TestDraftMutationsRejectSymlinkPaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		write  func(string) error
		delete func(string) error
		file   string
	}{
		{"note", func(dir string) error { return Write(dir, "changed") }, Delete, noteFile},
		{"title", func(dir string) error { return WriteTitle(dir, "changed") }, DeleteTitle, prTitleFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, parentLink := range []bool{false, true} {
				root := t.TempDir()
				outside := t.TempDir()
				outsideFile := filepath.Join(outside, tc.file)
				if err := os.WriteFile(outsideFile, []byte("keep"), filePerm); err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(root, noteDir)
				if parentLink {
					if err := os.Symlink(outside, dir); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				} else {
					if err := os.Mkdir(dir, dirPerm); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outsideFile, filepath.Join(dir, tc.file)); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				}
				if err := tc.write(root); err == nil {
					t.Fatalf("write accepted symlink (parent=%v)", parentLink)
				}
				if err := tc.delete(root); err == nil {
					t.Fatalf("delete accepted symlink (parent=%v)", parentLink)
				}
				got, err := os.ReadFile(outsideFile)
				if err != nil || string(got) != "keep" {
					t.Fatalf("outside file changed: %q, %v", got, err)
				}
			}
		})
	}
}

func TestReadTitle(t *testing.T) {
	cases := []struct {
		name    string
		writeFn func(dir string) // skip writing when nil → file missing
		want    string           // empty want means ok=false
	}{
		{"missing", nil, ""},
		{"single line", writeTitle("feat(x): do thing\n"), "feat(x): do thing"},
		{"trims whitespace", writeTitle("  feat(x): do thing  \n"), "feat(x): do thing"},
		{"first line only", writeTitle("feat(x): do thing\nignored body\nmore body\n"), "feat(x): do thing"},
		{"empty file", writeTitle(""), ""},
		{"only whitespace", writeTitle("  \n\n\t  \n"), ""},
		{"blank lines then content", writeTitle("\n\nfeat(x): do thing\n"), "feat(x): do thing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.writeFn != nil {
				tc.writeFn(dir)
			}
			title, ok, err := ReadTitle(dir)
			if err != nil {
				t.Fatalf("ReadTitle: %v", err)
			}
			wantOK := tc.want != ""
			if ok != wantOK {
				t.Errorf("ok = %v, want %v", ok, wantOK)
			}
			if title != tc.want {
				t.Errorf("title = %q, want %q", title, tc.want)
			}
		})
	}
}

func writeTitle(content string) func(string) {
	return func(dir string) {
		_ = os.MkdirAll(filepath.Join(dir, ".biomelab"), 0o755)
		_ = os.WriteFile(TitlePath(dir), []byte(content), 0o644)
	}
}

func TestWriteTitleRoundTrip(t *testing.T) {
	repo := initRepo(t)
	if err := WriteTitle(repo, "feat(x): do thing"); err != nil {
		t.Fatalf("WriteTitle: %v", err)
	}
	got, ok, err := ReadTitle(repo)
	if err != nil {
		t.Fatalf("ReadTitle: %v", err)
	}
	if !ok || got != "feat(x): do thing" {
		t.Errorf("ReadTitle = %q, ok=%v, want %q, ok=true", got, ok, "feat(x): do thing")
	}
}

func TestWriteTitleCollapsesNewlines(t *testing.T) {
	repo := initRepo(t)
	if err := WriteTitle(repo, "feat(x):\ndo\rthing\n"); err != nil {
		t.Fatalf("WriteTitle: %v", err)
	}
	data, err := os.ReadFile(TitlePath(repo))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "feat(x): do thing\n"
	if string(data) != want {
		t.Errorf("on-disk = %q, want %q", data, want)
	}
}

func TestWriteTitleEmptyDeletes(t *testing.T) {
	repo := initRepo(t)
	if err := WriteTitle(repo, "feat(x): do thing"); err != nil {
		t.Fatalf("WriteTitle: %v", err)
	}
	if _, err := os.Stat(TitlePath(repo)); err != nil {
		t.Fatalf("title should exist before whitespace overwrite: %v", err)
	}
	if err := WriteTitle(repo, "  \n\t  "); err != nil {
		t.Fatalf("WriteTitle whitespace: %v", err)
	}
	if _, err := os.Stat(TitlePath(repo)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("title should have been deleted, got err=%v", err)
	}
}

func TestDeleteTitle(t *testing.T) {
	repo := initRepo(t)
	if err := WriteTitle(repo, "feat(x): do thing"); err != nil {
		t.Fatalf("WriteTitle: %v", err)
	}
	if err := DeleteTitle(repo); err != nil {
		t.Fatalf("DeleteTitle: %v", err)
	}
	if _, err := os.Stat(TitlePath(repo)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("title should be gone, got err=%v", err)
	}
	// Idempotent on missing.
	if err := DeleteTitle(repo); err != nil {
		t.Fatalf("DeleteTitle missing: %v", err)
	}
}

func TestReadMissing(t *testing.T) {
	dir := t.TempDir()
	content, exists, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if exists {
		t.Errorf("exists = true, want false")
	}
	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
}

func TestExistsMissing(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Errorf("Exists = true on empty dir")
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	repo := initRepo(t)
	body := "# Hello\n\nSome **markdown** notes.\n"

	if err := Write(repo, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !Exists(repo) {
		t.Errorf("Exists = false after Write")
	}
	got, exists, err := Read(repo)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !exists {
		t.Errorf("exists = false after Write")
	}
	if got != body {
		t.Errorf("Read = %q, want %q", got, body)
	}
}

func TestWriteTrimsTrailingWhitespace(t *testing.T) {
	repo := initRepo(t)
	if err := Write(repo, "hello\n\n\n   \n\t\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _, err := Read(repo)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := "hello\n"
	if got != want {
		t.Errorf("Read = %q, want %q", got, want)
	}
}

func TestWriteAllWhitespaceDeletes(t *testing.T) {
	repo := initRepo(t)
	if err := Write(repo, "hi"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !Exists(repo) {
		t.Fatalf("note should exist before whitespace overwrite")
	}
	if err := Write(repo, "\n\n   \t\n"); err != nil {
		t.Fatalf("Write whitespace: %v", err)
	}
	if Exists(repo) {
		t.Errorf("note should have been deleted by all-whitespace Write")
	}
}

func TestDelete(t *testing.T) {
	repo := initRepo(t)
	if err := Write(repo, "x"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Delete(repo); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if Exists(repo) {
		t.Errorf("Exists = true after Delete")
	}
	// Delete on missing is a no-op.
	if err := Delete(repo); err != nil {
		t.Fatalf("Delete on missing: %v", err)
	}
}

func TestWriteExcludesNoteInMainWorktree(t *testing.T) {
	repo := initRepo(t)
	if err := Write(repo, "hi"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), excludeLine) {
		t.Errorf("exclude file missing %q:\n%s", excludeLine, data)
	}
}

func TestWriteExcludesNoteInLinkedWorktree(t *testing.T) {
	repo := initRepo(t)
	wtPath := addWorktree(t, repo, "feature/notes", ".biomelab-worktrees/notes")

	if err := Write(wtPath, "linked note"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Git only consults info/exclude from the common gitdir, not the
	// per-worktree gitdir, so the exclude entry must land in the MAIN
	// repo's .git/info/exclude — anything else and `git status` in the
	// linked worktree would still flag .biomelab/note.md as untracked.
	mainExclude := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(mainExclude)
	if err != nil {
		t.Fatalf("read main exclude: %v", err)
	}
	if !strings.Contains(string(data), excludeLine) {
		t.Errorf("main exclude file missing %q:\n%s", excludeLine, data)
	}
}

func TestWriteExcludesIdempotentAcrossWorktrees(t *testing.T) {
	repo := initRepo(t)
	wtPath := addWorktree(t, repo, "feature/notes", ".biomelab-worktrees/notes")

	// All worktrees share the common gitdir's exclude file. Writing notes
	// in both the main and linked worktrees must still produce only one
	// exclude entry.
	if err := Write(repo, "main note"); err != nil {
		t.Fatalf("Write main: %v", err)
	}
	if err := Write(wtPath, "linked note"); err != nil {
		t.Fatalf("Write linked: %v", err)
	}

	mainExclude := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(mainExclude)
	if err != nil {
		t.Fatalf("read main exclude: %v", err)
	}
	count := strings.Count(string(data), excludeLine)
	if count != 1 {
		t.Errorf("exclude line appears %d times across worktree writes, want 1:\n%s", count, data)
	}
}

func TestEnsureExcludedIdempotent(t *testing.T) {
	repo := initRepo(t)
	for i := range 3 {
		if err := Write(repo, "x"); err != nil {
			t.Fatalf("Write #%d: %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	count := strings.Count(string(data), excludeLine)
	if count != 1 {
		t.Errorf("exclude line appears %d times, want 1:\n%s", count, data)
	}
}

func TestEnsureExcludedAppendsNewlineWhenMissing(t *testing.T) {
	repo := initRepo(t)
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-existing content without a trailing newline.
	if err := os.WriteFile(excludePath, []byte("# user-added\nfoo.txt"), 0o644); err != nil {
		t.Fatalf("seed exclude: %v", err)
	}

	if err := Write(repo, "x"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	// The previous "foo.txt" line must still terminate properly and our
	// entry must appear on its own line.
	want := "# user-added\nfoo.txt\n" + excludeLine + "\n"
	if string(data) != want {
		t.Errorf("exclude = %q, want %q", data, want)
	}
}

func TestEnsureDir(t *testing.T) {
	repo := initRepo(t)
	if err := EnsureDir(repo); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	noteDir := filepath.Join(repo, ".biomelab")
	if _, err := os.Stat(noteDir); err != nil {
		t.Errorf("note dir not created: %v", err)
	}
	// Verify the exclude entry is set.
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), excludeLine) {
		t.Errorf("exclude file missing %q:\n%s", excludeLine, data)
	}
	// EnsureDir on existing dir should be idempotent.
	if err := EnsureDir(repo); err != nil {
		t.Fatalf("EnsureDir again: %v", err)
	}
}
