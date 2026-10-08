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
	readEnd sync.Once
	readEOF chan struct{} // closed when Read first returns an error (EOF)
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
	p := &Process{stopped: make(chan struct{}), file: f, cmd: cmd, done: make(chan struct{}), cancel: cancel, readEOF: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	go func() {
		select {
		case <-watch.Done():
		case <-p.done:
			// The root exited. A descendant that inherited the PTY slave
			// (for example a background job ignoring SIGHUP) would keep Read
			// blocked forever, so the session would never finish. Give the
			// reader a moment to drain the root's final output to EOF, then
			// tear the group down, as Windows does on root exit.
			select {
			case <-p.readEOF:
			case <-time.After(exitDrainGrace):
			case <-watch.Done():
			}
		}
		p.Stop()
	}()
	return p, nil
}

// exitDrainGrace is how long a natural exit waits for the PTY to reach EOF
// on its own before a lingering descendant is torn down.
const exitDrainGrace = 500 * time.Millisecond

func (p *Process) Read(b []byte) (int, error) {
	n, err := p.file.Read(b)
	if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
		err = io.EOF
	}
	if err != nil {
		p.readEnd.Do(func() { close(p.readEOF) })
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
			// should not open while a member of the shell's process group can
			// still hold the worktree as its cwd. Reap the leader first (an
			// unreaped zombie still counts as a member), then wait for the
			// group to disappear. The wait is bounded: a member stuck in
			// uninterruptible sleep cannot be killed, a recycled pgid could
			// otherwise be followed indefinitely, and restart waits on this
			// barrier without a timeout of its own. Unix allows removing a
			// directory that is still some process's cwd, so a late release
			// cannot make deletion fail. Jobs that an interactive shell moved
			// into their own process groups are outside this barrier.
			deadline := time.Now().Add(stopGroupTimeout)
			select {
			case <-p.done:
				waitGroupGone(pgid, deadline)
			case <-time.After(stopGroupTimeout):
			}
		}()
	})
}

// stopGroupTimeout bounds how long Stop waits for the killed group to exit.
const stopGroupTimeout = 5 * time.Second

// waitGroupGone polls until no process we can signal remains in group pgid,
// or until deadline. Signal 0 probes for existence without delivering
// anything. ESRCH means the group is gone; EPERM means the id now belongs to
// another user's group, so it ends the wait too. Descendants orphaned by the
// SIGKILL are reparented and reaped by init or launchd, so polling is enough.
func waitGroupGone(pgid int, deadline time.Time) {
	for syscall.Kill(-pgid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

func (p *Process) Close() error { p.Stop(); return nil }
func (p *Process) Wait() error  { <-p.done; return p.err }

// WaitStopped waits only for transport teardown, never for a UI callback.
func (p *Process) WaitStopped() { <-p.stopped }
