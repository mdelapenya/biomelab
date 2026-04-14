package warp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	openTabs   = make(map[string]bool)
	openTabsMu sync.Mutex
)

// OpenTab opens a new terminal panel for a worktree.
// If no tab for the repo exists yet, it creates one and runs the cd (+agent) there.
// Subsequent calls add a split panel to the existing repo tab.
func OpenTab(repoName, dir, agentCmd string) error {
	openTabsMu.Lock()
	tabExists := openTabs[repoName]
	if !tabExists {
		// Mark as created before releasing the lock to prevent duplicate creation.
		openTabs[repoName] = true
	}
	openTabsMu.Unlock()

	if !tabExists {
		if err := createRepoTab(repoName, dir, agentCmd); err != nil {
			// Roll back on failure.
			openTabsMu.Lock()
			delete(openTabs, repoName)
			openTabsMu.Unlock()
			return err
		}
		// The new tab already has the cd + agent command, no split needed.
		return nil
	}

	if err := focusRepoTab(repoName); err != nil {
		if err := createRepoTab(repoName, dir, agentCmd); err != nil {
			return err
		}
		return nil
	}

	return splitPanel(dir, agentCmd)
}

func createRepoTab(repoName, dir, agentCmd string) error {
	switch runtime.GOOS {
	case "darwin":
		return darwinNewTab(dir, agentCmd)
	case "linux":
		return linuxNewTab(repoName, dir)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func focusRepoTab(repoName string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return darwinFocusTab(repoName)
}

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

// --- macOS ---

func darwinNewTab(dir, agentCmd string) error {
	var text string
	if dir != "" {
		text = "cd " + escAS(dir)
		if agentCmd != "" {
			text += " && " + agentCmd
		}
	} else {
		text = agentCmd
	}

	return osascript([]string{
		`tell application "System Events"`,
		`    keystroke "t" using command down`,
		`    delay 0.5`,
		`    keystroke "` + text + `"`,
		`    key code 36`,
		`end tell`,
	})
}

func darwinFocusTab(repoName string) error {
	out, err := osascriptOutput([]string{
		`tell application "System Events"`,
		`    tell process "Warp"`,
		`        set frontmost to true`,
		`        repeat 20 times`,
		`            set winTitle to name of front window`,
		`            if winTitle contains "` + escAS(repoName) + `" then`,
		`                return "found"`,
		`            end if`,
		`            keystroke "]" using {shift down, command down}`,
		`            delay 0.1`,
		`        end repeat`,
		`    end tell`,
		`end tell`,
		`return "not found"`,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "found" {
		return fmt.Errorf("tab not found")
	}
	return nil
}

func darwinSplitPanel(dir, agentCmd string) error {
	var text string
	if dir != "" {
		text = "cd " + escAS(dir)
		if agentCmd != "" {
			text += " && " + agentCmd
		}
	} else {
		text = agentCmd
	}

	return osascript([]string{
		`tell application "System Events"`,
		`    keystroke "d" using {shift down, command down}`,
		`    delay 0.5`,
		`    keystroke "` + text + `"`,
		`    key code 36`,
		`end tell`,
	})
}

func osascript(lines []string) error {
	cmd := exec.Command("osascript", "-e", strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func osascriptOutput(lines []string) ([]byte, error) {
	cmd := exec.Command("osascript", "-e", strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// shellQuote wraps a string in single quotes for safe shell use.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// escAS escapes double quotes for use inside AppleScript double-quoted strings.
// Does NOT escape backslashes — they are needed for shell escape sequences
// like \033 inside printf commands.
func escAS(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// --- Linux ---

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

	return fmt.Errorf("no supported terminal found")
}

func linuxSplitPanel(dir, agentCmd string) error {
	var shellCmd string
	if dir != "" {
		shellCmd = "cd " + shellQuote(dir)
		if agentCmd != "" {
			shellCmd += " && " + agentCmd
		}
	} else {
		shellCmd = agentCmd
	}
	shellCmd += "; exec $SHELL"

	terminals := []struct {
		bin  string
		args []string
	}{
		{"gnome-terminal", []string{"--tab", "--working-directory", dir, "--", "sh", "-c", shellCmd}},
		{"konsole", []string{"--new-tab", "--workdir", dir, "-e", "sh", "-c", shellCmd}},
		{"xfce4-terminal", []string{"--tab", "--working-directory", dir, "--", "sh", "-c", shellCmd}},
	}

	for _, t := range terminals {
		if _, err := exec.LookPath(t.bin); err == nil {
			cmd := exec.Command(t.bin, t.args...)
			cmd.Stderr = os.Stderr
			return cmd.Start()
		}
	}

	return fmt.Errorf("no supported terminal found")
}
