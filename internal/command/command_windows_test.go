package command

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

type consoleState struct {
	Window   uintptr
	Codepage uint32
}

func TestWindowsConsoleHelper(t *testing.T) {
	mode := os.Getenv("BIOMELAB_CONSOLE_HELPER")
	if mode == "" {
		return
	}
	if mode == "parent" {
		// Each generation must apply the policy explicitly; creation flags
		// are not inherited by arbitrary third-party descendants.
		cmd := Background(os.Args[0], "-test.run=^TestWindowsConsoleHelper$")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONSOLE_HELPER=child")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	} else {
		proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
		hwnd, _, _ := proc.Call()
		codepage, _ := windows.GetConsoleCP()
		_ = json.NewEncoder(os.Stdout).Encode(consoleState{hwnd, codepage})
	}
	os.Exit(0)
}

func TestWindowsGUIBackgroundHasNoConsoleWindow(t *testing.T) {
	launcher := filepath.Join(t.TempDir(), "launcher.exe")
	build := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", launcher, "./testdata/launcher")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build GUI launcher: %v\n%s", err, out)
	}
	for _, mode := range []string{"child", "parent"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(launcher, os.Args[0], "-test.run=^TestWindowsConsoleHelper$")
			cmd.Env = append(os.Environ(), "BIOMELAB_CONSOLE_HELPER="+mode)
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			var state consoleState
			if err := json.Unmarshal(out, &state); err != nil {
				t.Fatalf("decode %q: %v", out, err)
			}
			// CREATE_NO_WINDOW suppresses the console window, but a windowless
			// console can still report a code page (437 on the Windows runner).
			// Keep it as diagnostic data, not a console-window assertion.
			if state.Window != 0 {
				t.Fatalf("unexpected console window: %+v", state)
			}
		})
	}
}
