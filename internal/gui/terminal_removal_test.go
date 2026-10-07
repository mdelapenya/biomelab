package gui

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWorktreeRemovalWaitsForMatchingTerminalCleanup(t *testing.T) {
	target := cardTerminalKey{path: "/repo/worktrees/target"}
	other := cardTerminalKey{path: "/repo/worktrees/other"}
	targetStopped := make(chan struct{})
	otherStopped := make(chan struct{})
	targetCancelled := false
	otherCancelled := false
	a := &App{cardTerminals: &cardTerminals{sessions: map[cardTerminalKey]*cardTerminalSession{
		target: {cancel: func() { targetCancelled = true }, stopped: targetStopped},
		other:  {cancel: func() { otherCancelled = true }, stopped: otherStopped},
	}}}
	waits := a.stopCardTerminals(func(k cardTerminalKey) bool { return k.path == target.path })
	if !targetCancelled || otherCancelled || len(waits) != 1 || a.cardTerminals.sessions[other] == nil || a.cardTerminals.sessions[target] != nil {
		t.Fatal("removal did not cancel only the matching terminal")
	}
	removed := make(chan struct{}, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- removeAfterCardTerminalCleanup(context.Background(), waits, time.Second, func() error {
			removed <- struct{}{}
			return nil
		})
	}()
	select {
	case <-removed:
		t.Fatal("worktree removal began before terminal cleanup")
	case err := <-finished:
		t.Fatalf("cleanup wait ended early: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(targetStopped)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("removal did not resume after terminal cleanup")
	}
	select {
	case <-removed:
	default:
		t.Fatal("removal was never called")
	}
	if otherCancelled || a.cardTerminals.sessions[other] == nil {
		t.Fatal("unrelated terminal did not survive removal")
	}
}

func TestWorktreeRemovalSkipsDeleteOnCleanupTimeout(t *testing.T) {
	stopped := make(chan struct{})
	called := false
	err := removeAfterCardTerminalCleanup(context.Background(), []<-chan struct{}{stopped}, 10*time.Millisecond, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "worktree was not deleted") || called {
		t.Fatalf("timeout did not prevent deletion: err=%v called=%v", err, called)
	}
}

func TestWorktreeRemovalBlocksReopenUntilCompletion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout bool
	}{
		{name: "removed"},
		{name: "cleanup timed out", timeout: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, re, w := terminalFixture(t)
			wt := re.state.Worktrees[2]
			key := cardKey(re, wt)
			old := seedCardTerminal(a, re, wt)
			other := seedCardTerminal(a, re, re.state.Worktrees[1])
			stopped := make(chan struct{})
			old.stopped = stopped
			cancelled := false
			old.cancel = func() { cancelled = true }
			a.handleEnter()
			removal := a.beginCardTerminalRemoval(key.path)
			if removal == nil || a.beginCardTerminalRemoval(key.path) != nil {
				t.Fatal("removal guard was not established exactly once")
			}
			waits := a.stopCardTerminals(func(k cardTerminalKey) bool { return k.path == key.path })
			if !cancelled || len(waits) != 1 {
				t.Fatal("matching session was not stopped")
			}
			a.handleEnter()
			a.confirmTerminalStop(true)
			if a.cardTerminals.sessions[key] != nil || a.cardTerminals.sessions[cardKey(re, re.state.Worktrees[1])] != other {
				t.Fatal("guard allowed a new shell or stopped an unrelated session")
			}
			removed := false
			finished := make(chan error, 1)
			go func() {
				finished <- removeAfterCardTerminalCleanup(context.Background(), waits, 50*time.Millisecond, func() error {
					removed = true
					return nil
				})
			}()
			if !tc.timeout {
				close(stopped)
			}
			err := <-finished
			if tc.timeout && (err == nil || removed) {
				t.Fatalf("timeout proceeded with removal: err=%v", err)
			}
			if !tc.timeout && (err != nil || !removed) {
				t.Fatalf("cleanup did not permit removal: err=%v", err)
			}
			if tc.timeout {
				if a.beginCardTerminalRemoval(key.path) != nil {
					t.Fatal("timeout released guard while old shell was unconfirmed")
				}
				a.handleEnter()
				a.confirmTerminalStop(true)
				if a.cardTerminals.sessions[key] != nil {
					t.Fatal("timeout allowed a new terminal session")
				}
				events := make(chan func(), 1)
				a.cardTerminals.dispatch = func(fn func(), _ bool) { events <- fn }
				watcherDone := a.releaseCardTerminalRemovalAfterCleanup(key.path, removal, waits)
				close(stopped)
				select {
				case fn := <-events:
					fn()
				case <-time.After(time.Second):
					t.Fatal("guard was not released after old shell stopped")
				}
				<-watcherDone
			} else {
				a.finishCardTerminalRemoval(key.path, removal)
			}
			if retry := a.beginCardTerminalRemoval(key.path); retry == nil {
				t.Fatal("worktree removal could not be retried after cleanup")
			} else {
				a.finishCardTerminalRemoval(key.path, retry)
			}
			replacement := seedCardTerminal(a, re, wt)
			a.handleEnter()
			if a.cardTerminals.sessions[key] != replacement || w.Canvas().Focused() != replacement.view {
				t.Fatal("terminal guard remained after removal completed")
			}
		})
	}
}

func TestWorktreeRemovalIgnoresStaleGuardRelease(t *testing.T) {
	a, re, _ := terminalFixture(t)
	path := cardKey(re, re.state.Worktrees[2]).path
	first := a.beginCardTerminalRemoval(path)
	a.finishCardTerminalRemoval(path, first)
	second := a.beginCardTerminalRemoval(path)
	a.finishCardTerminalRemoval(path, first)
	if a.beginCardTerminalRemoval(path) != nil {
		t.Fatal("stale cleanup released a newer removal guard")
	}
	a.finishCardTerminalRemoval(path, second)
}
