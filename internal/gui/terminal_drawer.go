package gui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	term "github.com/fyne-io/terminal"

	"github.com/mdelapenya/biomelab/internal/embeddedterminal"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

type cardTerminalKey struct{ repository, path, mode, sandbox, agent string }
type cardTerminalRemoval struct{ path string }

type cardTerminalSession struct {
	view    *term.Terminal
	cancel  context.CancelFunc
	done    chan struct{}
	stopped chan struct{} // closed after process handles have been released
	state   string
	running bool
	input   *terminalInput
	resize  *terminalResize
}

type cardTerminals struct {
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	transports                              sync.WaitGroup
	dispatch                                func(func(), bool)
	sessions                                map[cardTerminalKey]*cardTerminalSession
	removing                                map[string]*cardTerminalRemoval // UI-thread owned
	visible, expanded, autoExpanded, closed bool
	onAutoExpandedChanged                   func()
	key                                     cardTerminalKey
	title                                   *widget.Label
	slot                                    *fyne.Container
	content                                 fyne.CanvasObject
	offset                                  float64
	mainPanelHeightDelta                    float32
}

// terminalInput keeps slow PTY writes off the Fyne thread. The bounded queue
// accepts whole input events or reports an error, never a partial paste.
type terminalInput struct {
	ctx    context.Context
	queue  chan []byte
	report func(error)
	once   sync.Once
}

type terminalSize struct{ rows, cols uint16 }

// terminalResize keeps only the newest pending size. The Fyne callback never
// calls the native transport, whose resize operation can wait for pipe output.
type terminalResize struct{ pending chan terminalSize }

func newTerminalResize() *terminalResize {
	return &terminalResize{pending: make(chan terminalSize, 1)}
}
func (r *terminalResize) offer(rows, cols uint) {
	if rows == 0 || cols == 0 {
		return
	}
	size := terminalSize{uint16(min(rows, 65535)), uint16(min(cols, 65535))}
	select {
	case r.pending <- size:
		return
	default:
	}
	select {
	case <-r.pending:
	default:
	}
	select {
	case r.pending <- size:
	default:
	}
}
func (r *terminalResize) run(ctx context.Context, resize func(uint16, uint16) error) {
	for {
		var size terminalSize
		select {
		case <-ctx.Done():
			return
		case size = <-r.pending:
		}
	drain:
		for {
			select {
			case size = <-r.pending:
			default:
				break drain
			}
		}
		if ctx.Err() != nil {
			return
		}
		_ = resize(size.rows, size.cols)
	}
}

func (w *terminalInput) Write(b []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(b) > 1<<20 {
		err := fmt.Errorf("paste exceeds 1 MiB")
		if w.report != nil {
			w.report(err)
		}
		return 0, err
	}
	select {
	case w.queue <- append([]byte(nil), b...):
		return len(b), nil
	default:
		err := fmt.Errorf("terminal input is busy; try again")
		w.once.Do(func() {
			if w.report != nil {
				w.report(err)
			}
		})
		return 0, err
	}
}
func (*terminalInput) Close() error { return nil }

func canonicalTerminalPath(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path
}
func cardKey(re *repoEntry, wt git.Worktree) cardTerminalKey {
	target := targetForTerminal(re, wt.Path)
	root := wt.Path
	if re.group != nil {
		root = re.group.Path
	} else if re.repo != nil {
		root = re.repo.Root()
	}
	return cardTerminalKey{canonicalTerminalPath(root), canonicalTerminalPath(wt.Path), target.mode, target.sandbox, target.agent}
}
func embeddedCommand(key cardTerminalKey, wt git.Worktree) (string, []string) {
	if key.mode == "sandbox" && key.sandbox != "" {
		if wt.IsMain {
			return "", sandbox.RunAttachArgs(key.sandbox)
		}
		return "", sandbox.ExecAgentArgs(key.sandbox, wt.Path, key.agent)
	}
	return wt.Path, nil
}
func (a *App) selectedTerminalTarget() (*repoEntry, git.Worktree, bool) {
	re := a.activeRepo()
	if re == nil {
		return nil, git.Worktree{}, false
	}
	idx, ok := a.selectedWorktree()
	if !ok {
		return nil, git.Worktree{}, false
	}
	return re, re.state.Worktrees[idx], true
}
func (a *App) ensureCardTerminals() *cardTerminals {
	if a.cardTerminals != nil {
		return a.cardTerminals
	}
	p := &cardTerminals{sessions: make(map[cardTerminalKey]*cardTerminalSession), offset: 0.6}
	p.dispatch = func(fn func(), wait bool) {
		if wait {
			fyne.DoAndWait(fn)
		} else {
			fyne.Do(fn)
		}
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.onAutoExpandedChanged = func() {
		if a.cardTerminals == p && p.visible && !p.closed && a.dashboard != nil {
			a.dashboard.Rebuild()
		}
	}
	a.cardTerminals = p
	p.title = widget.NewLabel("Terminal")
	p.title.Truncation = fyne.TextTruncateEllipsis
	p.slot = container.NewStack()
	hide := newActionControl("Hide", theme.CancelIcon(), false, func() {
		a.leaveCardTerminal()
		p.visible = false
		a.dashboard.Rebuild()
	})
	expand := newActionControl("Expand / Restore", theme.ViewFullScreenIcon(), false, func() {
		if a.window != nil {
			a.window.Canvas().Unfocus()
		}
		p.expanded = !p.expanded
		a.dashboard.Rebuild()
		// Tapping the control unfocused the terminal; without this the next
		// keystrokes land on workspace shortcuts instead of the shell.
		if s := p.sessions[p.key]; s != nil && s.running && a.window != nil {
			a.window.Canvas().Focus(s.view)
		}
	})
	var more *actionControl
	more = newActionControl("", theme.MoreHorizontalIcon(), false, func() {
		menu := fyne.NewMenu("Terminal",
			fyne.NewMenuItem("Restart session", func() { a.confirmTerminalStop(true) }),
			fyne.NewMenuItem("Stop session", func() { a.confirmTerminalStop(false) }),
			fyne.NewMenuItem("Open in external terminal", func() {
				if re, wt, ok := a.selectedTerminalTarget(); ok {
					if p.removing[cardKey(re, wt).path] == nil {
						a.openOrActivateTerminal(re, wt)
					}
				}
			}),
		)
		widget.ShowPopUpMenuAtPosition(menu, a.window.Canvas(), fyne.CurrentApp().Driver().AbsolutePositionForObject(more).Add(fyne.NewPos(0, more.Size().Height)))
	})
	header := container.NewBorder(nil, nil, nil, container.NewHBox(expand, more, hide), p.title)
	hint := widget.NewLabel("Ctrl+Shift+Space returns to the workspace · Tab and Esc stay in the terminal")
	hint.Wrapping = fyne.TextWrapWord
	hint.TextStyle = fyne.TextStyle{Italic: true}
	p.content = inset(container.NewBorder(header, hint, nil, nil, p.slot), spaceSM, spaceXS)
	return p
}
func (a *App) leaveCardTerminal() {
	if a.window != nil {
		a.window.Canvas().Unfocus()
	}
	a.focus = focusRight
	a.updatePanelFocus()
}
func (a *App) wrapCardTerminal(body fyne.CanvasObject) fyne.CanvasObject {
	p := a.cardTerminals
	if p == nil || !p.visible || p.closed {
		return body
	}
	re, wt, ok := a.selectedTerminalTarget()
	if !ok {
		return body
	}
	key := cardKey(re, wt)
	if p.key != key {
		a.leaveCardTerminal()
		p.key = key
	}
	a.refreshCardTerminal()
	divider := &terminalDivider{}
	divider.ExtendBaseWidget(divider)
	root := container.New(&terminalDrawerLayout{panel: p}, body, divider, p.content)
	divider.drag = func(dy float32) {
		if h := root.Size().Height; h > 0 {
			p.offset += float64(dy / h)
			p.offset = max(0, min(1, p.offset))
			root.Refresh()
		}
	}
	return root
}
func (a *App) refreshCardTerminal() {
	p := a.cardTerminals
	if p == nil || p.closed {
		return
	}
	re, wt, ok := a.selectedTerminalTarget()
	if !ok {
		return
	}
	key := cardKey(re, wt)
	if key != p.key {
		return
	}
	label := wt.Branch
	if label == "" {
		label = filepath.Base(wt.Path)
	}
	repoName := filepath.Base(key.repository)
	if re.group != nil && re.group.Name != "" {
		repoName = re.group.Name
	}
	identity := repoName + " / " + label + " · " + key.mode
	if key.agent != "" {
		identity += " / " + key.agent
	}
	s := p.sessions[key]
	if s == nil {
		p.title.SetText("Terminal · " + identity)
		if p.removing[key.path] != nil {
			p.slot.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewLabel("Removing worktree…"))}
		} else {
			p.slot.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewButton("Open terminal", func() { a.openCardTerminal(re, wt) }))}
		}
	} else {
		p.title.SetText("Terminal · " + identity + " · " + s.state)
		p.slot.Objects = []fyne.CanvasObject{s.view}
	}
	p.slot.Refresh()
}
func (a *App) openCardTerminal(re *repoEntry, wt git.Worktree) {
	key := cardKey(re, wt)
	if p := a.cardTerminals; p != nil && p.removing[key.path] != nil {
		return
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		a.openOrActivateTerminal(re, wt)
		return
	}
	p := a.ensureCardTerminals()
	if p.closed {
		return
	}
	p.visible = true
	p.key = key
	if previous := p.sessions[key]; previous == nil || !previous.running {
		if previous != nil {
			previous.cancel()
		}
		a.startCardTerminal(key, wt, previous)
	}
	a.dashboard.Rebuild()
	a.window.Canvas().Focus(p.sessions[key].view)
}
func (a *App) startCardTerminal(key cardTerminalKey, wt git.Worktree, previous *cardTerminalSession) {
	p := a.cardTerminals
	ctx, cancel := context.WithCancel(p.ctx)
	s := &cardTerminalSession{view: term.New(), cancel: cancel, done: make(chan struct{}), stopped: make(chan struct{}), resize: newTerminalResize(), state: "Starting", running: true}
	p.sessions[key] = s
	s.view.Palette = func(index int) color.Color { return terminalANSIColor(index) }
	s.view.OnReturnToWorkspace = a.leaveCardTerminal
	s.input = &terminalInput{ctx: ctx, queue: make(chan []byte, 64), report: func(err error) {
		s.state = err.Error()
		a.refreshCardTerminal()
	}}
	s.view.AttachWriter(s.input)
	s.view.OnResize = func(rows, cols uint) {
		s.resize.offer(rows, cols)
	}
	current := func() bool { return !p.closed && p.ctx.Err() == nil && p.sessions[key] == s }
	dir, args := embeddedCommand(key, wt)
	p.transports.Add(1)
	go func() {
		defer close(s.done)
		defer cancel()
		var process *embeddedterminal.Process
		err := waitPreviousSession(ctx, previous, restartCleanupTimeout)
		if err == nil {
			process, err = embeddedterminal.Start(ctx, dir, args, 24, 80)
		}
		if err != nil {
			p.transports.Done()
			p.dispatch(func() {
				if current() {
					s.running = false
					s.state = "Failed: " + err.Error()
					s.view.Feed([]byte("Failed to start terminal: " + err.Error() + "\r\nUse the session menu to restart or open an external terminal."))
					a.refreshCardTerminal()
				}
			}, false)
			// This session never ran a process, but it replaced previous, so
			// its barrier must not open before previous's does: deletion and
			// the next restart only see this session.
			if previous == nil {
				close(s.stopped)
				return
			}
			go func() {
				<-previous.stoppedCh()
				close(s.stopped)
				if errors.Is(err, errPreviousSessionRunning) {
					p.dispatch(func() {
						if current() && !s.running {
							s.state = "Stopped: the previous session has ended; restart is available"
							a.refreshCardTerminal()
						}
					}, false)
				}
			}()
			return
		}
		resizeDone := make(chan struct{})
		go func() {
			defer close(resizeDone)
			s.resize.run(ctx, process.Resize)
		}()
		go func() {
			<-s.done
			process.WaitStopped()
			<-resizeDone
			close(s.stopped)
		}()
		go func() {
			_ = process.Wait()
			if ctx.Err() != nil {
				process.Stop()
			}
			// Quit waits on transports, so release this one only after the
			// whole owned process tree is gone. A shell that exited on its
			// own still needs its transport teardown (which ends lingering
			// descendants) before Biomelab may exit; shutdown bounds the wait.
			process.WaitStopped()
			p.transports.Done()
		}()
		defer process.Stop()
		p.dispatch(func() {
			if !current() {
				return
			}
			s.state = "Running"
			rows, cols := s.view.Dimensions()
			s.view.OnResize(rows, cols)
			a.refreshCardTerminal()
		}, true)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-s.input.queue:
					if _, err := process.Write(b); err != nil {
						return
					}
				}
			}
		}()
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := process.Read(buffer)
			if n > 0 {
				p.dispatch(func() {
					if current() {
						s.view.Feed(buffer[:n])
					}
				}, true)
			}
			if readErr != nil {
				if readErr != io.EOF {
					err = readErr
				}
				break
			}
		}
		process.Stop()
		if waitErr := process.Wait(); err == nil {
			err = waitErr
		}
		stopped := ctx.Err() != nil
		p.dispatch(func() {
			if current() {
				a.finishCardTerminalSession(key, s, stopped, err)
			}
		}, false)
	}()
}

// restartCleanupTimeout bounds how long a restart waits for the previous
// session's process tree. The backends keep WaitStopped honest (it opens only
// once the tree has ended, which worktree deletion relies on), so a tree that
// cannot be ended must fail the restart visibly instead of leaving it in
// "Starting" forever.
const restartCleanupTimeout = 10 * time.Second

var errPreviousSessionRunning = errors.New("the previous session is still shutting down; try again in a moment")

// stoppedCh is the barrier that opens once the session's process tree has
// ended: stopped when the session has one, otherwise done. Restart and
// worktree deletion both wait on it.
func (s *cardTerminalSession) stoppedCh() <-chan struct{} {
	if s.stopped != nil {
		return s.stopped
	}
	return s.done
}

// waitPreviousSession waits for previous (if any) to finish its teardown.
func waitPreviousSession(ctx context.Context, previous *cardTerminalSession, timeout time.Duration) error {
	if previous == nil {
		return nil
	}
	done := previous.stoppedCh()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return errPreviousSessionRunning
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) finishCardTerminalSession(key cardTerminalKey, s *cardTerminalSession, stopped bool, err error) {
	p := a.cardTerminals
	if p == nil || p.closed || p.sessions[key] != s {
		return
	}
	s.running = false
	s.state = "Exited"
	if stopped {
		s.state = "Stopped"
	} else if err != nil {
		s.state = "Exited: " + err.Error()
	}
	if !stopped && err == nil && p.visible && p.key == key {
		if re, wt, ok := a.selectedTerminalTarget(); ok && cardKey(re, wt) == key {
			a.leaveCardTerminal()
			p.visible = false
			a.dashboard.Rebuild()
			return
		}
	}
	a.refreshCardTerminal()
}
func (a *App) confirmTerminalStop(restart bool) {
	p := a.cardTerminals
	if p == nil || a.dialogOpen {
		return
	}
	key := p.key
	if p.removing[key.path] != nil {
		return
	}
	s := p.sessions[key]
	if s == nil || (!restart && !s.running) {
		return
	}
	re, wt, ok := a.selectedTerminalTarget()
	if !ok || cardKey(re, wt) != key {
		return
	}
	action := func() {
		if p.removing[key.path] != nil || p.sessions[key] != s {
			return
		}
		s.cancel()
		s.state = "Stopping"
		if restart {
			a.startCardTerminal(key, wt, s)
		}
		a.refreshCardTerminal()
		if restart {
			a.window.Canvas().Focus(p.sessions[key].view)
		}
	}
	if !s.running {
		action()
		return
	}
	wasFocused := a.window.Canvas().Focused() == s.view
	done := a.openDialog()
	var d dialog.Dialog
	cancel := newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	label, title, body := "Stop session", "Stop terminal session?", "This stops the shell or agent running in this card's terminal."
	if restart {
		label, title, body = "Restart session", "Restart terminal session?", "This stops the shell or agent running in this card's terminal and starts a new one."
	}
	confirm := newDialogButton(label, func() { d.Hide(); action() }, func() { d.Hide() })
	confirm.Importance = widget.HighImportance
	d = dialog.NewCustomWithoutButtons(title, container.NewVBox(
		dialogText(body),
		dialogFooter(cancel, confirm)), a.window)
	d.SetOnClosed(func() {
		done()
		if wasFocused && !a.dialogOpen && p.visible && !p.closed && p.key == key && p.sessions[key] == s {
			a.window.Canvas().Focus(s.view)
		}
	})
	a.activeDialog = d
	d.Show()
	a.window.Canvas().Focus(cancel)
}
func (a *App) beginCardTerminalRemoval(path string) *cardTerminalRemoval {
	p := a.ensureCardTerminals()
	if p.removing == nil {
		p.removing = make(map[string]*cardTerminalRemoval)
	}
	if p.removing[path] != nil {
		return nil
	}
	removal := &cardTerminalRemoval{path: path}
	p.removing[path] = removal
	return removal
}
func (a *App) finishCardTerminalRemoval(path string, removal *cardTerminalRemoval) {
	if p := a.cardTerminals; p != nil {
		if p.removing[path] == removal && removal != nil {
			delete(p.removing, path)
			a.refreshCardTerminal()
		}
	}
}
func (a *App) releaseCardTerminalRemovalAfterCleanup(path string, removal *cardTerminalRemoval, stopped []<-chan struct{}) <-chan struct{} {
	p := a.cardTerminals
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, done := range stopped {
			select {
			case <-done:
			case <-p.ctx.Done():
				return
			}
		}
		if p.ctx.Err() == nil {
			p.dispatch(func() { a.finishCardTerminalRemoval(path, removal) }, false)
		}
	}()
	return done
}
func (a *App) stopCardTerminals(match func(cardTerminalKey) bool) []<-chan struct{} {
	p := a.cardTerminals
	if p == nil {
		return nil
	}
	if p.visible && match(p.key) {
		a.leaveCardTerminal()
		p.visible = false
	}
	var stopped []<-chan struct{}
	for key, s := range p.sessions {
		if match(key) {
			s.cancel()
			stopped = append(stopped, s.stoppedCh())
			delete(p.sessions, key)
		}
	}
	a.refreshCardTerminal()
	return stopped
}
func (a *App) closeCardTerminals() {
	if p := a.cardTerminals; p != nil && !p.closed {
		p.closed = true
		p.cancel()
	}
}

// Called after the native event loop returns. Wait only for transports, never
// renderer workers that may be waiting for an event loop which has stopped.
func (p *cardTerminals) shutdown() {
	p.cancel()
	done := make(chan struct{})
	go func() { p.transports.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(terminalShutdownTimeout):
	}
}

// terminalShutdownTimeout bounds how long Quit waits for owned process trees.
// A normal teardown finishes well within it. It exceeds the Unix backend's
// five-second process-group wait; on Windows teardown has no fixed bound (it
// waits for the job to empty, after its five-second member budget), so a
// tree that cannot be ended is abandoned here rather than keeping Biomelab
// open. The job's kill-on-close limit still ends it when Biomelab exits.
const terminalShutdownTimeout = 6 * time.Second

// Collapse only the browser region when the window cannot fit two useful panes.
// Keep the preference independently of the clamped geometry, like the sidebar.
type terminalDrawerLayout struct{ panel *cardTerminals }

func (*terminalDrawerLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, scaledSize(200))
}
func (l *terminalDrawerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	body, divider, drawer := objects[0], objects[1], objects[2]
	// Measure against the body height with the normal Main panel. Swapping it
	// for a shorter owner summary must not change the expansion decision.
	autoExpanded := size.Height+l.panel.mainPanelHeightDelta < scaledSize(440)
	if l.panel.autoExpanded != autoExpanded {
		l.panel.autoExpanded = autoExpanded
		if l.panel.onAutoExpandedChanged != nil {
			fyne.Do(l.panel.onAutoExpandedChanged)
		}
	}
	if l.panel.expanded || autoExpanded {
		body.Hide()
		divider.Hide()
		drawer.Move(fyne.Position{})
		drawer.Resize(size)
		return
	}
	body.Show()
	divider.Show()
	y := max(scaledSize(200), min(size.Height-scaledSize(220), size.Height*float32(l.panel.offset)))
	body.Move(fyne.Position{})
	body.Resize(fyne.NewSize(size.Width, y-scaledSize(4)))
	divider.Move(fyne.NewPos(0, y-scaledSize(4)))
	divider.Resize(fyne.NewSize(size.Width, scaledSize(8)))
	drawer.Move(fyne.NewPos(0, y+scaledSize(4)))
	drawer.Resize(fyne.NewSize(size.Width, max(0, size.Height-y-scaledSize(4))))
}

type terminalDivider struct {
	widget.BaseWidget
	drag func(float32)
}

func (d *terminalDivider) Dragged(e *fyne.DragEvent) { d.drag(e.Dragged.DY) }
func (*terminalDivider) DragEnd()                    {}
func (*terminalDivider) Cursor() desktop.Cursor      { return desktop.VResizeCursor }
func (*terminalDivider) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(&terminalDividerLayout{}, canvas.NewRectangle(colorBorder)))
}

type terminalDividerLayout struct{}

func (*terminalDividerLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, scaledSize(8))
}
func (*terminalDividerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.NewPos(0, (size.Height-1)/2))
	objects[0].Resize(fyne.NewSize(size.Width, 1))
}

// Indexed ANSI colors follow the app palette even for retained scrollback.
// Explicit application RGB colors are preserved by the emulator.
type terminalANSIColor int

func (c terminalANSIColor) RGBA() (uint32, uint32, uint32, uint32) {
	dark := []uint32{0x202228, 0xef7d7d, 0x89c991, 0xd8b66c, 0x8baff1, 0xc59bea, 0x79c6ce, 0xd7d9de, 0x9298a3, 0xff9696, 0xa4dda9, 0xf0d18a, 0xaac5ff, 0xdfb8ff, 0xa0e4e9, 0xffffff}
	light := []uint32{0x24262c, 0xb3261e, 0x137333, 0x806300, 0x124da8, 0x872487, 0x086970, 0x555963, 0x636775, 0xaa241b, 0x0a6b29, 0x765900, 0x0747a0, 0x772078, 0x00616b, 0x32343b}
	palette := dark
	if t, ok := fyne.CurrentApp().Settings().Theme().(*biomeTheme); ok && t.Variant() == VariantLight {
		palette = light
	}
	value := palette[int(c)%len(palette)]
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 255}.RGBA()
}
