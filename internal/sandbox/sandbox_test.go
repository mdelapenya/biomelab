package sandbox

import (
	"reflect"
	"testing"
)

func TestCreateArgs(t *testing.T) {
	got := CreateArgs("my-sandbox", "claude", "/tmp/repo")
	want := []string{"sbx", "create", "--name", "my-sandbox", "claude", "/tmp/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CreateArgs() = %v, want %v", got, want)
	}
}

func TestRunDetachedWithBranchArgs(t *testing.T) {
	got := RunDetachedWithBranchArgs("my-sandbox", "feature/login")
	want := []string{"sbx", "run", "-d", "--branch", "feature/login", "my-sandbox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunDetachedWithBranchArgs() = %v, want %v", got, want)
	}
}

func TestRunWithBranchArgs(t *testing.T) {
	got := RunWithBranchArgs("my-sandbox", "feature/login")
	want := []string{"sbx", "run", "--branch", "feature/login", "my-sandbox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunWithBranchArgs() = %v, want %v", got, want)
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
	args := []string{"sbx", "run", "--branch", "feature", "my-sandbox"}
	got := CommandString(args)
	want := "sbx run --branch feature my-sandbox"
	if got != want {
		t.Errorf("CommandString() = %q, want %q", got, want)
	}
}
