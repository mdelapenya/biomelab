package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mdelapenya/biomelab/internal/command/internal/consolediag"
)

func TestWindowsConsoleHelper(t *testing.T) {
	mode := os.Getenv("BIOMELAB_CONSOLE_HELPER")
	if mode == "" {
		return
	}
	role := "policy-child"
	if mode == "parent" {
		role = "policy-parent"
	}
	if err := consolediag.Snapshot(role, "ready").Emit(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "helper could not write console diagnostics")
		os.Exit(2)
	}
	if mode == "parent" {
		// Each generation must apply the policy explicitly; creation flags
		// are not inherited by arbitrary third-party descendants.
		cmd := Background(os.Args[0], "-test.run=^TestWindowsConsoleHelper$")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONSOLE_HELPER=child")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := consolediag.Run(role, cmd); err != nil {
			fmt.Fprintln(os.Stderr, "policy parent failed; inspect process trace stages and exit codes")
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func TestWindowsGUIBackgroundHasNoConsoleWindow(t *testing.T) {
	if os.Getenv(consolediag.DirectoryEnv) == "" {
		t.Setenv(consolediag.DirectoryEnv, t.TempDir())
	}
	launcher := filepath.Join(t.TempDir(), "launcher.exe")
	build := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", launcher, "./testdata/launcher")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build GUI launcher: %v\n%s", err, out)
	}
	for _, mode := range []string{"child", "parent"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(launcher, os.Args[0], "-test.run=^TestWindowsConsoleHelper$")
			cmd.Env = append(os.Environ(), "BIOMELAB_CONSOLE_HELPER="+mode)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if stderr.Len() > 0 {
				t.Logf("helper stderr: %s", stderr.Bytes()[:min(stderr.Len(), 8192)])
			}
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(out))
			var states []consolediag.State
			for {
				var state consolediag.State
				err := decoder.Decode(&state)
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("decode diagnostic stream (%d bytes): %v", len(out), err)
				}
				t.Logf("console state: %+v", state)
				states = append(states, state)
				// CREATE_NO_WINDOW suppresses windows, not all console state. Code pages
				// and attached-process counts are diagnostics and may be nonzero.
				if state.Window != 0 {
					t.Errorf("unexpected console window: %+v", state)
				}
				if state.PID <= 0 || state.PPID <= 0 || state.Phase != "ready" {
					t.Errorf("incomplete process snapshot: %+v", state)
				}
			}
			expected := []string{"policy-child"}
			if mode == "parent" {
				expected = []string{"policy-parent", "policy-child"}
			}
			if len(states) != len(expected) {
				t.Fatalf("got %d snapshots, want %d", len(states), len(expected))
			}
			for i, role := range expected {
				if states[i].Role != role {
					t.Errorf("snapshot %d role=%q want %q", i, states[i].Role, role)
				}
			}
			if mode == "parent" && states[1].PPID != states[0].PID {
				t.Error("descendant is not attached to the expected process tree")
			}

		})
	}
}
