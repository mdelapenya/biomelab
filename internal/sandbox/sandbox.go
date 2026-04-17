package sandbox

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Available returns true if the sbx binary is found in PATH.
func Available() bool {
	_, err := exec.LookPath("sbx")
	return err == nil
}

// CreateArgs returns the arguments for creating a sandbox without attaching:
// sbx create --name <name> <agent> <repoPath>
func CreateArgs(name, agent, repoPath string) []string {
	return []string{"sbx", "create", "--name", name, agent, repoPath}
}

// RunDetachedWithBranchArgs returns the arguments for creating a worktree
// inside an existing sandbox (detached, non-interactive):
// sbx run -d --branch <branch> <sandboxName>
func RunDetachedWithBranchArgs(sandboxName, branch string) []string {
	return []string{"sbx", "run", "-d", "--branch", branch, sandboxName}
}

// RunWithBranchArgs returns the arguments for attaching to a *linked* sandbox
// worktree (interactive): sbx run --branch <branch> <sandboxName> -- -c
//
// The trailing `-- -c` is forwarded to the sandbox agent so it resumes the
// previous session (continue mode) instead of starting a fresh one — this
// preserves the agent's conversation/history context across reconnects.
//
// Do NOT use this for the main worktree — passing `--branch main` would make
// sbx create a new worktree called "main". Use RunArgs instead.
func RunWithBranchArgs(sandboxName, branch string) []string {
	return []string{"sbx", "run", "--branch", branch, sandboxName, "--", "-c"}
}

// RunArgs returns the arguments for attaching to the main sandbox worktree
// (interactive): sbx run <sandboxName> -- -c
//
// No --branch flag because the main worktree already exists at the sandbox
// root; passing --branch would make sbx create a new linked worktree.
func RunArgs(sandboxName string) []string {
	return []string{"sbx", "run", sandboxName, "--", "-c"}
}

// RemoveArgs returns the arguments for removing a sandbox:
// sbx rm --force <name>
func RemoveArgs(name string) []string {
	return []string{"sbx", "rm", "--force", name}
}

// Remove runs sbx rm to remove a sandbox. Returns output and any error.
func Remove(name string) (string, error) {
	cmd := exec.Command("sbx", "rm", "--force", name)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Start runs sbx run -d to start a stopped sandbox (detached, no attach).
func Start(name string) (string, error) {
	cmd := exec.Command("sbx", "run", "-d", name)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Stop runs sbx stop to stop a sandbox without removing it.
func Stop(name string) (string, error) {
	cmd := exec.Command("sbx", "stop", name)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// SanitizeName creates a safe sandbox name from parts.
// Replaces slashes and spaces with dashes, lowercases.
func SanitizeName(parts ...string) string {
	name := strings.Join(parts, "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ToLower(name)
	return name
}

// CommandString joins args into a single shell command string.
func CommandString(args []string) string {
	return strings.Join(args, " ")
}

// Create runs sbx create as a background process and returns when it completes.
// Returns the combined stdout+stderr output and any error.
func Create(args []string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunDetached runs an sbx command (typically run -d) as a background process.
// Returns the combined stdout+stderr output and any error.
func RunDetached(args []string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Status represents the state of a sandbox.
type Status int

const (
	StatusNotFound Status = iota // sandbox does not exist
	StatusRunning                // sandbox exists and is running
	StatusStopped                // sandbox exists but is stopped
)

// CheckAllStatuses returns a map of sandbox name → status for all known sandboxes.
// Runs one "sbx ls --json" call. Names not in the result have StatusNotFound.
func CheckAllStatuses() map[string]Status {
	cmd := exec.Command("sbx", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	var result struct {
		Sandboxes []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sandboxes"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil
	}
	m := make(map[string]Status, len(result.Sandboxes))
	for _, s := range result.Sandboxes {
		if s.Status == "running" {
			m[s.Name] = StatusRunning
		} else {
			m[s.Name] = StatusStopped
		}
	}
	return m
}

// CheckStatus returns the status of a sandbox by name.
// Runs "sbx ls --json" and looks for a matching entry.
func CheckStatus(name string) Status {
	cmd := exec.Command("sbx", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return StatusNotFound
	}
	var result struct {
		Sandboxes []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sandboxes"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return StatusNotFound
	}
	for _, s := range result.Sandboxes {
		if s.Name == name {
			if s.Status == "running" {
				return StatusRunning
			}
			return StatusStopped
		}
	}
	return StatusNotFound
}

// VersionInfo holds sbx client and server version strings.
type VersionInfo struct {
	Client string
	Server string
}

// Version returns the sbx client and server versions.
func Version() VersionInfo {
	cmd := exec.Command("sbx", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return VersionInfo{}
	}
	var info VersionInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "Client Version:"); ok {
			info.Client = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "Server Version:"); ok {
			info.Server = strings.TrimSpace(after)
		}
	}
	return info
}

// Preflight checks whether sbx is fully bootstrapped (installed, authenticated,
// daemon running, network policy set). Runs "sbx ls --json" which exercises
// the full stack. Returns nil if ready, or an error with user-facing instructions.
func Preflight() error {
	if !Available() {
		return fmt.Errorf("sbx CLI not found in PATH — install it from https://docs.docker.com/ai/sandboxes/")
	}
	cmd := exec.Command("sbx", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		return fmt.Errorf("sbx not ready — run 'sbx ls' in a terminal to complete setup (auth, network policy).\nsbx output: %s", output)
	}
	return nil
}
