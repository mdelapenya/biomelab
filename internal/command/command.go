// Package command constructs noninteractive helper processes. Requested terminal
// windows and application launches must use their own platform launch policy.
package command

import (
	"context"
	"os/exec"
	"time"
)

// Background constructs a helper that does not create a Windows console window.
// It preserves exec.Cmd's argument, environment, directory and I/O behavior.
// Prefer BackgroundContext when the caller has a cancellation scope or deadline.
func Background(name string, args ...string) *exec.Cmd {
	return configure(exec.Command(name, args...))
}

// BackgroundContext constructs a cancellable helper without a Windows console window.
// The caller owns the deadline; interactive sessions must not use this function.
func BackgroundContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return configure(exec.CommandContext(ctx, name, args...))
}

func configure(cmd *exec.Cmd) *exec.Cmd {
	configurePlatform(cmd)
	// A descendant can keep output pipes open after the direct child exits or
	// is cancelled. Bound that wait without imposing a runtime limit on the job.
	// Cancellation still kills only the direct child, not a whole process tree.
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
