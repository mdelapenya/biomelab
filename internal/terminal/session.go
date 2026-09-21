package terminal

import (
	"context"
	"time"
)

// SessionExists rechecks the shell CWD, emulator kind and process ancestry
// immediately before a user-requested activation. Cached PIDs alone are not
// sufficient: the process may have exited or its PID may have been reused.
// This is a best-effort check, not an atomic guarantee against process exit.
func SessionExists(expected Info, worktree string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return NewDetector().sessionExists(ctx, expected, worktree)
}

func (d *Detector) sessionExists(ctx context.Context, expected Info, worktree string) bool {
	procs, err := d.lister.Processes(ctx)
	if err != nil {
		return false
	}
	for _, current := range d.DetectFromProcessesContext(ctx, procs, []string{worktree})[worktree] {
		if current == expected && ctx.Err() == nil {
			return true
		}
	}
	return false
}
