//go:build darwin || linux

// Package embeddedterminal owns processes attached to the in-app terminal.
package embeddedterminal

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Process owns a PTY and its process group. Stop is idempotent and never waits
// for a renderer; Wait returns after the child has been reaped.
type Process struct {
	stopped chan struct{}
	file    *os.File
	fdMu    sync.Mutex // serializes Setsize/Fd with Close
	closed  bool
	cmd     *exec.Cmd
	done    chan struct{}
	stop    sync.Once
	err     error
	cancel  context.CancelFunc
}

// Start starts argv directly, preserving argument boundaries. An empty argv
// starts the user's interactive shell in dir.
func Start(ctx context.Context, dir string, argv []string, rows, cols uint16) (*Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		argv = []string{shell}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: max(rows, 1), Cols: max(cols, 1)})
	if err != nil {
		return nil, err
	}
	watch, cancel := context.WithCancel(ctx)
	p := &Process{stopped: make(chan struct{}), file: f, cmd: cmd, done: make(chan struct{}), cancel: cancel}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	go func() {
		select {
		case <-watch.Done():
			p.Stop()
		case <-p.done:
		}
	}()
	return p, nil
}

func (p *Process) Read(b []byte) (int, error) {
	n, err := p.file.Read(b)
	if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
		err = io.EOF
	}
	return n, err
}

func (p *Process) Write(b []byte) (int, error) { return p.file.Write(b) }
func (p *Process) Resize(rows, cols uint16) error {
	p.fdMu.Lock()
	defer p.fdMu.Unlock()
	if p.closed {
		return io.ErrClosedPipe
	}
	return pty.Setsize(p.file, &pty.Winsize{Rows: max(rows, 1), Cols: max(cols, 1)})
}

// Stop tears down the owned group, including foreground children that ignore
// EOF. Escalation is bounded and happens off the UI thread.
func (p *Process) Stop() {
	p.stop.Do(func() {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
		p.cancel()
		go func() {
			defer close(p.stopped)
			p.fdMu.Lock()
			p.closed = true
			_ = p.file.Close()
			p.fdMu.Unlock()
			timer := time.NewTimer(250 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
			// A descendant can survive the direct child's exit. Always signal
			// the group; ESRCH is the normal already-gone result.
			pgid := p.cmd.Process.Pid
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			// WaitStopped is the restart and worktree-deletion barrier, so it
			// must not open while any group member can still hold the worktree
			// as its cwd. Reap the leader first (an unreaped zombie still
			// counts as a member), then wait for the group to disappear.
			// Callers bound this wait themselves.
			<-p.done
			waitGroupGone(pgid)
		}()
	})
}

// waitGroupGone polls until no process we can signal remains in group pgid.
// Signal 0 probes for existence without delivering anything. ESRCH means the
// group is gone; EPERM can only mean the id was reused by a group we never
// owned, so it ends the wait too. Descendants orphaned by the SIGKILL are
// reparented and reaped by init or launchd, which is why polling is enough.
func waitGroupGone(pgid int) {
	for syscall.Kill(-pgid, 0) == nil {
		time.Sleep(10 * time.Millisecond)
	}
}

func (p *Process) Close() error { p.Stop(); return nil }
func (p *Process) Wait() error  { <-p.done; return p.err }

// WaitStopped waits only for transport teardown, never for a UI callback.
func (p *Process) WaitStopped() { <-p.stopped }
