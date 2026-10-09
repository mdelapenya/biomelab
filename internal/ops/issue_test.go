package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/object"

	gitops "github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/notes"
)

func TestSuggestedIssueBranch(t *testing.T) {
	tests := []struct {
		name, title, want string
	}{
		{"punctuation", "Fix: API / parser... now!", "issue-82-fix-api-parser-now"},
		{"unicode only", "你好 🧪", "issue-82"},
		{"mixed unicode", "Crème brûlée", "issue-82-cr-me-br-l-e"},
		{"empty", "", "issue-82"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestedIssueBranch(github.IssueInfo{Number: 82, Title: tt.title})
			if got != tt.want {
				t.Fatalf("SuggestedIssueBranch() = %q, want %q", got, tt.want)
			}
		})
	}
	got := SuggestedIssueBranch(github.IssueInfo{Number: 1, Title: strings.Repeat("a", 80)})
	if got != "issue-1-"+strings.Repeat("a", 60) {
		t.Fatalf("long slug = %q", got)
	}
}

func TestValidateIssueBranch(t *testing.T) {
	for _, valid := range []string{"issue-82-fix", "A", "123"} {
		if err := ValidateIssueBranch(valid); err != nil {
			t.Errorf("ValidateIssueBranch(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "-start", "with/slash", "../escape", "has space", "café", "HEAD", strings.Repeat("a", 101)} {
		if err := ValidateIssueBranch(invalid); err == nil {
			t.Errorf("ValidateIssueBranch(%q) succeeded", invalid)
		}
	}
}

func TestCreateWorktreeFromIssue(t *testing.T) {
	dir, repo := newOpsRepo(t, nil)
	mainHead := gitOutput(t, dir, "rev-parse", "HEAD")
	issue := github.IssueInfo{Number: 82, Title: "Fetch an issue", Body: "Body text", URL: "https://github.com/o/r/issues/82"}
	result := CreateWorktreeFromIssue(repo, issue, "issue-82-fetch-an-issue")
	if result.Err != nil || result.NotesErr != nil {
		t.Fatalf("CreateWorktreeFromIssue: Err=%v NotesErr=%v", result.Err, result.NotesErr)
	}
	wantPath := filepath.Join(dir, ".biomelab-worktrees", result.BranchName)
	if result.WtPath != wantPath {
		t.Fatalf("WtPath = %q, want %q", result.WtPath, wantPath)
	}
	if got := gitOutput(t, result.WtPath, "rev-parse", "HEAD"); got != mainHead {
		t.Errorf("worktree base = %s, want %s", got, mainHead)
	}
	if got := gitOutput(t, dir, "rev-parse", "HEAD"); got != mainHead {
		t.Errorf("main HEAD changed to %s", got)
	}
	title, ok, err := notes.ReadTitle(result.WtPath)
	if err != nil || !ok || title != issue.Title {
		t.Errorf("title = %q, %v, %v", title, ok, err)
	}
	body, ok, err := notes.Read(result.WtPath)
	wantBody := "# Issue 82: Fetch an issue\n\nSource: https://github.com/o/r/issues/82\n\nBody text\n"
	if err != nil || !ok || body != wantBody {
		t.Errorf("note = %q, %v, %v; want %q", body, ok, err, wantBody)
	}
	for _, rel := range []string{".biomelab/note.md", ".biomelab/pr-title.md", ".biomelab/issue.md", ".biomelab/progress.md"} {
		cmd := exec.Command("git", "check-ignore", "-q", rel)
		cmd.Dir = result.WtPath
		if err := cmd.Run(); err != nil {
			t.Errorf("%s is not ignored: %v", rel, err)
		}
	}
	issueSnapshot, err := os.ReadFile(notes.IssuePath(result.WtPath))
	wantIssueSnapshot := strings.TrimSuffix(wantBody, "\n")
	if err != nil || string(issueSnapshot) != wantIssueSnapshot {
		t.Errorf("issue snapshot = %q, %v; want %q", issueSnapshot, err, wantIssueSnapshot)
	}
	if err := os.WriteFile(notes.Path(result.WtPath), []byte("# PR draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes.TitlePath(result.WtPath), []byte("feat: draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes.ProgressPath(result.WtPath), []byte("completed issue setup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := notes.InitializeIssueContext(result.WtPath, "replacement issue context"); err != nil {
		t.Fatal(err)
	}
	issueSnapshot, _ = os.ReadFile(notes.IssuePath(result.WtPath))
	progress, _ := os.ReadFile(notes.ProgressPath(result.WtPath))
	if string(issueSnapshot) != wantIssueSnapshot || string(progress) != "completed issue setup\n" {
		t.Errorf("context was changed on retry: issue=%q progress=%q", issueSnapshot, progress)
	}
	if draft, _, err := notes.Read(result.WtPath); err != nil || draft != "# PR draft\n" {
		t.Errorf("PR draft note = %q, %v", draft, err)
	}
	bootstrap, err := os.ReadFile(filepath.Join(result.WtPath, "AGENTS.md"))
	if err != nil || !strings.Contains(string(bootstrap), ".biomelab/note.md") || !strings.Contains(string(bootstrap), ".biomelab/pr-title.md") {
		t.Errorf("agent bootstrap does not load task notes: %q, %v", bootstrap, err)
	}
	if got := gitOutput(t, dir, "branch", "--show-current"); got != "master" {
		t.Errorf("main branch changed to %q", got)
	}
}

func TestCreateWorktreeFromIssue_DuplicateDoesNotOverwriteNotes(t *testing.T) {
	_, repo := newOpsRepo(t, nil)
	issue := github.IssueInfo{Number: 2, Title: "Original", URL: "https://example.test/2"}
	first := CreateWorktreeFromIssue(repo, issue, "issue-2-original")
	if first.Err != nil || first.NotesErr != nil {
		t.Fatalf("first create: %+v", first)
	}
	second := CreateWorktreeFromIssue(repo, github.IssueInfo{Number: 2, Title: "Replacement"}, first.BranchName)
	if second.Err == nil || second.WtPath != "" {
		t.Fatalf("duplicate result = %+v", second)
	}
	title, _, _ := notes.ReadTitle(first.WtPath)
	if title != "Original" {
		t.Fatalf("existing title overwritten with %q", title)
	}
}

func TestCreateWorktreeFromIssue_NotesConflictPreservesCheckedOutFile(t *testing.T) {
	dir, repo := newOpsRepo(t, map[string]string{".biomelab/note.md": "checked in\n"})
	result := CreateWorktreeFromIssue(repo, github.IssueInfo{Number: 3, Title: "Conflict", URL: "https://example.test/3"}, "issue-3-conflict")
	if result.Err != nil || result.WtPath == "" || result.NotesErr == nil {
		t.Fatalf("result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(result.WtPath, ".biomelab", "note.md"))
	if err != nil || string(data) != "checked in\n" {
		t.Fatalf("checked-out note changed: %q, %v (repo %s)", data, err, dir)
	}
	if _, err := os.Stat(notes.TitlePath(result.WtPath)); !os.IsNotExist(err) {
		t.Fatalf("title written despite conflict: %v", err)
	}
}

func TestCreateWorktreeFromIssue_ContextConflictDoesNotWriteOtherArtifacts(t *testing.T) {
	for _, artifact := range []string{".biomelab/issue.md", ".biomelab/progress.md"} {
		t.Run(artifact, func(t *testing.T) {
			_, repo := newOpsRepo(t, map[string]string{artifact: "checked in\n"})
			result := CreateWorktreeFromIssue(repo, github.IssueInfo{Number: 30, Title: "Conflict", URL: "https://example.test/30"}, "issue-30-conflict")
			if result.Err != nil || result.WtPath == "" || result.NotesErr == nil {
				t.Fatalf("result = %+v", result)
			}
			data, err := os.ReadFile(filepath.Join(result.WtPath, filepath.FromSlash(artifact)))
			if err != nil || string(data) != "checked in\n" {
				t.Fatalf("checked-out context changed: %q, %v", data, err)
			}
			for _, path := range []string{notes.Path(result.WtPath), notes.TitlePath(result.WtPath), notes.IssuePath(result.WtPath), notes.ProgressPath(result.WtPath)} {
				if path == filepath.Join(result.WtPath, filepath.FromSlash(artifact)) {
					continue
				}
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("artifact written despite context conflict: %s (%v)", path, err)
				}
			}
		})
	}
}

func TestCreateWorktreeFromIssue_RejectsStorageExclusionFailure(t *testing.T) {
	dir, repo := newOpsRepo(t, nil)
	if err := os.RemoveAll(filepath.Join(dir, ".git", "info")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "info"), []byte("blocks exclude directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CreateWorktreeFromIssue(repo, github.IssueInfo{Number: 4, Title: "Partial", URL: "https://example.test/4"}, "issue-4-partial")
	if result.Err == nil || !strings.Contains(result.Err.Error(), "exclude generated worktrees directory") || result.WtPath != "" || result.NotesErr != nil {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".biomelab-worktrees")); !os.IsNotExist(err) {
		t.Fatalf("storage created without exclusion: %v", err)
	}
	if branch := gitOutput(t, dir, "branch", "--list", "issue-4-partial"); branch != "" {
		t.Fatalf("branch created without exclusion: %q", branch)
	}
}

func TestWriteIssueNotes_ExclusionFailurePreservesCreatedWorktree(t *testing.T) {
	dir, repo := newOpsRepo(t, nil)
	const branch = "issue-4-partial"
	if err := repo.CreateWorktree(branch); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(dir, ".biomelab-worktrees", branch)
	// Storage exclusion must succeed before creation. Break it afterwards to
	// exercise the independent note-writing phase, without timing or permissions.
	if err := os.RemoveAll(filepath.Join(dir, ".git", "info")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "info"), []byte("blocks exclude directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeIssueNotes(wtPath, github.IssueInfo{Number: 4, Title: "Partial", URL: "https://example.test/4"})
	if err == nil || !strings.Contains(err.Error(), "save issue title") || !strings.Contains(err.Error(), "save issue note") || !strings.Contains(err.Error(), "ensure excluded") {
		t.Fatalf("missing independent note-write failure: %v", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("created worktree not preserved: %v", err)
	}
	if got := gitOutput(t, wtPath, "branch", "--show-current"); got != branch {
		t.Fatalf("created branch not preserved: %q", got)
	}
	for _, path := range []string{notes.TitlePath(wtPath), notes.Path(wtPath), notes.IssuePath(wtPath), notes.ProgressPath(wtPath)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("artifact remained visible after exclusion failure: %s (%v)", path, err)
		}
	}
}

func TestWriteIssueNotes_RejectsSymlinkArtifact(t *testing.T) {
	wtPath := newNotesTestRepo(t)
	noteDir := filepath.Join(wtPath, ".biomelab")
	if err := os.Mkdir(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(external, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, notes.Path(wtPath)); err != nil {
		t.Fatal(err)
	}
	err := writeIssueNotes(wtPath, github.IssueInfo{Number: 5, Title: "Unsafe", URL: "https://example.test/5"})
	if err == nil || !strings.Contains(err.Error(), "notes conflict") {
		t.Fatalf("writeIssueNotes error = %v", err)
	}
	data, err := os.ReadFile(external)
	if err != nil || string(data) != "keep\n" {
		t.Fatalf("symlink target changed: %q, %v", data, err)
	}
}

// disableCommitSigning keeps a developer's global commit.gpgSign=true from
// making go-git refuse test commits (it has no signer plugin registered).
func disableCommitSigning(t *testing.T, repo *gogit.Repository) {
	t.Helper()
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Commit.GpgSign = config.NewOptBool(false)
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func newNotesTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	disableCommitSigning(t, raw)
	wt, err := raw.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("initial", &gogit.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newOpsRepo(t *testing.T, files map[string]string) (string, *gitops.Repository) {
	t.Helper()
	dir := t.TempDir()
	raw, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	disableCommitSigning(t, raw)
	wt, err := raw.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if files == nil {
		files = make(map[string]string)
	}
	files["README.md"] = "initial\n"
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	_, err = wt.Commit("initial", &gogit.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gitops.OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, repo
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
