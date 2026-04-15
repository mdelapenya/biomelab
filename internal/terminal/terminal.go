package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Open opens a new terminal window.
// If command is non-empty, the terminal runs that command.
// If dir is non-empty, the terminal starts a shell in that directory.
func Open(dir, command string) error {
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
	return cmd.Start()
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
	return cmd.Start()
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

// shellQuote wraps a string in single quotes for safe shell use.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
