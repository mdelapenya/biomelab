package terminal

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/mdelapenya/biomelab/internal/command"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procShowWindowAsync      = user32.NewProc("ShowWindowAsync")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
)

const swRestore = 9

// activateSessionWindows raises the window of a live tracked session. The
// caller has already confirmed the shell is alive, so a window Biomelab opened
// still exists.
//
// A Windows Terminal window we launched carries a per-session name (wtWindow),
// and `wt -w <name> focus-tab` raises exactly that window by name — unaffected
// by the tab title the shell may overwrite, and without the caller needing to
// map a pseudoconsole to a visible HWND. A classic console instead reports its
// own HWND through the handshake, re-validated here by title before focus.
//
// A Windows Terminal session with no name is one the OS default-terminal
// setting delegated a PowerShell fallback into: we never invoked wt.exe for it,
// so there is no window to name or target. That case still refuses focus.
func activateSessionWindows(ctx context.Context, s *Session) error {
	if s.info.Kind == WindowsTerminal {
		if s.wtWindow == "" {
			return fmt.Errorf("this Windows Terminal session was started by the OS default-terminal setting and cannot be focused automatically; switch to the existing window manually")
		}
		return focusWindowsTerminal(ctx, s.wtWindow)
	}
	if s.windowTitle == "" {
		return fmt.Errorf("cannot safely identify the existing terminal window; switch to it manually")
	}
	value, err := strconv.ParseUint(s.windowID, 10, 64)
	if err != nil || value == 0 {
		return fmt.Errorf("native PowerShell console handle is unavailable; switch to it manually")
	}
	hwnd := windows.Handle(value)
	visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	if visible == 0 || windowText(hwnd) != s.windowTitle {
		return fmt.Errorf("native PowerShell console identity changed; switch to it manually")
	}
	procShowWindowAsync.Call(uintptr(hwnd), swRestore)
	focused, _, focusErr := procSetForegroundWindow.Call(uintptr(hwnd))
	if focused == 0 {
		return fmt.Errorf("Windows refused to focus the existing terminal window: %v", focusErr)
	}
	return nil
}

// focusWindowsTerminal raises the named Windows Terminal window. Windows
// Terminal itself resolves the name to the right window and activates it, so no
// HWND lookup is needed; targeting a name whose window is gone is a no-op rather
// than opening a new window. The helper runs without its own console window.
func focusWindowsTerminal(ctx context.Context, window string) error {
	wt, err := exec.LookPath("wt.exe")
	if err != nil {
		return fmt.Errorf("Windows Terminal (wt.exe) is unavailable to focus the existing window; switch to it manually")
	}
	if err := command.BackgroundContext(ctx, wt, "-w", window, "focus-tab").Run(); err != nil {
		return fmt.Errorf("could not focus the existing Windows Terminal window: %w", err)
	}
	return nil
}

func windowText(hwnd windows.Handle) string {
	length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}
	buf := make([]uint16, int(length)+1)
	written, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if written == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:written])
}
