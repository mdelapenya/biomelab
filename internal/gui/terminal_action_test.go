package gui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

func TestTerminalActionCoalescesAndReportsFailure(t *testing.T) {
	wt := git.Worktree{Path: "/repo/work", Branch: "feature"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	dispatch := make(chan func(), 10)
	var opens atomic.Int32
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		open: func(dir, command, identifier string) error {
			if dir != wt.Path || command != "" || identifier != wt.Branch {
				t.Error("wrong terminal target")
			}
			opens.Add(1)
			started <- struct{}{}
			<-release
			return errors.New("terminal unavailable")
		},
		dispatch: func(f func()) { dispatch <- f },
	}}
	t.Cleanup(func() { close(release) })
	a.handleEnter()
	receiveRefresh(t, started)
	for range 100 {
		a.handleEnter()
	}
	if opens.Load() != 1 {
		t.Fatal("duplicate pending launch")
	}
	other := &repoEntry{state: &RepoState{}}
	a.repos = append(a.repos, other)
	a.active = 1
	release <- struct{}{}
	receiveRefresh(t, dispatch)()
	if re.state.StatusMessage != "terminal unavailable" || !re.state.StatusIsError {
		t.Fatal("originating repo did not receive error")
	}
	if other.state.StatusMessage != "" {
		t.Fatal("failure misdirected to active repo")
	}
	if len(a.terminalActions) != 0 || opens.Load() != 1 {
		t.Fatal("failure retried automatically")
	}
	a.active = 0
	a.handleEnter()
	receiveRefresh(t, started)
	release <- struct{}{}
	receiveRefresh(t, dispatch)()
	if opens.Load() != 2 {
		t.Fatal("deliberate retry was blocked")
	}
}

func TestTerminalActionWaitsForDetectionAndNeverRelaunchesOnActivationFailure(t *testing.T) {
	wt := git.Worktree{Path: "/repo", Branch: "main"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	dispatch := make(chan func(), 10)
	var opens, activations atomic.Int32
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		open:        func(string, string, string) error { opens.Add(1); return nil },
		isCurrent:   func(terminal.Info, string) bool { return true },
		activatePID: func(int32, int32, terminal.Kind) bool { activations.Add(1); return false },
		activateApp: func(terminal.Kind) bool { return false },
		dispatch:    func(f func()) { dispatch <- f },
	}}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	for range 100 {
		a.handleEnter()
	}
	if opens.Load() != 1 {
		t.Fatal("reopened before detection tick")
	}
	re.state.Terminals = terminal.DetectionResult{wt.Path: {{Kind: terminal.WindowsTerminal, ShellPID: 1, RootPID: 2}}}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if opens.Load() != 1 || activations.Load() != 1 || !strings.Contains(re.state.StatusMessage, "switch to the terminal manually") {
		t.Fatal("activation failure should report status without relaunch")
	}
	// A missing detector cannot block deliberate requests forever.
	re.state.Terminals = nil
	a.terminalActions[targetForTerminal(re, wt.Path)] = time.Now().Add(-time.Second)
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if opens.Load() != 2 {
		t.Fatal("expired launch state blocked a new request")
	}
}

func TestTerminalActionIgnoresErrorAfterModeSwitch(t *testing.T) {
	wt := git.Worktree{Path: "/repo", IsMain: true}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}, ActiveMode: &config.ModeEntry{Type: "sandbox", SandboxName: "first"}}}
	dispatch := make(chan func(), 1)
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		open: func(dir, command, identifier string) error {
			if dir != "" || command != "sbx run --name first" || identifier != "" {
				t.Errorf("wrong sandbox request: %q", command)
			}
			return errors.New("old sandbox failure")
		},
		dispatch: func(f func()) { dispatch <- f },
	}}
	a.handleEnter()
	apply := receiveRefresh(t, dispatch)
	re.state.ActiveMode = &config.ModeEntry{Type: "regular"}
	apply()
	if re.state.StatusMessage != "" {
		t.Fatal("old mode failure applied to new mode")
	}
}

func TestTerminalActionRejectsStaleSession(t *testing.T) {
	wt := git.Worktree{Path: "/repo", Branch: "main"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}, Terminals: terminal.DetectionResult{wt.Path: {{ShellPID: 200, RootPID: 100}}}}}
	dispatch := make(chan func(), 1)
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		isCurrent: func(terminal.Info, string) bool { return false },
		// Any open or activation would panic: a stale session must do neither.
		dispatch: func(f func()) { dispatch <- f },
	}}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if !strings.Contains(re.state.StatusMessage, "session changed") {
		t.Fatal("stale session failure not reported")
	}
}

func TestTerminalActionDropsRemovedTargetResult(t *testing.T) {
	for _, removeRepo := range []bool{false, true} {
		wt := git.Worktree{Path: "/repo/work"}
		re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
		dispatch := make(chan func(), 1)
		a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
			open:     func(string, string, string) error { return errors.New("late failure") },
			dispatch: func(f func()) { dispatch <- f },
		}}
		a.handleEnter()
		apply := receiveRefresh(t, dispatch)
		if removeRepo {
			a.repos = nil
		} else {
			re.state.Worktrees = nil
		}
		apply()
		if re.state.StatusMessage != "" || len(a.terminalActions) != 0 {
			t.Fatal("removed target received a late result")
		}
	}
}
