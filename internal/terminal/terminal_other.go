//go:build !windows

package terminal

import (
	"context"
	"fmt"
)

func windowsOpen(launchRequest) error {
	return fmt.Errorf("terminal: Windows launcher is unavailable on this platform")
}

func activateSessionWindows(context.Context, *Session) error {
	return fmt.Errorf("terminal: Windows activation is unavailable on this platform")
}
