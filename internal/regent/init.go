package regent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mdelapenya/biomelab/internal/git"

	"github.com/mdelapenya/biomelab/internal/command"
)

// regentExcludeLine is the git info/exclude pattern that hides .regent/
// from `git status` and prevents accidental commits. Mirrors the
// "/.biomelab/" convention used by the notes package — anchored to the
// worktree root so each worktree's own .regent/ stays hidden.
const regentExcludeLine = "/.regent/"

// EnsureInit runs `rgt init <wtPath>` so the worktree gains the .regent/
// audit log and Claude Code hooks that re_gent ships. The call is a no-op
// for the rgt invocation in two cases:
//   - rgt is not in PATH (feature is opt-in; nothing to install)
//   - <wtPath>/.regent/ already exists (idempotency)
//
// The git info/exclude is always refreshed when wtPath is non-empty,
// even on the no-op branches above: a pre-existing .regent/ from a fresh
// rgt install may pre-date this code path, so we still want it hidden
// from `git status`. EnsureExcluded is itself idempotent.
//
// Returns a non-nil error only when rgt is present and the init command
// itself fails (or the exclude write fails). Callers typically log and
// continue — this is a best-effort capability bootstrap.
func EnsureInit(wtPath string) error {
	if wtPath == "" {
		return nil
	}

	regentExists := false
	if info, err := os.Stat(filepath.Join(wtPath, ".regent")); err == nil && info.IsDir() {
		regentExists = true
	}

	if !regentExists {
		bin, err := exec.LookPath("rgt")
		if err != nil {
			// rgt missing: skip everything — there's nothing to hide yet,
			// and writing hooks for a tool the user hasn't installed is
			// noise. Once rgt arrives, the next migration tick wires up.
			return nil
		}
		// rgt init does NOT accept a path arg — it always operates on the
		// current working directory. Set cmd.Dir to target wtPath.
		// `--skip-hook --skip-skills` keeps the call non-interactive: rgt's
		// hook installer is gated behind a TTY prompt that we can't answer
		// from a GUI process, so we ask rgt to set up only the .regent/
		// store and write the Claude Code hooks ourselves via
		// EnsureClaudeHooks below.
		cmd := command.Background(bin, "init", "--skip-hook", "--skip-skills")
		cmd.Dir = wtPath
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("rgt init: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	// Install Claude Code hooks ourselves — always, not only on fresh
	// init — so worktrees that previously ran `rgt init` without hooks
	// (e.g. when rgt's installer hit the no-TTY bug) get repaired on the
	// next migration tick. EnsureClaudeHooks is idempotent: existing rgt
	// entries are deduplicated, non-rgt entries are preserved.
	if err := EnsureClaudeHooks(wtPath); err != nil {
		return fmt.Errorf("install claude hooks: %w", err)
	}

	if err := git.EnsureExcluded(wtPath, regentExcludeLine); err != nil {
		return fmt.Errorf("exclude .regent/: %w", err)
	}
	return nil
}

// MigrateAll runs EnsureInit on each path, ignoring per-path errors. Used
// at startup to retroactively initialize regent on worktrees that existed
// before the user installed rgt (or before this feature shipped).
func MigrateAll(wtPaths []string) {
	for _, p := range wtPaths {
		_ = EnsureInit(p)
	}
}
