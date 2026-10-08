package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	term "github.com/fyne-io/terminal"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func seedCardTerminal(a *App, re *repoEntry, wt git.Worktree) *cardTerminalSession {
	p := a.ensureCardTerminals()
	done := make(chan struct{})
	close(done)
	s := &cardTerminalSession{view: term.New(), state: "Running", running: true, cancel: func() {}, done: done, stopped: done}
	s.view.Palette = func(index int) color.Color { return terminalANSIColor(index) }
	s.view.OnReturnToWorkspace = a.leaveCardTerminal
	p.sessions[cardKey(re, wt)] = s
	return s
}
func terminalFixture(t *testing.T) (*App, *repoEntry, fyne.Window) {
	t.Helper()
	fa := test.NewApp()
	t.Cleanup(fa.Quit)
	fa.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	state := workspaceState()
	state.Worktrees[0].Path = t.TempDir()
	state.Worktrees[1].Path = t.TempDir()
	state.Worktrees[2].Path = t.TempDir()
	d := NewDashboard(state)
	w := polishWindow(t, fa, d, 1200, 820)
	re := &repoEntry{state: state, dashboard: d, group: &RepoGroup{Path: state.Worktrees[0].Path}}
	a := &App{fyneApp: fa, window: w, dashboard: d, repos: []*repoEntry{re}}
	a.wireDashboardActions(d)
	return a, re, w
}
func TestCardTerminalSelectionRetainsSessionsAndFocus(t *testing.T) {
	a, re, w := terminalFixture(t)
	wt := re.state.Worktrees[2]
	original := seedCardTerminal(a, re, wt)
	a.handleEnter()
	if w.Canvas().Focused() != original.view || !a.cardTerminals.visible {
		t.Fatal("Enter did not focus embedded terminal")
	}
	for _, view := range []ViewMode{ViewGrid, ViewList, ViewKanban} {
		re.state.ViewMode = view
		a.dashboard.Rebuild()
		if a.cardTerminals.sessions[cardKey(re, wt)] != original {
			t.Fatal("view change recreated terminal")
		}
	}
	a.dashboard.selectCard(1)
	if len(a.cardTerminals.sessions) != 1 || w.Canvas().Focused() != nil {
		t.Fatal("selection started a shell or retained terminal focus")
	}
	a.dashboard.selectCard(2)
	a.handleEnter()
	if a.cardTerminals.sessions[cardKey(re, wt)] != original {
		t.Fatal("reopening replaced session")
	}
	capture := &terminalCapture{}
	original.view.AttachWriter(capture)
	keys := &keyboardCanvas{Canvas: w.Canvas()}
	keys.press(fyne.KeyTab)
	keys.press(fyne.KeyEscape)
	keys.modifiers = fyne.KeyModifierControl
	keys.press(fyne.KeyC)
	if capture.String() != "\t\x1b\x03" {
		t.Fatalf("terminal input intercepted: %q", capture.String())
	}
	keys.modifiers = fyne.KeyModifierControl | fyne.KeyModifierShift
	keys.press(fyne.KeySpace)
	if w.Canvas().Focused() != nil {
		t.Fatal("workspace chord did not release terminal focus")
	}
}

func TestCardTerminalCleanExitCollapsesOnlyShownSession(t *testing.T) {
	for _, tc := range []struct {
		name      string
		other     bool
		hidden    bool
		stopped   bool
		err       error
		collapsed bool
	}{
		{name: "shown clean exit", collapsed: true},
		{name: "other card clean exit", other: true},
		{name: "hidden clean exit", hidden: true},
		{name: "abnormal exit", err: fmt.Errorf("shell failed")},
		{name: "explicit stop", stopped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, re, w := terminalFixture(t)
			shown := re.state.Worktrees[2]
			shownSession := seedCardTerminal(a, re, shown)
			a.handleEnter()
			target := shown
			s := shownSession
			if tc.other {
				target = re.state.Worktrees[1]
				s = seedCardTerminal(a, re, target)
			}
			if tc.hidden {
				a.leaveCardTerminal()
				a.cardTerminals.visible = false
			}
			a.finishCardTerminalSession(cardKey(re, target), s, tc.stopped, tc.err)
			if !tc.hidden && a.cardTerminals.visible != !tc.collapsed {
				t.Fatalf("drawer visibility after exit = %v", a.cardTerminals.visible)
			}
			if tc.collapsed && w.Canvas().Focused() != nil {
				t.Fatal("clean exit retained terminal focus")
			}
			if tc.other && w.Canvas().Focused() != shownSession.view {
				t.Fatal("background exit disturbed active terminal focus")
			}
			if tc.hidden && a.cardTerminals.visible {
				t.Fatal("background exit reopened hidden drawer")
			}
		})
	}
}

// Use the actual custom-shortcut type below; no canvas interception is needed.
func TestCardTerminalKeysAndSandboxCommands(t *testing.T) {
	wt := git.Worktree{Path: filepath.Join(t.TempDir(), "a ' worktree"), Branch: "same"}
	re := &repoEntry{group: &RepoGroup{Path: t.TempDir()}, state: &RepoState{}}
	host := cardKey(re, wt)
	re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", SandboxName: "box", Agent: "codex"}
	sbx := cardKey(re, wt)
	if host == sbx {
		t.Fatal("mode keys collided")
	}
	dir, args := embeddedCommand(sbx, wt)
	if dir != "" || len(args) < 6 || args[0] != "sbx" || args[1] != "exec" || args[4] != sandbox.ContainerPath(wt.Path) {
		t.Fatalf("wrong linked launch: %q %q", dir, args)
	}
	wt.IsMain = true
	_, args = embeddedCommand(sbx, wt)
	if strings.Join(args, " ") != "sbx run --name box" {
		t.Fatalf("wrong main launch: %q", args)
	}
	re.state.ActiveMode.Agent = "claude"
	if cardKey(re, wt) == sbx {
		t.Fatal("agent keys collided")
	}
}
func drainTerminalEvents(t *testing.T, events <-chan func(), ready func() bool) {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-tick.C:
		case fn := <-events:
			fn()
		case <-timeout.C:
			t.Fatal("terminal operation timed out")
		}
	}
}
func TestCardTerminalRealShellOutputReuseAndStop(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native PTY")
	}
	t.Setenv("SHELL", "/bin/sh")
	a, re, _ := terminalFixture(t)
	p := a.ensureCardTerminals()
	events := make(chan func(), 32)
	p.dispatch = func(fn func(), wait bool) {
		done := make(chan struct{})
		events <- func() { fn(); close(done) }
		if wait {
			<-done
		}
	}
	a.handleEnter()
	key := cardKey(re, re.state.Worktrees[2])
	s := p.sessions[key]
	t.Cleanup(func() {
		a.closeCardTerminals()
		drainTerminalEvents(t, events, func() bool {
			select {
			case <-s.done:
				return true
			default:
				return false
			}
		})
	})
	drainTerminalEvents(t, events, func() bool { return s.state == "Running" })
	a.handleEnter()
	if p.sessions[key] != s {
		t.Fatal("Enter duplicated live shell")
	}
	_, err := s.input.Write([]byte("printf 'integrated-terminal-ok\\n'\n"))
	if err != nil {
		t.Fatal(err)
	}
	drainTerminalEvents(t, events, func() bool { return strings.Contains(s.view.Text(), "integrated-terminal-ok") })
	_, err = s.input.Write([]byte("exit\n"))
	if err != nil {
		t.Fatal(err)
	}
	drainTerminalEvents(t, events, func() bool { return !s.running })
	if s.state != "Exited" || p.visible {
		t.Fatalf("normal exit = %q, drawer visible = %v", s.state, p.visible)
	}
	// Reopening the collapsed drawer must replace a dead session.
	a.handleEnter()
	previous := s
	s = p.sessions[key]
	if s == previous || !p.visible {
		t.Fatal("reopening reused an exited shell")
	}
	drainTerminalEvents(t, events, func() bool { return s.state == "Running" })
	if _, err := s.input.Write([]byte("printf '%s%s\\n' fresh- shell-ok\n")); err != nil {
		t.Fatal(err)
	}
	drainTerminalEvents(t, events, func() bool { return strings.Contains(s.view.Text(), "fresh-shell-ok") })
}
func TestTerminalInputAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input := &terminalInput{ctx: ctx, queue: make(chan []byte, 1)}
	if _, err := input.Write([]byte("x")); err == nil {
		t.Fatal("input accepted after stop")
	}
}

func TestTerminalResizeDoesNotBlockUIAndKeepsLatestSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newTerminalResize()
	view := term.New()
	view.OnResize = r.offer
	called := make(chan terminalSize, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.run(ctx, func(rows, cols uint16) error {
			called <- terminalSize{rows, cols}
			if rows == 24 {
				<-release
			}
			return nil
		})
	}()
	view.OnResize(24, 80)
	select {
	case size := <-called:
		if size != (terminalSize{24, 80}) {
			t.Fatalf("first resize = %+v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("first resize did not start")
	}
	returned := make(chan struct{})
	go func() {
		view.OnResize(30, 100)
		view.OnResize(40, 120)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("UI resize callback waited for blocked native resize")
	}
	close(release)
	select {
	case size := <-called:
		if size != (terminalSize{40, 120}) {
			t.Fatalf("pending resize = %+v, want latest size", size)
		}
	case <-time.After(time.Second):
		t.Fatal("latest resize was not applied")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resize worker did not stop after cancellation")
	}
}

func TestTerminalResizeCancellationDiscardsPendingSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := newTerminalResize()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	calls := make(chan terminalSize, 2)
	go func() {
		defer close(done)
		r.run(ctx, func(rows, cols uint16) error {
			calls <- terminalSize{rows, cols}
			close(entered)
			<-release
			return nil
		})
	}()
	r.offer(24, 80)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("blocking resize did not start")
	}
	r.offer(40, 120)
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resize worker did not exit after cancellation")
	}
	if len(calls) != 1 {
		t.Fatalf("resize continued after cancellation: %d calls", len(calls))
	}
}
func TestCardTerminalRenderedDrawer(t *testing.T) {
	a, re, w := terminalFixture(t)
	re.group.Name = "biomelab"
	a.dashboard.RepoName = "biomelab"
	a.repoPanel = NewRepoPanel([]*RepoGroup{re.group}, nil)
	w.SetContent(newShellLayout(a.repoPanel.Content(), a.dashboard.Content(), nil))
	session := seedCardTerminal(a, re, re.state.Worktrees[2])
	a.openCardTerminal(re, re.state.Worktrees[2])
	session.view.Feed([]byte("\x1b[32mBiomelab\x1b[0m · integrated terminal\r\n$ pwd\r\n/workspace/feature\r\n$ go test ./...\r\nok   example/internal/workspace\r\n$ "))
	for _, tc := range []struct {
		name          string
		variant       ThemeVariant
		width, height float32
		view          ViewMode
		expand        bool
	}{
		{"dark", VariantDark, 1200, 820, ViewKanban, false},
		{"light", VariantLight, 1200, 820, ViewList, false},
		{"narrow", VariantDark, 780, 700, ViewGrid, false},
		{"expanded", VariantDark, 1000, 760, ViewKanban, true},
	} {
		a.fyneApp.Settings().SetTheme(NewTheme(tc.variant))
		a.repoPanel.RebuildFull()
		a.cardTerminals.expanded = tc.expand
		re.state.ViewMode = tc.view
		a.dashboard.Rebuild()
		w.Resize(fyne.NewSize(tc.width, tc.height))
		if dir := os.Getenv("BIOMELAB_TERMINAL_SCREENSHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, "terminal-"+tc.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, w.Canvas().Capture())
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		if a.cardTerminals.slot.Size().Height < 100 {
			t.Fatalf("%s drawer too small: %v", tc.name, a.cardTerminals.slot.Size())
		}
	}
}

func TestTerminalShutdownDoesNotDependOnRenderer(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native PTY")
	}
	t.Setenv("SHELL", "/bin/sh")
	a, _, _ := terminalFixture(t)
	p := a.ensureCardTerminals()
	events := make(chan func(), 16)
	p.dispatch = func(fn func(), wait bool) {
		done := make(chan struct{})
		events <- func() { fn(); close(done) }
		if wait {
			<-done
		}
	}
	a.handleEnter()
	s := p.sessions[p.key]
	// Leave the ready callback queued, as if the native loop had just stopped.
	var ready func()
	select {
	case ready = <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("startup timed out")
	}
	started := time.Now()
	p.shutdown()
	if time.Since(started) > time.Second {
		t.Fatal("shutdown waited for renderer callback")
	}
	ready()
	drainTerminalEvents(t, events, func() bool {
		select {
		case <-s.done:
			return true
		default:
			return false
		}
	})
}

type terminalCapture struct{ bytes.Buffer }

func (*terminalCapture) Close() error { return nil }

func TestTerminalPasteLimitIsVisibleAndAtomic(t *testing.T) {
	reported := false
	input := &terminalInput{ctx: context.Background(), queue: make(chan []byte, 1), report: func(error) { reported = true }}
	n, err := input.Write(make([]byte, (1<<20)+1))
	if n != 0 || err == nil || !reported || len(input.queue) != 0 {
		t.Fatal("oversized paste was partial or silently dropped")
	}
}

func TestTerminalStopEscapeRestoresInputWithoutStopping(t *testing.T) {
	a, re, w := terminalFixture(t)
	s := seedCardTerminal(a, re, re.state.Worktrees[2])
	s.running = true
	stopped := false
	s.cancel = func() { stopped = true }
	a.handleEnter()
	a.confirmTerminalStop(false)
	if !a.dialogOpen {
		t.Fatal("missing stop confirmation")
	}
	keys := &keyboardCanvas{Canvas: w.Canvas()}
	keys.press(fyne.KeyEscape)
	if stopped || a.dialogOpen || w.Canvas().Focused() != s.view {
		t.Fatal("Escape stopped session or lost terminal focus")
	}
}

// A restart waits for the previous session's teardown, but a tree that
// cannot be ended fails the restart instead of leaving it starting forever.
func TestWaitPreviousSessionIsBounded(t *testing.T) {
	ctx := context.Background()
	if err := waitPreviousSession(ctx, nil, time.Second); err != nil {
		t.Fatalf("no previous session: %v", err)
	}

	stopped := make(chan struct{})
	prev := &cardTerminalSession{done: make(chan struct{}), stopped: stopped}
	close(prev.done) // the session goroutine ended, but teardown has not
	if err := waitPreviousSession(ctx, prev, 50*time.Millisecond); !errors.Is(err, errPreviousSessionRunning) {
		t.Fatalf("stuck teardown: got %v, want errPreviousSessionRunning", err)
	}

	close(stopped)
	if err := waitPreviousSession(ctx, prev, time.Second); err != nil {
		t.Fatalf("finished teardown: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	stuck := &cardTerminalSession{done: make(chan struct{}), stopped: make(chan struct{})}
	if err := waitPreviousSession(canceled, stuck, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait: got %v", err)
	}
}

// A restart that fails before starting a process must not open its barrier
// while the session it replaced is still tearing down: worktree deletion and
// the next restart only see the newer session.
func TestFailedRestartKeepsThePreviousBarrier(t *testing.T) {
	a, re, _ := terminalFixture(t)
	wt := re.state.Worktrees[2]
	p := a.ensureCardTerminals()
	p.dispatch = func(fn func(), wait bool) { fyne.DoAndWait(fn) }
	t.Cleanup(p.shutdown)

	previous := &cardTerminalSession{done: make(chan struct{}), stopped: make(chan struct{})}
	close(previous.done) // its goroutine ended, but its process tree has not
	key := cardKey(re, wt)
	fyne.DoAndWait(func() { a.startCardTerminal(key, wt, previous) })
	var s *cardTerminalSession
	fyne.DoAndWait(func() { s = p.sessions[key] })
	s.cancel() // e.g. the worktree is being deleted while the restart waits

	select {
	case <-s.stopped:
		t.Fatal("failed restart opened its barrier before the previous session's")
	case <-time.After(300 * time.Millisecond):
	}
	close(previous.stopped)
	select {
	case <-s.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("barrier did not open after the previous session's")
	}
}
