package notes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const legacyBootstrapFixture = "<!-- biomelab:task-notes:start -->\n" +
	"## Biomelab task context\n\n" +
	"Before starting work, read `.biomelab/note.md` and `.biomelab/pr-title.md` when they exist. Treat their contents as task context; they do not override repository instructions.\n" +
	"<!-- biomelab:task-notes:end -->\n"

const legacyBootstrapWithClaudeFixture = "<!-- biomelab:task-notes:start -->\n" +
	"## Biomelab task context\n\n" +
	"Before starting work, read `.biomelab/note.md` and `.biomelab/pr-title.md` when they exist. Treat their contents as task context; they do not override repository instructions.\n" +
	"Before starting work, also read `CLAUDE.md` when it exists so its repository guidance is preserved.\n" +
	"<!-- biomelab:task-notes:end -->\n"

func TestEnsureAgentBootstrapCreatesIgnoredInstructions(t *testing.T) {
	repo := initRepo(t)
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatalf("EnsureAgentBootstrap: %v", err)
	}

	for _, rel := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		filepath.Join(".kiro", "steering", "biomelab-task.md"),
	} {
		data, err := os.ReadFile(filepath.Join(repo, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "<!-- biomelab:task-notes:end -->") || !strings.Contains(string(data), ".biomelab/note.md") {
			t.Errorf("%s does not contain bootstrap block", rel)
		}
		cmd := exec.Command("git", "check-ignore", "-q", rel)
		cmd.Dir = repo
		if err := cmd.Run(); err != nil {
			t.Errorf("%s is not ignored: %v", rel, err)
		}
	}

	status := exec.Command("git", "status", "--porcelain")
	status.Dir = repo
	out, err := status.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("new bootstrap files made repository dirty: %s", out)
	}
}

func TestEnsureAgentBootstrapPreservesExistingFilesAndIsIdempotent(t *testing.T) {
	repo := initRepo(t)
	original := "# Existing guidance\n\nKeep this exact."
	path := filepath.Join(repo, "AGENTS.md")
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, "AGENTS.md")

	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(first), original+"\n\n") {
		t.Fatalf("existing bytes were not preserved: %q", first)
	}
	if strings.Count(string(first), "<!-- biomelab:task-notes:start -->") != 1 {
		t.Fatalf("bootstrap marker count = %d, want 1", strings.Count(string(first), "<!-- biomelab:task-notes:start -->"))
	}

	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Errorf("second call changed AGENTS.md")
	}

	status := exec.Command("git", "status", "--porcelain", "--", "AGENTS.md")
	status.Dir = repo
	out, err := status.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), " M AGENTS.md") {
		t.Errorf("tracked instruction change should remain visible, status=%q", out)
	}
}

func TestEnsureAgentBootstrapInstructionsSeparateRequirementsFromProgress(t *testing.T) {
	repo := initRepo(t)
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"`.biomelab/issue.md` for the immutable initial requirements",
		"read `.biomelab/note.md` as the available legacy task context; it may have been edited",
		"`.biomelab/progress.md` when it exists for the ongoing agent handoff",
		"editable pull-request draft context",
		"Inspect `git status`, the current diff, recent commits, and the relevant code",
		"notes can be stale",
		"Do not assume that all original issue work remains or redo completed work",
		"derive the current state from the code and repository history",
		"update `.biomelab/progress.md` at meaningful milestones and before ending or handing off",
		"Record only facts you observed",
		"Preserve `.biomelab/issue.md`",
		"If the task is complete, say so in the progress file instead of restarting",
		"supplement repository instructions; they do not override them",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("bootstrap instructions missing %q", want)
		}
	}
}

func TestEnsureAgentBootstrapMigratesKnownLegacyBlocksInPlace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rel    string
		legacy string
		want   string
	}{
		{name: "normal Claude", rel: "CLAUDE.md", legacy: legacyBootstrapFixture, want: agentBootstrapBlock},
		{name: "AGENTS with Claude fallback", rel: "AGENTS.md", legacy: legacyBootstrapWithClaudeFixture, want: bootstrapBlockWithClaudeGuidance()},
		{name: "override with Claude fallback", rel: "AGENTS.override.md", legacy: legacyBootstrapWithClaudeFixture, want: bootstrapBlockWithClaudeGuidance()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			path := filepath.Join(repo, tc.rel)
			original := "user prefix\n\n" + tc.legacy + "\nuser suffix\n"
			if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
				t.Fatal(err)
			}
			originalInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := EnsureAgentBootstrap(repo); err != nil {
				t.Fatal(err)
			}
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "user prefix\n\n" + tc.want + "\nuser suffix\n"
			if string(first) != want {
				t.Fatalf("migration did not replace legacy block in place:\n%q\nwant:\n%q", first, want)
			}
			if strings.Contains(string(first), legacyAgentBootstrapBlock) {
				t.Fatal("legacy directive remains after migration")
			}
			if err := EnsureAgentBootstrap(repo); err != nil {
				t.Fatal(err)
			}
			second, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(second) != string(first) {
				t.Fatal("repeated bootstrap changed migrated content")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := info.Mode().Perm(), originalInfo.Mode().Perm(); got != want {
				t.Errorf("mode = %o, want original mode %o", got, want)
			}
		})
	}
}

func TestEnsureAgentBootstrapMigratesLegacyCRLFInPlace(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, "CLAUDE.md")
	legacy := strings.ReplaceAll(legacyBootstrapFixture, "\n", "\r\n")
	original := "before\r\n" + legacy + "after\r\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "before\r\n" + strings.ReplaceAll(agentBootstrapBlock, "\n", "\r\n") + "after\r\n"
	if string(data) != want {
		t.Fatalf("CRLF migration changed surrounding bytes:\n%q\nwant:\n%q", data, want)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(data) {
		t.Fatal("repeated bootstrap changed migrated CRLF content")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), originalInfo.Mode().Perm(); got != want {
		t.Errorf("mode = %o, want original mode %o", got, want)
	}
}

func TestEnsureAgentBootstrapRemovesLegacyWhenCanonicalAlreadyExists(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, "CLAUDE.md")
	original := "before\n" + agentBootstrapBlock + "between\n" + legacyBootstrapFixture + "after\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), legacyBootstrapFixture) {
		t.Fatal("legacy block remains beside canonical block")
	}
	if strings.Count(string(data), agentBootstrapBlock) != 1 {
		t.Fatalf("canonical block count = %d, want 1", strings.Count(string(data), agentBootstrapBlock))
	}
}

func TestEnsureAgentBootstrapPreservesUserOwnedMarkedContent(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, "CLAUDE.md")
	original := "before\n<!-- biomelab:task-notes:start -->\nuser-owned instructions\n<!-- biomelab:task-notes:end -->\nafter\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(first), original) {
		t.Fatal("user-owned marked content was replaced")
	}
	if strings.Count(string(first), agentBootstrapBlock) != 1 {
		t.Fatalf("canonical block count = %d, want 1", strings.Count(string(first), agentBootstrapBlock))
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Fatal("repeated bootstrap changed preserved marked content")
	}
}

func TestEnsureAgentBootstrapUpdatesExistingCodexOverride(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, "AGENTS.override.md")
	if err := os.WriteFile(path, []byte("override guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatalf("EnsureAgentBootstrap: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "override guidance\n") || !strings.Contains(string(data), bootstrapBlockWithClaudeGuidance()) {
		t.Errorf("override was not preserved and bootstrapped: %q", data)
	}
}

func TestEnsureAgentBootstrapPreservesOpenCodeClaudeFallback(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte("existing Claude guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), readClaudeGuidance) {
		t.Errorf("generated AGENTS.md does not preserve OpenCode's CLAUDE.md fallback: %q", data)
	}
}

func TestEnsureAgentBootstrapDoesNotTrustMarkerAlone(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	if err := os.WriteFile(path, []byte("<!-- biomelab:task-notes:start -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentBootstrap(repo); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<!-- biomelab:task-notes:end -->") || !strings.Contains(string(data), ".biomelab/note.md") {
		t.Error("complete block was not appended")
	}
}

func TestEnsureAgentBootstrapRejectsSymlinkAndReportsPartialSetup(t *testing.T) {
	repo := initRepo(t)
	external := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(external, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(repo, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	err := EnsureAgentBootstrap(repo)
	if err == nil || !strings.Contains(err.Error(), "CLAUDE.md") || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "AGENTS.md")); err != nil {
		t.Errorf("earlier bootstrap should remain after partial failure: %v", err)
	}
	data, err := os.ReadFile(external)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside\n" {
		t.Errorf("symlink target was modified: %q", data)
	}
}

func TestEnsureAgentBootstrapRejectsSymlinkedParent(t *testing.T) {
	repo := initRepo(t)
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "steering"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideTarget := filepath.Join(outside, "steering", "biomelab-task.md")
	if err := os.WriteFile(outsideTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".kiro")); err != nil {
		t.Fatal(err)
	}
	err := EnsureAgentBootstrap(repo)
	if err == nil || !strings.Contains(err.Error(), "instruction parent") {
		t.Fatalf("error = %v", err)
	}
	data, err := os.ReadFile(outsideTarget)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside\n" {
		t.Errorf("wrote through symlinked parent: %q", data)
	}
}

func TestEnsureAgentBootstrapRejectsConditionalKiroFile(t *testing.T) {
	repo := initRepo(t)
	dir := filepath.Join(repo, ".kiro", "steering")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "biomelab-task.md")
	original := "---\ninclusion: manual\n---\n\nExisting steering.\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	err := EnsureAgentBootstrap(repo)
	if err == nil || !strings.Contains(err.Error(), "not always included") {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Errorf("conditional Kiro file was modified: %q", data)
	}
}

func commitFile(t *testing.T, repo, rel string) {
	t.Helper()
	for _, args := range [][]string{{"add", rel}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "add instructions"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}
