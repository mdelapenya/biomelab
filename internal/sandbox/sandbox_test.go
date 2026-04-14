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

func TestCommandString(t *testing.T) {
	args := []string{"sbx", "run", "--branch", "feature", "my-sandbox"}
	got := CommandString(args)
	want := "sbx run --branch feature my-sandbox"
	if got != want {
		t.Errorf("CommandString() = %q, want %q", got, want)
	}
}
