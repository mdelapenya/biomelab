package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreateArgs(t *testing.T) {
	t.Run("no kits", func(t *testing.T) {
		got := CreateArgs("my-sandbox", "claude", "/tmp/repo", nil)
		want := []string{"sbx", "create", "--name", "my-sandbox", "claude", "/tmp/repo"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CreateArgs() = %v, want %v", got, want)
		}
	})
	t.Run("with kits", func(t *testing.T) {
		got := CreateArgs("my-sandbox", "claude", "/tmp/repo", []string{
			"git+https://example.com#dir=a",
			"git+https://example.com#dir=b",
		})
		want := []string{
			"sbx", "create", "--name", "my-sandbox",
			"--kit", "git+https://example.com#dir=a",
			"--kit", "git+https://example.com#dir=b",
			"claude", "/tmp/repo",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CreateArgs() = %v, want %v", got, want)
		}
	})
}

func TestCreateWithKitArgsPlacesSandboxBasePositionally(t *testing.T) {
	got := CreateWithKitArgs("my-sandbox", "docker.io/sbx/claude-kit:latest", "/tmp/repo", []string{
		"docker.io/sbx/playwright-kit:latest", "docker.io/sbx/task-kit:latest",
	})
	want := []string{"sbx", "create", "--name", "my-sandbox",
		"--kit", "docker.io/sbx/playwright-kit:latest",
		"--kit", "docker.io/sbx/task-kit:latest",
		"docker.io/sbx/claude-kit:latest", "/tmp/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CreateWithKitArgs = %v, want %v", got, want)
	}
}

func TestRunAttachArgs(t *testing.T) {
	got := RunAttachArgs("my-sandbox")
	want := []string{"sbx", "run", "--name", "my-sandbox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunAttachArgs() = %v, want %v", got, want)
	}
}

func TestExecAgentArgs(t *testing.T) {
	got := ExecAgentArgs("my-sandbox", "/Users/me/repo/.biomelab-worktrees/feat", "claude")
	want := []string{
		"sbx", "exec", "-it", "-w", "/Users/me/repo/.biomelab-worktrees/feat",
		"my-sandbox", "bash", "-c",
		"if [ -f /usr/local/lib/sandbox/start-agent ]; then exec /bin/bash /usr/local/lib/sandbox/start-agent; else exec claude; fi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExecAgentArgs() = %v, want %v", got, want)
	}
}

func TestExecAgentArgsTranslatesWindowsWorktreePath(t *testing.T) {
	got := ExecAgentArgs("my-sandbox", `C:\Users\me\repo\.biomelab-worktrees\feat`, "claude")
	// sbx mounts the host workspace at its POSIX equivalent, so the host
	// spelling would make `sbx exec -w` fail inside the Linux container.
	if got[4] != "/c/Users/me/repo/.biomelab-worktrees/feat" {
		t.Errorf("ExecAgentArgs() workdir = %q, want the container path", got[4])
	}
}

func TestContainerPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"posix path unchanged", "/Users/me/repo/wt", "/Users/me/repo/wt"},
		{"drive letter", `C:\Users\me\repo`, "/c/Users/me/repo"},
		{"lowercase drive", `d:\work`, "/d/work"},
		{"drive root", `C:\`, "/c/"},
		{"forward slashes", "C:/Users/me", "/c/Users/me"},
		{"non-ascii and spaces preserved", `C:\Users\Peña\a b\c`, "/c/Users/Peña/a b/c"},
		// A UNC share has no mirrored mount point; rewriting it would invent a
		// path that does not exist instead of failing where the user can see it.
		{"unc path unchanged", `\\server\share\repo`, `\\server\share\repo`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainerPath(tt.in); got != tt.want {
				t.Errorf("ContainerPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"claude", "claude"},
		{"/a/b-c.d", "/a/b-c.d"},
		{"", "''"},
		{"a b", "'a b'"},
		{"it's", `'it'\''s'`},
		{"$HOME", "'$HOME'"},
	}
	for _, tt := range tests {
		if got := ShellQuote(tt.in); got != tt.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"owner/repo"}, "owner-repo"},
		{[]string{"owner/repo", "claude"}, "owner-repo-claude"},
		{[]string{"My Repo", "gemini"}, "my-repo-gemini"},
	}
	for _, tt := range tests {
		got := SanitizeName(tt.parts...)
		if got != tt.want {
			t.Errorf("SanitizeName(%v) = %q, want %q", tt.parts, got, tt.want)
		}
	}
}

func TestCandidates(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "one", "widget")
	b := filepath.Join(root, "two", "widget")
	for _, path := range []string{a, b} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	an := GeneratedName(a, "claude")
	bn := GeneratedName(b, "claude")
	if an == bn || !strings.HasPrefix(an, "widget-claude-") || !strings.HasPrefix(bn, "widget-claude-") {
		t.Fatalf("repo identity collapsed: %q and %q", an, bn)
	}
	if got := Candidates("", "widget", a, "claude"); !reflect.DeepEqual(got, []string{an}) {
		t.Fatalf("new repo candidates = %v", got)
	}
	if got := Candidates("widget-claude", "widget", a, "claude"); !reflect.DeepEqual(got, []string{"widget-claude", an}) {
		t.Fatalf("stored legacy name was lost: %v", got)
	}
	if _, _, ok := MatchStatus(map[string]Status{bn: StatusRunning, "widget-claude": StatusStopped}, Candidates("", "widget", a, "claude")); ok {
		t.Fatal("repo adopted another repo's or an ambiguous legacy sandbox")
	}
	if got := Candidates("", "widget", "", "claude"); got != nil {
		t.Fatalf("missing repo path yielded candidates: %v", got)
	}
	if got := Candidates("explicit", "widget", "", "claude"); !reflect.DeepEqual(got, []string{"explicit"}) {
		t.Fatalf("explicit association lost: %v", got)
	}
}

func TestGeneratedNameCanonicalPath(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, want := GeneratedName(link, "Claude"), GeneratedName(root, "Claude"); got != want {
		t.Fatalf("alias generated a different name: %q != %q", got, want)
	}
}

func TestPreflightContextCancelsDaemonProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sbx shell script is Unix-only")
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "sbx"), []byte("#!/bin/sh\nexec sleep 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := PreflightContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("probe cancellation = %v after %s", err, time.Since(start))
	}
}

func TestMatchStatus(t *testing.T) {
	m := map[string]Status{
		"mdelapenya-pay2class-claude": StatusRunning,
		"other-claude":                StatusStopped,
	}
	t.Run("first candidate matches", func(t *testing.T) {
		name, status, ok := MatchStatus(m, []string{"mdelapenya-pay2class-claude", "pay2class-claude"})
		if !ok || name != "mdelapenya-pay2class-claude" || status != StatusRunning {
			t.Errorf("got (%q, %v, %v)", name, status, ok)
		}
	})
	t.Run("second candidate matches when first missing", func(t *testing.T) {
		name, status, ok := MatchStatus(m, []string{"pay2class-claude", "mdelapenya-pay2class-claude"})
		if !ok || name != "mdelapenya-pay2class-claude" || status != StatusRunning {
			t.Errorf("got (%q, %v, %v)", name, status, ok)
		}
	})
	t.Run("no candidate matches", func(t *testing.T) {
		_, _, ok := MatchStatus(m, []string{"nope", "also-nope"})
		if ok {
			t.Error("expected ok=false")
		}
	})
	t.Run("empty candidates", func(t *testing.T) {
		_, _, ok := MatchStatus(m, nil)
		if ok {
			t.Error("expected ok=false")
		}
	})
}

func TestCommandString(t *testing.T) {
	t.Run("plain args", func(t *testing.T) {
		got := CommandString([]string{"sbx", "run", "--name", "my-sandbox"})
		want := "sbx run --name my-sandbox"
		if got != want {
			t.Errorf("CommandString() = %q, want %q", got, want)
		}
	})
	t.Run("quotes args needing it", func(t *testing.T) {
		got := CommandString([]string{"bash", "-c", "echo hi"})
		want := "bash -c 'echo hi'"
		if got != want {
			t.Errorf("CommandString() = %q, want %q", got, want)
		}
	})
}
