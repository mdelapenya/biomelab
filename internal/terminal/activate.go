package terminal

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// titlePrefix is the string prepended to all biomelab terminal window titles.
const titlePrefix = "biomelab: "

// Title returns the full window title for a given identifier (usually a branch name).
func Title(identifier string) string {
	return titlePrefix + identifier
}

// ActivateByPID looks up the TTY for the given shell PID and activates the
// terminal window/tab that owns it. This is the most reliable activation
// method — it doesn't depend on window titles (which shell prompts overwrite).
//
// On macOS: resolves PID → tty via lsof, then uses AppleScript to find and
// activate the Terminal.app/iTerm2 tab with that tty.
// On Linux: uses xdotool to find and activate the window owned by rootPID.
func ActivateByPID(shellPID, rootPID int32, kind Kind) bool {
	switch runtime.GOOS {
	case "darwin":
		tty := ttyForPID(shellPID)
		if tty == "" {
			return false
		}
		ok, _ := activateDarwinByTTY(tty, kind)
		return ok
	case "linux":
		return activateLinuxByPID(rootPID)
	default:
		return false
	}
}

// ActivateApp brings a terminal emulator application to the foreground without
// searching for a specific window. Use this as a last-resort fallback.
func ActivateApp(kind Kind) bool {
	switch runtime.GOOS {
	case "darwin":
		return activateAppDarwin(kind)
	case "linux":
		return activateAppLinux(kind)
	default:
		return false
	}
}

// ttyForPID resolves the controlling terminal device for a process.
// Returns a path like "/dev/ttys003" or "" if unavailable.
func ttyForPID(pid int32) string {
	cmd := exec.Command("lsof", "-p", fmt.Sprintf("%d", pid), "-a", "-d", "0", "-F", "n")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/dev/") {
			return line[1:] // strip the 'n' field prefix
		}
	}
	return ""
}

// activateDarwinByTTY uses AppleScript to find and activate the Terminal.app
// or iTerm2 tab whose tty matches, then brings its window to front.
func activateDarwinByTTY(tty string, kind Kind) (bool, error) {
	switch kind {
	case ITerm2:
		return activateITerm2ByTTY(tty)
	default:
		// Terminal.app and other emulators that follow Terminal.app's scripting model.
		return activateTerminalAppByTTY(tty)
	}
}

func activateTerminalAppByTTY(tty string) (bool, error) {
	script := fmt.Sprintf(`
tell application "Terminal"
	repeat with w in windows
		repeat with t in tabs of w
			if tty of t is %q then
				set selected tab of w to t
				set index of w to 1
				activate
				return true
			end if
		end repeat
	end repeat
end tell
return false`, tty)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func activateITerm2ByTTY(tty string) (bool, error) {
	script := fmt.Sprintf(`
tell application "iTerm"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if tty of s is %q then
					select s
					tell w to select t
					activate
					return true
				end if
			end repeat
		end repeat
	end repeat
end tell
return false`, tty)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// activateLinuxByPID uses xdotool to find and activate the window owned by
// the terminal emulator's root PID.
func activateLinuxByPID(rootPID int32) bool {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return false
	}

	cmd := exec.Command("xdotool", "search", "--pid", fmt.Sprintf("%d", rootPID))
	out, err := cmd.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return false
	}

	windowID := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	activateCmd := exec.Command("xdotool", "windowactivate", windowID)
	return activateCmd.Run() == nil
}

// darwinAppNames maps terminal Kind to the macOS application name used by
// `open -a`. Only populated for emulators likely found on macOS.
var darwinAppNames = map[Kind]string{
	TerminalApp: "Terminal",
	ITerm2:      "iTerm",
	Alacritty:   "Alacritty",
	Kitty:       "kitty",
	WezTerm:     "WezTerm",
	Hyper:       "Hyper",
}

func activateAppDarwin(kind Kind) bool {
	name, ok := darwinAppNames[kind]
	if !ok {
		return false
	}
	cmd := exec.Command("open", "-a", name)
	return cmd.Run() == nil
}

func activateAppLinux(kind Kind) bool {
	if _, err := exec.LookPath("wmctrl"); err != nil {
		return false
	}
	class := strings.ToLower(string(kind))
	cmd := exec.Command("wmctrl", "-x", "-a", class)
	return cmd.Run() == nil
}
