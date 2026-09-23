package sandbox

import (
	"reflect"
	"testing"
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
	tests := []struct {
		name     string
		stored   string
		repoName string
		repoPath string
		agent    string
		want     []string
	}{
		{
			// biomelab stored "<repo>-<agent>" but sbx actually created
			// "<agent>-<repo>". Both orderings must appear in the list.
			name:     "both orderings included when repo is owner/name",
			stored:   "pay2class-claude",
			repoName: "mdelapenya/pay2class",
			repoPath: "/Users/me/src/github.com/mdelapenya/pay2class",
			agent:    "claude",
			want: []string{
				"pay2class-claude",
				"mdelapenya-pay2class-claude",
				"claude-mdelapenya-pay2class",
				"claude-pay2class",
			},
		},
		{
			name:     "all forms converge — deduplicated",
			stored:   "acme-widget-claude",
			repoName: "acme/widget",
			repoPath: "/tmp/widget",
			agent:    "claude",
			want: []string{
				"acme-widget-claude",
				"claude-acme-widget",
				"widget-claude",
				"claude-widget",
			},
		},
		{
			name:     "stored empty — still derives from repo in both orderings",
			stored:   "",
			repoName: "owner/repo",
			repoPath: "/tmp/repo",
			agent:    "claude",
			want: []string{
				"owner-repo-claude",
				"claude-owner-repo",
				"repo-claude",
				"claude-repo",
			},
		},
		{
			name:     "no agent — only stored name returned",
			stored:   "abc",
			repoName: "owner/repo",
			repoPath: "/tmp/repo",
			agent:    "",
			want:     []string{"abc"},
		},
		{
			name:   "everything empty",
			stored: "",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Candidates(tt.stored, tt.repoName, tt.repoPath, tt.agent)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Candidates() = %v, want %v", got, tt.want)
			}
		})
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
