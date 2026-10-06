package regent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/command"
)

// ansiEscape matches CSI/OSC escape sequences emitted by tools that color
// their output. Stripped from rgt output so the dialog renders plain text.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07`)

// Log runs `rgt log --limit <limit>` in wtPath and returns the (ANSI-
// stripped) combined output. Returns an empty string with no error when
// .regent/ does not exist for the worktree — rgt would emit an error in
// that case and the GUI should render an empty state, not a panic.
// Sets NO_COLOR=1 so well-behaved tools skip color output; the regex
// strips anything that leaks through.
func Log(wtPath string, limit int) (string, error) {
	return LogContext(context.Background(), wtPath, limit)
}

// LogContext bounds the rgt command and allows callers to cancel stale reads.
func LogContext(ctx context.Context, wtPath string, limit int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if wtPath == "" {
		return "", nil
	}
	if _, err := os.Stat(filepath.Join(wtPath, ".regent")); err != nil {
		return "", nil
	}
	bin, err := exec.LookPath("rgt")
	if err != nil {
		return "", fmt.Errorf("rgt not found in PATH")
	}
	args := []string{"log"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	cmd := command.BackgroundContext(ctx, bin, args...)
	cmd.Dir = wtPath
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	clean := stripANSI(string(out))
	if err != nil {
		if ctx.Err() != nil {
			return clean, fmt.Errorf("rgt log: %w", ctx.Err())
		}
		return clean, fmt.Errorf("rgt log: %s: %w", strings.TrimSpace(clean), err)
	}
	return clean, nil
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
