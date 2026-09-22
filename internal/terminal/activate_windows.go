package terminal

import (
	"context"
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
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

// activateSessionWindows targets only a window proven to belong to the live
// tracked session. Windows Terminal has no supported mapping from its child
// shell PID or WT_SESSION value to a visible HWND. Title lookup is not safe:
// one process can own several windows, the active tab can change, and users can
// disable terminal titles in the title bar. Refuse focus rather than guessing
// or invoking wt.exe, which would create a window if the target disappeared.
func activateSessionWindows(ctx context.Context, s *Session) error {
	_ = ctx
	if s.windowTitle == "" {
		return fmt.Errorf("cannot safely identify the existing terminal window; switch to it manually")
	}
	var hwnd windows.Handle
	if s.info.Kind == WindowsTerminal {
		return fmt.Errorf("Windows Terminal cannot be focused safely; switch to the existing window manually")
	} else {
		value, err := strconv.ParseUint(s.windowID, 10, 64)
		if err != nil || value == 0 {
			return fmt.Errorf("native PowerShell console handle is unavailable; switch to it manually")
		}
		hwnd = windows.Handle(value)
		visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if visible == 0 || windowText(hwnd) != s.windowTitle {
			return fmt.Errorf("native PowerShell console identity changed; switch to it manually")
		}
	}
	procShowWindowAsync.Call(uintptr(hwnd), swRestore)
	focused, _, focusErr := procSetForegroundWindow.Call(uintptr(hwnd))
	if focused == 0 {
		return fmt.Errorf("Windows refused to focus the existing terminal window: %v", focusErr)
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
