//go:build linux

package embeddedterminal

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Linux only: on macOS a session leader's exit drains and then revokes the
// controlling tty, so no output is ever left behind for the exit watcher's
// idle timer to cut off (TestNaturalExitWithDescendantHoldingPTYEndsSession
// covers the session ending there).
func TestNaturalExitKeepsOutputForSlowReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A background child that ignores SIGHUP writes the output at once and
	// keeps the PTY open; the root exits a little later, with that output
	// still unread, which is the case in which the exit watcher decides when
	// the session ends. The reader then drains slowly, as when each chunk
	// waits for a busy UI thread. Time spent outside Read must not count as
	// idle, or the tail is thrown away.
	p, err := Start(ctx, t.TempDir(), []string{"/bin/sh", "-c",
		`trap '' HUP; (i=0; while [ $i -lt 20 ]; do echo "line $i"; i=$((i+1)); done; printf TAIL-END; sleep 30) & sleep 0.3; exit 0`}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	select {
	case <-p.done: // the root has exited with the output still unread
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
