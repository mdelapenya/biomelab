package warp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

var (
	// tracks which repo tabs have been created in this session.
	openTabs   = make(map[string]bool)
	openTabsMu sync.Mutex
)

// OpenTab opens a new terminal panel for a worktree.
// If no tab for the repo exists yet, it creates one first.
// Subsequent calls add a split panel to the existing repo tab.
func OpenTab(repoName, dir, agentCmd string) error {
	openTabsMu.Lock()
	tabExists := openTabs[repoName]
	openTabsMu.Unlock()

	if !tabExists {
		if err := createRepoTab(repoName, dir); err != nil {
			return err
		}
		openTabsMu.Lock()
		openTabs[repoName] = true
		openTabsMu.Unlock()
	} else {
		// Focus the repo tab before splitting.
		if err := focusRepoTab(repoName); err != nil {
			// If we can't focus, create a new tab instead.
			if err := createRepoTab(repoName, dir); err != nil {
				return err
			}
		}
	}

	return splitPanel(dir, agentCmd)
}

// createRepoTab creates a new terminal tab and sets its title to the repo name.
func createRepoTab(repoName, dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return darwinNewTab(repoName, dir)
	case "linux":
		return linuxNewTab(repoName, dir)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// focusRepoTab brings the repo tab to focus.
func focusRepoTab(repoName string) error {
	if runtime.GOOS != "darwin" {
		// On non-macOS, we can't reliably focus tabs. Assume it's still focused.
		return nil
	}
	return darwinFocusTab(repoName)
}

// splitPanel opens a split panel in the current tab and runs the command.
func splitPanel(dir, agentCmd string) error {
	switch runtime.GOOS {
	case "darwin":
		return darwinSplitPanel(dir, agentCmd)
	case "linux":
		return linuxSplitPanel(dir, agentCmd)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// --- macOS implementations using keyboard shortcuts via System Events ---

func darwinNewTab(repoName, dir string) error {
	// Cmd+T for new tab, then set title via escape sequence.
	titleCmd := fmt.Sprintf(`printf '\033]0;%s\007'`, repoName)
	script := fmt.Sprintf(`
tell application "System Events"
    keystroke "t" using command down
    delay 0.5
    keystroke "cd %s && %s"
    key code 36
end tell`, dir, titleCmd)

	cmd := exec.Command("osascript", "-e", script)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func darwinFocusTab(repoName string) error {
	// Use Cmd+Shift+[ and Cmd+Shift+] to cycle tabs and check titles.
	// This is fragile, so we just try to find the tab by title using
	// System Events accessibility attributes on all UI elements.
	// If not found within a reasonable attempt, return error.
	script := fmt.Sprintf(`
tell application "System Events"
    tell process "Warp"
        set frontmost to true
        -- Try to find tab by cycling through them (max 20 tabs)
        repeat 20 times
            -- Get the title of the front window
            set winTitle to name of front window
            if winTitle contains %q then
                return "found"
            end if
            -- Move to next tab
            keystroke "}" using command down
            delay 0.1
        end repeat
    end tell
end tell
return "not found"`, repoName)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if string(out) != "found\n" {
		return fmt.Errorf("tab %q not found", repoName)
	}
	return nil
}

func darwinSplitPanel(dir, agentCmd string) error {
	command := fmt.Sprintf("cd %s", dir)
	if agentCmd != "" {
		command += " && " + agentCmd
	}

	script := fmt.Sprintf(`
tell application "System Events"
    keystroke "d" using {shift down, command down}
    delay 0.5
    keystroke %q
    key code 36
end tell`, command)

	cmd := exec.Command("osascript", "-e", script)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --- Linux implementations ---

func linuxNewTab(repoName, dir string) error {
	terminals := []struct {
		bin  string
		args []string
	}{
		{"gnome-terminal", []string{"--tab", "--title", repoName, "--working-directory", dir}},
		{"konsole", []string{"--new-tab", "--workdir", dir, "-p", "tabtitle=" + repoName}},
		{"xfce4-terminal", []string{"--tab", "--title", repoName, "--working-directory", dir}},
	}

	for _, t := range terminals {
		if _, err := exec.LookPath(t.bin); err == nil {
			cmd := exec.Command(t.bin, t.args...)
			cmd.Stderr = os.Stderr
			return cmd.Start()
		}
	}

	return fmt.Errorf("no supported terminal found — consider running inside tmux")
}

func linuxSplitPanel(dir, agentCmd string) error {
	// Most Linux terminals don't support split panels.
	// Fall back to opening a new tab.
	command := ""
	if agentCmd != "" {
		command = agentCmd
	}

	terminals := []struct {
		bin  string
		args []string
	}{
		{"gnome-terminal", []string{"--tab", "--working-directory", dir}},
		{"konsole", []string{"--new-tab", "--workdir", dir}},
		{"xfce4-terminal", []string{"--tab", "--working-directory", dir}},
	}

	for _, t := range terminals {
		if _, err := exec.LookPath(t.bin); err == nil {
			args := t.args
			if command != "" {
				args = append(args, "--", "sh", "-c", command+"; exec $SHELL")
			}
			cmd := exec.Command(t.bin, args...)
			cmd.Stderr = os.Stderr
			return cmd.Start()
		}
	}

	return fmt.Errorf("no supported terminal found")
}
