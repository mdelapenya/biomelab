//go:build darwin || linux

package embeddedterminal

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	if err := waitPidGone(pid, 2*time.Second); err != nil {
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
	if err := waitPidGone(pid, 2*time.Second); err != nil {
		t.Fatalf("descendant %d survived natural exit: %v", pid, err)
	}
}

func TestNaturalExitKeepsOutputForSlowReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The root writes a little output and exits at once. The reader then
	// drains it in small chunks, slowly, as when each chunk waits for a busy
	// UI thread. Time spent outside Read must not count as idle, or the tail
	// is thrown away. The output stays far below the smallest PTY buffer
	// (about 1 KiB on macOS), so the root never blocks on an unread PTY.
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c",
		`i=0; while [ $i -lt 20 ]; do echo "line $i"; i=$((i+1)); done; printf TAIL-END; exit 0`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	select {
	case <-p.done: // the root has exited before the first read
	case <-time.After(5 * time.Second):
		t.Fatal("root did not exit")
	}
	var out []byte
	buf := make([]byte, 48)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		n, err := p.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	if !strings.Contains(string(out), "TAIL-END") {
		t.Fatalf("final output cut off after %d bytes; tail: %q", len(out), out[max(0, len(out)-80):])
	}
}

// waitPidGone polls until pid no longer exists. A killed descendant is
// reparented and remains a zombie until init or launchd reaps it; it no
// longer runs or holds a working directory, but kill(pid, 0) still succeeds
// for it. A process that was never killed (sleep 30) stays alive far longer
// than the window, so the check still catches a real survivor.
func waitPidGone(pid int, window time.Duration) error {
	deadline := time.Now().Add(window)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			if err == nil {
				return errors.New("process still exists")
			}
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
}
