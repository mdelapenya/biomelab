//go:build darwin || linux

package embeddedterminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestProcessWorkingDirectoryArgumentsAndReaping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir := t.TempDir()
	p, err := Start(ctx, dir, []string{"/bin/sh", "-c", `printf '%s\n' "$PWD" "$1"; stty size`, "sh", "literal ' space $value"}, 23, 81)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	b, err := io.ReadAll(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"literal ' space $value", "23 81"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output %q missing %q", text, want)
		}
	}
	// macOS may resolve /var to /private/var in the shell's PWD.
	if !strings.Contains(text, dir) {
		t.Fatalf("working directory missing: %q", text)
	}
}

func TestStopTerminatesProcessIgnoringEOFAndHangup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c", `trap '' HUP; printf ready; while :; do sleep 1; done`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	ready := make([]byte, 5)
	if _, err := io.ReadFull(p, ready); err != nil {
		t.Fatal(err)
	}
	cancel()
	p.Stop()
	p.Stop()
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("child survived cancellation")
	}
}

func TestCanceledStartDoesNotLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, "", []string{"/bin/sh", "-c", "exit 0"}, 24, 80); err == nil {
		t.Fatal("canceled start succeeded")
	}
}

func TestConcurrentResizeAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c", `trap '' HUP; printf ready; while :; do sleep 1; done`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	ready := make([]byte, 5)
	if _, err := io.ReadFull(p, ready); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				if err := p.Resize(uint16(20+j%30), uint16(70+j%40)); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					return
				}
			}
		}()
	}
	p.Stop()
	p.Stop()
	finished := make(chan struct{})
	go func() {
		workers.Wait()
		p.WaitStopped()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent resize and Stop did not finish")
	}
	if err := p.Resize(30, 100); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("post-close Resize = %v, want closed pipe", err)
	}
	reaped := make(chan struct{})
	go func() { _ = p.Wait(); close(reaped) }()
	select {
	case <-reaped:
	case <-time.After(3 * time.Second):
		t.Fatal("child survived Stop")
	}
}

func TestWaitStoppedWaitsForDescendantsToExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The backgrounded subshell ignores HUP and outlives the root's EOF, so
	// only the group SIGKILL can end it. WaitStopped must not open before it
	// is gone: deletion and restart rely on that barrier.
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c",
		`trap '' HUP; (trap '' HUP; while :; do sleep 1; done) & printf 'pid=%s;ready' "$!"; wait`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	var out []byte
	buf := make([]byte, 64)
	for !strings.Contains(string(out), "ready") {
		n, err := p.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			t.Fatalf("read %q: %v", out, err)
		}
	}
	var pid int
	if _, err := fmt.Sscanf(string(out[strings.Index(string(out), "pid="):]), "pid=%d;", &pid); err != nil || pid <= 0 {
		t.Fatalf("descendant pid from %q: %v", out, err)
	}
	p.Stop()
	stopped := make(chan struct{})
	go func() { p.WaitStopped(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitStopped did not return after Stop")
	}
	if err := processEnded(pid); err != nil {
		t.Fatalf("descendant %d still exists when WaitStopped returned: %v", pid, err)
	}
}

func TestNaturalExitWithDescendantHoldingPTYEndsSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The root exits normally while a backgrounded child that ignores HUP
	// still holds the PTY slave. The session must still reach EOF (so the
	// drawer can collapse) after the root's final output, and the child
	// must be gone once WaitStopped returns.
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c",
		`trap '' HUP; sleep 30 & printf 'pid=%s;ROOT-FINAL' "$!"; exit 0`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	result := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(p)
		result <- b
	}()
	var out []byte
	select {
	case out = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("root exit with a descendant holding the PTY never reached EOF")
	}
	if !strings.Contains(string(out), "ROOT-FINAL") {
		t.Fatalf("final output lost: %q", out)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("natural exit status: %v", err)
	}
	var pid int
	if i := strings.Index(string(out), "pid="); i < 0 {
		t.Fatalf("missing descendant pid: %q", out)
	} else if _, err := fmt.Sscanf(string(out[i:]), "pid=%d;", &pid); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { p.WaitStopped(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitStopped did not return after natural exit")
	}
	if err := processEnded(pid); err != nil {
		t.Fatalf("descendant %d survived natural exit: %v", pid, err)
	}
}

func TestNaturalExitKeepsOutputForSlowReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The root exits at once, leaving a background child that ignores SIGHUP
	// and keeps the PTY open: the case in which the exit watcher decides when
	// the session ends. The child writes the output only after the root has
	// exited, so the root never has unread output of its own (on macOS a
	// session leader's exit waits for its tty to drain) and the watcher's
	// idle timer runs on Linux and macOS alike. The reader then drains
	// slowly, as when each chunk waits for a busy UI thread. Time spent
	// outside Read must not count as idle, or the tail is thrown away. The
	// output stays below the smallest PTY buffer (about 1 KiB on macOS).
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c",
		`trap '' HUP; (sleep 0.2; i=0; while [ $i -lt 20 ]; do echo "line $i"; i=$((i+1)); done; printf TAIL-END) & exit 0`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	select {
	case <-p.done: // the root has exited before any output exists
	case <-time.After(5 * time.Second):
		t.Fatal("root did not exit")
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("natural exit status: %v", err)
	}
	result := make(chan []byte, 1)
	go func() {
		var out []byte
		buf := make([]byte, 48)
		for {
			n, err := p.Read(buf)
			out = append(out, buf[:n]...)
			if err != nil {
				result <- out
				return
			}
			time.Sleep(exitDrainGrace + 300*time.Millisecond)
		}
	}()
	var out []byte
	select {
	case out = <-result:
	case <-time.After(30 * time.Second):
		t.Fatal("session never reached EOF after the root exited")
	}
	if !strings.Contains(string(out), "TAIL-END") {
		t.Fatalf("final output cut off after %d bytes; tail: %q", len(out), out[max(0, len(out)-80):])
	}
}

// processEnded reports whether pid has stopped running: it no longer exists,
// its id now belongs to another user (EPERM), or it is a zombie. A killed
// descendant is reparented and stays a zombie until init or launchd reaps it
// (forever under a PID 1 that never reaps); it no longer runs or holds a
// working directory, but kill(pid, 0) still succeeds for it. Checked once,
// so a process that is still running when WaitStopped returns fails.
func processEnded(pid int) error {
	switch err := syscall.Kill(pid, 0); {
	case errors.Is(err, syscall.ESRCH), errors.Is(err, syscall.EPERM):
		return nil
	case err != nil:
		return err
	}
	state, err := processState(pid)
	if err != nil {
		return err
	}
	if state == "Z" || state == "gone" {
		return nil
	}
	return fmt.Errorf("process is still running (state %q)", state)
}

// processState returns the one-letter scheduler state of pid, from /proc on
// Linux and from ps elsewhere (macOS).
func processState(pid int) (string, error) {
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		// The state follows the parenthesised command name, which may
		// itself contain spaces or parentheses.
		s := string(b)
		if i := strings.LastIndexByte(s, ')'); i >= 0 && i+2 < len(s) {
			return s[i+2 : i+3], nil
		}
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return "gone", nil
		}
		return "", fmt.Errorf("ps: %w", err)
	}
	stat := strings.TrimSpace(string(out))
	if stat == "" {
		return "gone", nil
	}
	return stat[:1], nil
}
