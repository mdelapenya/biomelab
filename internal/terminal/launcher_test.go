package terminal

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/mdelapenya/biomelab/internal/command"
)

func TestLauncherHelper(t *testing.T) {
	switch os.Getenv("BIOMELAB_LAUNCHER_HELPER") {
	case "failure":
		os.Exit(7)
	case "session":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
}

func TestLauncherReportsEarlyFailureWithoutWaitingForSession(t *testing.T) {
	cmd := command.Background(os.Args[0], "-test.run=^TestLauncherHelper$")
	cmd.Env = append(os.Environ(), "BIOMELAB_LAUNCHER_HELPER=failure", "GORACE=atexit_sleep_ms=0")
	// Give the helper ample startup time on loaded Windows CI hosts. The
	// production grace is shorter; this tests propagation of an observed exit.
	if err := startLauncherWithGrace(cmd, 5*time.Second); err == nil {
		t.Fatal("immediate launcher failure was lost")
	}

	cmd = command.Background(os.Args[0], "-test.run=^TestLauncherHelper$")
	cmd.Env = append(os.Environ(), "BIOMELAB_LAUNCHER_HELPER=session", "GORACE=atexit_sleep_ms=0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	done := make(chan error, 1)
	go func() { done <- startLauncher(cmd) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waited for a terminal session that is still reading input")
	}
}

func TestTitleIsShellData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX terminal command")
	}
	title := "O'Brien %s $(printf injected); 日本語"
	out, err := exec.Command("sh", "-c", titleEscape(title)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "\033]0;"+Title(title)+"\007" {
		t.Fatalf("title was interpreted: %q", out)
	}
}
