package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OpenWithTitle opens a new terminal window with a biomelab-specific window
// title. The identifier (typically a branch name) is used to set the title
// to "biomelab: <identifier>", making the window discoverable for later
// activation via Activate().
//
// If identifier is empty, this behaves identically to Open.
func OpenWithTitle(dir, command, identifier string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("terminal launch on Windows is not supported yet; open a terminal in the worktree manually")
	}
	if identifier == "" {
		return Open(dir, command)
	}
	shellCmd, err := buildShellCmdWithTitle(dir, command, identifier)
	if err != nil {
		return err
	}
	if t := os.Getenv("BIOME_TERMINAL"); t != "" {
		return openCustomRaw(t, shellCmd)
	}
	switch runtime.GOOS {
	case "darwin":
		return darwinOpenRaw(shellCmd)
	case "linux":
		return linuxOpenRaw(shellCmd)
	default:
		return fmt.Errorf("terminal: unsupported platform %s — set BIOME_TERMINAL", runtime.GOOS)
	}
}

// Open opens a new terminal window.
// If command is non-empty, the terminal runs that command.
// If dir is non-empty, the terminal starts a shell in that directory.
func Open(dir, command string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("terminal launch on Windows is not supported yet; open a terminal in the worktree manually")
	}
	if t := os.Getenv("BIOME_TERMINAL"); t != "" {
		return openCustom(t, dir, command)
	}
	switch runtime.GOOS {
	case "darwin":
		return darwinOpen(dir, command)
	case "linux":
		return linuxOpen(dir, command)
	default:
		return fmt.Errorf("terminal: unsupported platform %s — set BIOME_TERMINAL", runtime.GOOS)
	}
}

// openCustom launches a user-specified terminal emulator with -e sh -c "...".
func openCustom(terminal, dir, command string) error {
	shellCmd, err := buildShellCmd(dir, command)
	if err != nil {
		return err
	}
	cmd := exec.Command(terminal, "-e", "sh", "-c", shellCmd)
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

// darwinOpen writes a temporary .command file and opens it with `open`.
// macOS natively executes .command files in the default terminal without
// requiring Automation or Accessibility permissions.
func darwinOpen(dir, command string) error {
	shellCmd, err := buildShellCmd(dir, command)
	if err != nil {
		return err
	}

	f, err := os.CreateTemp("", "biomelab-*.command")
	if err != nil {
		return fmt.Errorf("terminal: create temp file: %w", err)
	}
	path := f.Name()

	// The script removes itself on exit so temp files don't accumulate.
	script := "#!/bin/bash\nrm -f " + shellQuote(path) + "\n" + shellCmd + "\n"
	if _, err := f.WriteString(script); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("terminal: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("terminal: close temp file: %w", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("terminal: chmod temp file: %w", err)
	}

	cmd := exec.Command("open", filepath.Clean(path))
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// linuxOpen uses x-terminal-emulator (system default) to open a new window.
func linuxOpen(dir, command string) error {
	const term = "x-terminal-emulator"
	if _, err := exec.LookPath(term); err != nil {
		return fmt.Errorf("terminal: %s not found — set BIOME_TERMINAL", term)
	}

	shellCmd, err := buildShellCmd(dir, command)
	if err != nil {
		return err
	}

	cmd := exec.Command(term, "-e", "sh", "-c", shellCmd)
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

// buildShellCmd constructs a shell command string from dir and/or command.
func buildShellCmd(dir, command string) (string, error) {
	if command != "" {
		return command + "; exec $SHELL", nil
	}
	if dir != "" {
		return "cd " + shellQuote(dir) + "; exec $SHELL", nil
	}
	return "", fmt.Errorf("terminal: nothing to run (no dir or command)")
}

// buildShellCmdWithTitle wraps buildShellCmd and prepends an ANSI title escape.
// If identifier is empty, no title is prepended.
func buildShellCmdWithTitle(dir, command, identifier string) (string, error) {
	base, err := buildShellCmd(dir, command)
	if err != nil {
		return "", err
	}
	if identifier == "" {
		return base, nil
	}
	return titleEscape(identifier) + base, nil
}

// titleEscape returns a printf command that sets the terminal window title
// via the standard OSC (Operating System Command) escape sequence.
func titleEscape(identifier string) string {
	return "printf '\\033]0;%s\\007' " + shellQuote(Title(identifier)) + "; "
}

// openCustomRaw launches a user-specified terminal with a pre-built shell command.
func openCustomRaw(terminal, shellCmd string) error {
	cmd := exec.Command(terminal, "-e", "sh", "-c", shellCmd)
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

// darwinOpenRaw writes a pre-built shell command to a .command file and opens it.
func darwinOpenRaw(shellCmd string) error {
	f, err := os.CreateTemp("", "biomelab-*.command")
	if err != nil {
		return fmt.Errorf("terminal: create temp file: %w", err)
	}
	path := f.Name()

	script := "#!/bin/bash\nrm -f " + shellQuote(path) + "\n" + shellCmd + "\n"
	if _, err := f.WriteString(script); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("terminal: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("terminal: close temp file: %w", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("terminal: chmod temp file: %w", err)
	}

	cmd := exec.Command("open", filepath.Clean(path))
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// linuxOpenRaw launches x-terminal-emulator with a pre-built shell command.
func linuxOpenRaw(shellCmd string) error {
	const term = "x-terminal-emulator"
	if _, err := exec.LookPath(term); err != nil {
		return fmt.Errorf("terminal: %s not found — set BIOME_TERMINAL", term)
	}

	cmd := exec.Command(term, "-e", "sh", "-c", shellCmd)
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

// shellQuote wraps a string in single quotes for safe shell use.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// startLauncher reports immediate launcher failures, then reaps long-lived
// launchers asynchronously without waiting for the interactive session.
func startLauncher(cmd *exec.Cmd) error {
	return startLauncherWithGrace(cmd, 250*time.Millisecond)
}

func startLauncherWithGrace(cmd *exec.Cmd, grace time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("terminal launcher: %w", err)
		}
		return nil
	case <-timer.C:
		return nil
	}
}
