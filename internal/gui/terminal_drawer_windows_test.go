//go:build windows

package gui

import (
	"errors"
	"testing"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

func TestCardTerminalWindowsUsesExternalFallback(t *testing.T) {
	wt := git.Worktree{Path: t.TempDir(), Branch: "feature"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	opened := make(chan string, 1)
	dispatch := make(chan func(), 1)
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		find: noTerminal,
		open: func(dir, command, title string) (*terminal.Session, error) {
			opened <- dir
			return nil, errors.New("fixture launch")
		},
		dispatch: func(fn func()) { dispatch <- fn },
	}}
	a.openCardTerminal(re, wt)
	if got := receiveRefresh(t, opened); got != wt.Path {
		t.Fatalf("fallback dir %q", got)
	}
	receiveRefresh(t, dispatch)()
	if a.cardTerminals != nil {
		t.Fatal("unsupported transport was initialized")
	}
}
