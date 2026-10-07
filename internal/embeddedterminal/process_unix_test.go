//go:build darwin || linux

package embeddedterminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
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
