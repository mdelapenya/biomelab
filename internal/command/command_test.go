package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

type helperResult struct {
	Args  []string
	Dir   string
	Env   string
	Input string
}

func TestCommandHelper(t *testing.T) {
	if os.Getenv("BIOMELAB_COMMAND_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	switch args[0] {
	case "echo":
		input, _ := io.ReadAll(os.Stdin)
		dir, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(helperResult{args[1:], dir, os.Getenv("BIOMELAB_COMMAND_VALUE"), string(input)})
		fmt.Fprint(os.Stderr, "helper stderr")
	case "exit":
		os.Exit(7)
	case "sleep":
		time.Sleep(time.Minute)
	case "short-sleep":
		time.Sleep(time.Second)
	case "hold-pipe":
		// Deliberately leave a short-lived descendant holding the output pipe
		// after this helper exits, as a CLI wrapper can do in production.
		child := Background(os.Args[0], "-test.run=^TestCommandHelper$", "--", "short-sleep")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func TestBackgroundBoundsInheritedPipeWait(t *testing.T) {
	cmd := Background(os.Args[0], "-test.run=^TestCommandHelper$", "--", "hold-pipe")
	cmd.Env = append(os.Environ(), "BIOMELAB_COMMAND_HELPER=1", "GORACE=atexit_sleep_ms=0")
	cmd.WaitDelay = 50 * time.Millisecond
	if _, err := cmd.Output(); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("want bounded pipe wait, got %v", err)
	}
}

func TestBackgroundIO(t *testing.T) {
	args := []string{"space here", "O'Brien", "日本語", "a;&$()", `C:\work tree\`}
	cmd := Background(os.Args[0], append([]string{"-test.run=^TestCommandHelper$", "--", "echo"}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "BIOMELAB_COMMAND_HELPER=1", "BIOMELAB_COMMAND_VALUE=preserved")
	cmd.Stdin = strings.NewReader("input\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var got helperResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	wantDir, err := os.Stat(cmd.Dir)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := os.Stat(got.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(wantDir, gotDir) || !reflect.DeepEqual(args, got.Args) || got.Env != "preserved" || got.Input != "input\n" || stderr.String() != "helper stderr" {
		t.Fatalf("I/O changed: %+v, stderr=%q", got, stderr.String())
	}
}

func TestBackgroundFailureAndCancellation(t *testing.T) {
	t.Run("exit status", func(t *testing.T) {
		cmd := Background(os.Args[0], "-test.run=^TestCommandHelper$", "--", "exit")
		cmd.Env = append(os.Environ(), "BIOMELAB_COMMAND_HELPER=1")
		var exitErr *exec.ExitError
		if err := cmd.Run(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("got %v, want exit 7", err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		cmd := BackgroundContext(ctx, os.Args[0], "-test.run=^TestCommandHelper$", "--", "sleep")
		cmd.Env = append(os.Environ(), "BIOMELAB_COMMAND_HELPER=1")
		start := time.Now()
		if err := cmd.Run(); err == nil {
			t.Fatal("expected cancellation")
		}
		if ctx.Err() != context.DeadlineExceeded || time.Since(start) > 5*time.Second {
			t.Fatal("command did not stop at deadline")
		}
	})
}
