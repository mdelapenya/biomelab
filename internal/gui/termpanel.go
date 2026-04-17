package gui

import (
	"os"
	"os/exec"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/creack/pty"
	"github.com/fyne-io/terminal"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// session wraps a live terminal widget with a force-close hook. Sandbox
// sessions kill the sbx child process directly; plain shell sessions fall
// back to writing Ctrl-D (the only input path fyne-io/terminal exposes).
// view is what gets mounted into the panel slot — a scrollForwarder that
// wraps the terminal so mouse wheel events translate to cursor keys.
type session struct {
	term    *terminal.Terminal
	view    fyne.CanvasObject
	cleanup func()
}

// scrollForwarder wraps a terminal widget and implements fyne.Scrollable so
// mouse wheel events translate into cursor-key escape sequences written
// straight into the PTY. fyne-io/terminal does not implement Scrollable,
// so wheel events otherwise have no effect — breaking the UX of pagers
// (less, man), REPLs, tmux, vim, and shell history scrolling.
type scrollForwarder struct {
	widget.BaseWidget
	term *terminal.Terminal
}

func newScrollForwarder(t *terminal.Terminal) *scrollForwarder {
	s := &scrollForwarder{term: t}
	s.ExtendBaseWidget(s)
	return s
}

// CreateRenderer renders the wrapped terminal as our sole visual. Keyboard
// and mouse-click events still route directly to the terminal (innermost
// widget under the cursor), so we only intercept scroll.
func (s *scrollForwarder) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.term)
}

// Scrolled translates vertical wheel deltas into repeated up/down arrow
// presses. This matches the convention readline, less, and vim use when
// the terminal emulator is not in mouse-tracking mode.
func (s *scrollForwarder) Scrolled(ev *fyne.ScrollEvent) {
	dy := ev.Scrolled.DY
	if dy == 0 {
		return
	}
	// One "notch" is roughly 10–20 px on most platforms. Round up so small
	// nudges still produce at least one line.
	lines := int(dy / 12)
	if lines == 0 {
		if dy > 0 {
			lines = 1
		} else {
			lines = -1
		}
	}
	seq := []byte{0x1b, '[', 'A'} // up
	if lines < 0 {
		seq = []byte{0x1b, '[', 'B'} // down
		lines = -lines
	}
	for i := 0; i < lines; i++ {
		_, _ = s.term.Write(seq)
	}
}

// TermPanel holds embedded terminal sessions keyed by an opaque string.
// Regular-mode sessions (plain shell at a worktree) and sandbox-mode
// sessions (sbx run ... ) live in the same cache with distinct key prefixes
// so the same worktree in different modes does not collide.
type TermPanel struct {
	mu       sync.Mutex
	slot     *fyne.Container
	sessions map[string]*session
	active   string
	empty    fyne.CanvasObject
}

// NewTermPanel creates an empty terminal panel with a placeholder.
func NewTermPanel() *TermPanel {
	placeholder := monoText("(press Enter on a worktree to open a terminal)", colorDimGray, false)
	p := &TermPanel{
		sessions: make(map[string]*session),
		empty:    container.NewCenter(placeholder),
	}
	p.slot = container.NewStack(p.empty)
	return p
}

// Content returns the renderable panel container.
func (p *TermPanel) Content() fyne.CanvasObject { return p.slot }

// Active returns the currently visible terminal, or nil when the placeholder
// is shown.
func (p *TermPanel) Active() *terminal.Terminal {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active == "" {
		return nil
	}
	if s := p.sessions[p.active]; s != nil {
		return s.term
	}
	return nil
}

// ShowShell shows (creating on first use) a plain $SHELL session at path.
func (p *TermPanel) ShowShell(path string) {
	if path == "" {
		return
	}
	key := shellKey(path)
	if p.showExisting(key) {
		return
	}

	t := terminal.New()
	t.SetStartDir(path)
	p.register(key, newSession(t, func() { t.Exit() }))

	go func() {
		_ = t.RunLocalShell()
		p.onSessionEnd(key)
	}()
}

// ShowSandbox shows (creating on first use) a session attached to a linked
// sandbox worktree via `sbx run --branch <branch>`.
func (p *TermPanel) ShowSandbox(sbxName, branch string) {
	if sbxName == "" || branch == "" {
		return
	}
	p.startSandbox(sandboxKey(sbxName, branch), sandbox.RunWithBranchArgs(sbxName, branch))
}

// ShowSandboxMain shows (creating on first use) a session attached to the
// main sandbox worktree via `sbx run` (no --branch).
func (p *TermPanel) ShowSandboxMain(sbxName string) {
	if sbxName == "" {
		return
	}
	p.startSandbox(sandboxMainKey(sbxName), sandbox.RunArgs(sbxName))
}

// RemoveShell terminates and drops the shell session for path.
func (p *TermPanel) RemoveShell(path string) { p.remove(shellKey(path)) }

// RemoveSandbox terminates and drops the sandbox session for sbxName+branch.
func (p *TermPanel) RemoveSandbox(sbxName, branch string) {
	p.remove(sandboxKey(sbxName, branch))
}

// RemoveSandboxMain terminates and drops the main sandbox session.
func (p *TermPanel) RemoveSandboxMain(sbxName string) {
	p.remove(sandboxMainKey(sbxName))
}

// CloseAll terminates every cached session. Called both from the tray Quit
// handler and from the Fyne Lifecycle OnStopped hook so other exit paths
// (Cmd+Q, dock Quit) still clean up their PTY children.
func (p *TermPanel) CloseAll() {
	p.mu.Lock()
	cleanups := make([]func(), 0, len(p.sessions))
	for _, s := range p.sessions {
		if s.cleanup != nil {
			cleanups = append(cleanups, s.cleanup)
		}
	}
	p.sessions = make(map[string]*session)
	p.active = ""
	p.mu.Unlock()

	for _, c := range cleanups {
		c()
	}
}

func newSession(t *terminal.Terminal, cleanup func()) *session {
	return &session{
		term:    t,
		view:    newScrollForwarder(t),
		cleanup: cleanup,
	}
}

func (p *TermPanel) showExisting(key string) bool {
	p.mu.Lock()
	s, ok := p.sessions[key]
	if ok {
		p.active = key
	}
	p.mu.Unlock()
	if !ok {
		return false
	}
	p.slot.Objects = []fyne.CanvasObject{s.view}
	p.slot.Refresh()
	return true
}

func (p *TermPanel) register(key string, s *session) {
	p.mu.Lock()
	p.sessions[key] = s
	p.active = key
	p.mu.Unlock()

	p.slot.Objects = []fyne.CanvasObject{s.view}
	p.slot.Refresh()
}

// startSandbox spawns the given sbx command under a PTY and wires it to a
// new terminal widget. Resizes originating from the widget are forwarded to
// the PTY via the Config listener (fyne-io/terminal's built-in resize only
// fires for RunLocalShell).
func (p *TermPanel) startSandbox(key string, args []string) {
	if p.showExisting(key) {
		return
	}
	if len(args) == 0 {
		return
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		fyne.LogError("sandbox pty start", err)
		return
	}

	t := terminal.New()
	p.register(key, newSession(t, func() { _ = cmd.Process.Kill() }))

	cfgCh := make(chan terminal.Config, 8)
	t.AddListener(cfgCh)
	go func() {
		var lastRows, lastCols uint
		for cfg := range cfgCh {
			// Skip no-op notifications (PWD change, repeated same size)
			// so rapid VSplit drags don't pile up ioctl calls and SIGWINCH
			// floods into the sandbox child.
			if cfg.Rows == lastRows && cfg.Columns == lastCols {
				continue
			}
			lastRows, lastCols = cfg.Rows, cfg.Columns
			_ = pty.Setsize(ptyFile, &pty.Winsize{
				Rows: uint16(cfg.Rows),
				Cols: uint16(cfg.Columns),
			})
		}
	}()

	go func() {
		_ = t.RunWithConnection(ptyFile, ptyFile)
		_ = ptyFile.Close()
		_ = cmd.Process.Kill() // no-op if already dead
		t.RemoveListener(cfgCh)
		p.onSessionEnd(key)
	}()
}

// onSessionEnd is invoked when a session's shell exits on its own (e.g.
// user typed `exit`). Drops the cache entry so the next click on that card
// starts a fresh session, and reverts the slot to the placeholder if the
// dead session was visible.
func (p *TermPanel) onSessionEnd(key string) {
	p.mu.Lock()
	_, ok := p.sessions[key]
	if ok {
		delete(p.sessions, key)
	}
	clearVisible := p.active == key
	if clearVisible {
		p.active = ""
	}
	p.mu.Unlock()

	if !ok || !clearVisible {
		return
	}
	fyne.Do(func() {
		p.slot.Objects = []fyne.CanvasObject{p.empty}
		p.slot.Refresh()
	})
}

func (p *TermPanel) remove(key string) {
	p.mu.Lock()
	s, ok := p.sessions[key]
	if ok {
		delete(p.sessions, key)
	}
	clearVisible := p.active == key
	if clearVisible {
		p.active = ""
	}
	p.mu.Unlock()

	if ok && s.cleanup != nil {
		s.cleanup()
	}
	if clearVisible {
		p.slot.Objects = []fyne.CanvasObject{p.empty}
		p.slot.Refresh()
	}
}

func shellKey(path string) string           { return "shell:" + path }
func sandboxKey(name, branch string) string { return "sbx:" + name + ":" + branch }
func sandboxMainKey(name string) string     { return "sbx-main:" + name }
