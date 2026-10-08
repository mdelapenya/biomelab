//go:build windows

package gui

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mdelapenya/biomelab/internal/terminal"
)

func forbidExternalTerminal(a *App) <-chan struct{} {
	opened := make(chan struct{}, 1)
	a.terminalDeps = &terminalDependencies{
		find: noTerminal,
		open: func(string, string, string) (*terminal.Session, error) {
			opened <- struct{}{}
			return nil, errors.New("unexpected external terminal launch")
		},
	}
	return opened
}

func assertNoExternalTerminal(t *testing.T, opened <-chan struct{}) {
	t.Helper()
	select {
	case <-opened:
		t.Fatal("card opened an external terminal")
	default:
	}
}

func TestCardTerminalWindowsFocusesAndReusesIntegratedSession(t *testing.T) {
	a, re, w := terminalFixture(t)
	opened := forbidExternalTerminal(a)
	wt := re.state.Worktrees[2]
	s := seedCardTerminal(a, re, wt)
	a.handleEnter()
	if a.cardTerminals.sessions[cardKey(re, wt)] != s || !a.cardTerminals.visible || w.Canvas().Focused() != s.view {
		t.Fatal("card did not focus the existing integrated session")
	}
	a.leaveCardTerminal()
	a.cardTerminals.visible = false
	a.handleEnter()
	if a.cardTerminals.sessions[cardKey(re, wt)] != s || w.Canvas().Focused() != s.view {
		t.Fatal("reopening duplicated a live integrated session")
	}
	assertNoExternalTerminal(t, opened)
}

// ConPTY normally echoes the command itself. Require the marker on its own
// line so the echoed input cannot masquerade as command output.
func terminalHasOutputLine(text, marker string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == marker {
			return true
		}
	}
	return false
}

// goroutineDumps records the tests that already logged a goroutine dump.
var goroutineDumps sync.Map

func drainWindowsTerminalEvents(t *testing.T, events <-chan func(), ready func() bool, session ...*cardTerminalSession) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	// Re-check readiness periodically: some conditions (a session's done
	// channel closing) become true without dispatching another event, and
	// waiting only for events would then block until the deadline.
	recheck := time.NewTicker(10 * time.Millisecond)
	defer recheck.Stop()
	for !ready() {
		select {
		case fn := <-events:
			fn()
		case <-recheck.C:
		case <-deadline.C:
			// Dump every goroutine so a hang in CI shows where it is stuck;
			// once per test and capped, so repeated timeouts cannot flood the
			// log and truncate the state line below.
			if _, dumped := goroutineDumps.LoadOrStore(t.Name(), true); !dumped {
				buf := make([]byte, 256<<10)
				t.Logf("goroutines at timeout:\n%s", buf[:runtime.Stack(buf, true)])
			}
			if len(session) == 0 {
				t.Fatal("Windows terminal operation timed out")
			}
			output := session[0].view.Text()
			if len(output) > 512 {
				output = output[len(output)-512:]
			}
			t.Fatalf("Windows terminal operation timed out: state=%q running=%v recent output=%q", session[0].state, session[0].running, output)
		}
	}
}

func TestCardTerminalWindowsRealShellLifecycle(t *testing.T) {
	a, re, w := terminalFixture(t)
	opened := forbidExternalTerminal(a)
	p := a.ensureCardTerminals()
	events := make(chan func(), 64)
	p.dispatch = func(fn func(), wait bool) {
		done := make(chan struct{})
		events <- func() { fn(); close(done) }
		if wait {
			<-done
		}
	}
	t.Cleanup(func() {
		a.closeCardTerminals()
		for _, s := range p.sessions {
			drainWindowsTerminalEvents(t, events, func() bool {
				select {
				case <-s.done:
					return true
				default:
					return false
				}
			}, s)
		}
		p.shutdown()
	})
	a.handleEnter()
	key := cardKey(re, re.state.Worktrees[2])
	s := p.sessions[key]
	if s == nil || !p.visible || w.Canvas().Focused() != s.view {
		t.Fatal("card did not open and focus integrated terminal")
	}
	drainWindowsTerminalEvents(t, events, func() bool { return s.state != "Starting" }, s)
	if s.state != "Running" {
		t.Fatalf("shell startup: %s", s.state)
	}
	a.handleEnter()
	if p.sessions[key] != s {
		t.Fatal("opening card duplicated live shell")
	}
	assertNoExternalTerminal(t, opened)
	if _, err := s.input.Write([]byte("echo BIOMELAB_WINDOWS_OUTPUT_OK\r")); err != nil {
		t.Fatal(err)
	}
	drainWindowsTerminalEvents(t, events, func() bool {
		return terminalHasOutputLine(s.view.Text(), "BIOMELAB_WINDOWS_OUTPUT_OK")
	}, s)
	if _, err := s.input.Write([]byte("exit\r")); err != nil {
		t.Fatal(err)
	}
	drainWindowsTerminalEvents(t, events, func() bool { return !s.running }, s)
	if s.state != "Exited" || p.visible || w.Canvas().Focused() != nil {
		t.Fatalf("normal exit: state=%s visible=%v focused=%v", s.state, p.visible, w.Canvas().Focused())
	}
	a.handleEnter()
	fresh := p.sessions[key]
	if fresh == s || !p.visible || w.Canvas().Focused() != fresh.view {
		t.Fatal("reopening did not focus a fresh shell")
	}
	drainWindowsTerminalEvents(t, events, func() bool { return fresh.state != "Starting" }, fresh)
	if fresh.state != "Running" {
		t.Fatalf("fresh shell startup: %s", fresh.state)
	}
	if _, err := fresh.input.Write([]byte("echo BIOMELAB_WINDOWS_FRESH_OK\r")); err != nil {
		t.Fatal(err)
	}
	drainWindowsTerminalEvents(t, events, func() bool {
		return terminalHasOutputLine(fresh.view.Text(), "BIOMELAB_WINDOWS_FRESH_OK")
	}, fresh)
	assertNoExternalTerminal(t, opened)
}

func TestCardTerminalWindowsStartFailureStaysInDrawer(t *testing.T) {
	a, re, _ := terminalFixture(t)
	opened := forbidExternalTerminal(a)
	re.state.Worktrees[2].Path = filepath.Join(t.TempDir(), "missing-worktree")
	p := a.ensureCardTerminals()
	events := make(chan func(), 8)
	p.dispatch = func(fn func(), wait bool) {
		done := make(chan struct{})
		events <- func() { fn(); close(done) }
		if wait {
			<-done
		}
	}
	a.handleEnter()
	s := p.sessions[p.key]
	if s == nil {
		t.Fatal("missing integrated session")
	}
	drainWindowsTerminalEvents(t, events, func() bool { return !s.running }, s)
	if !strings.HasPrefix(s.state, "Failed:") || !strings.Contains(s.view.Text(), "open an external terminal") {
		t.Fatalf("startup failure was not visible in the drawer: state=%q text=%q", s.state, s.view.Text())
	}
	assertNoExternalTerminal(t, opened)
	p.shutdown()
}
