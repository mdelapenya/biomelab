package terminal

import (
	"context"
	"testing"

	"github.com/mdelapenya/biomelab/internal/process"
)

func TestSessionExistsRejectsStaleAssociation(t *testing.T) {
	lister := &mockLister{procs: []process.Info{
		{PID: 100, PPID: 1, Name: "Terminal"},
		{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
	}}
	d := NewDetectorWithLister(lister)
	expected := Info{Kind: TerminalApp, ShellPID: 200, RootPID: 100}
	if !d.sessionExists(context.Background(), expected, "/project") {
		t.Fatal("live session not found")
	}
	lister.procs[1].Cwd = "/different-worktree"
	if d.sessionExists(context.Background(), expected, "/project") {
		t.Fatal("stale CWD accepted")
	}
	lister.procs[1].Cwd = "/project"
	lister.procs[0].Name = "code"
	if d.sessionExists(context.Background(), expected, "/project") {
		t.Fatal("reused emulator PID accepted")
	}
	lister.procs = lister.procs[:1]
	if d.sessionExists(context.Background(), expected, "/project") {
		t.Fatal("dead shell accepted")
	}
}
