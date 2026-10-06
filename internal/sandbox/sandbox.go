package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/command"
)

// Available returns true if the sbx binary is found in PATH.
func Available() bool {
	_, err := exec.LookPath("sbx")
	return err == nil
}

// CreateArgs returns the arguments for creating a sandbox without attaching:
// sbx create --name <name> [--kit <url>...] <agent> <repoPath>
//
// kitURLs is optional; pass nil for a vanilla sandbox. Each reference is
// emitted as its own --kit flag. Biomelab offers kits only during initial
// creation; updating an existing agent container's kits is not exposed.
func CreateArgs(name, agent, repoPath string, kitURLs []string) []string {
	args := []string{"sbx", "create", "--name", name}
	for _, k := range kitURLs {
		args = append(args, "--kit", k)
	}
	return append(args, agent, repoPath)
}

// CreateWithKitArgs uses a sandbox kit as the positional workload. Only mixin
// references belong in --kit flags; the engine permits one sandbox base.
func CreateWithKitArgs(name, sandboxRef, repoPath string, mixinRefs []string) []string {
	return CreateArgs(name, sandboxRef, repoPath, mixinRefs)
}

// StartAgentScript is the well-known path where the sbx daemon writes the
// per-sandbox agent launcher. `sbx run` executes it as
// `/bin/bash <script>`; we do the same via `sbx exec` so a worktree session
// gets the agent's default flags and persistent env.
const StartAgentScript = "/usr/local/lib/sandbox/start-agent"

// RunAttachArgs returns the arguments for attaching an interactive agent
// session to an existing sandbox in its primary workspace (repo root):
// sbx run --name <sandboxName>
func RunAttachArgs(sandboxName string) []string {
	return []string{"sbx", "run", "--name", sandboxName}
}

// ExecAgentArgs returns the arguments for an interactive agent session that
// starts inside workdir, a host worktree path translated with ContainerPath.
// It runs the daemon's start-agent script when present and falls back to the
// bare agent binary. sbx exec starts a stopped sandbox automatically.
func ExecAgentArgs(sandboxName, workdir, agent string) []string {
	script := "if [ -f " + StartAgentScript + " ]; then exec /bin/bash " +
		StartAgentScript + "; else exec " + ShellQuote(agent) + "; fi"
	return []string{"sbx", "exec", "-it", "-w", ContainerPath(workdir), sandboxName, "bash", "-c", script}
}

// ContainerPath maps a host path to the path the sandbox sees for it. sbx
// mirrors the host workspace inside the container, so on macOS and Linux the
// two spellings are identical and the path is returned unchanged. A Windows
// drive-letter path is not: sbx mounts C:\a\b at /c/a/b, and passing the host
// spelling to `sbx exec -w` fails with "chdir ...: No such file or directory".
// Anything that is not a drive-letter path (including UNC paths, which have no
// mirrored mount point) is left alone for the caller to fail on visibly.
func ContainerPath(hostPath string) string {
	if len(hostPath) < 2 || hostPath[1] != ':' || !isDriveLetter(hostPath[0]) {
		return hostPath
	}
	rest := strings.TrimPrefix(strings.ReplaceAll(hostPath[2:], `\`, "/"), "/")
	return "/" + strings.ToLower(hostPath[:1]) + "/" + rest
}

func isDriveLetter(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// ShellQuote returns s safe for interpolation into a POSIX shell command
// line. Simple tokens are returned unchanged; anything else is single-quoted.
func ShellQuote(s string) string {
	if s != "" && safeShellToken.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var safeShellToken = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)

// RemoveArgs returns the arguments for removing a sandbox:
// sbx rm --force <name>
func RemoveArgs(name string) []string {
	return []string{"sbx", "rm", "--force", name}
}

// Remove runs sbx rm to remove a sandbox. Returns output and any error.
func Remove(name string) (string, error) {
	cmd := command.Background("sbx", "rm", "--force", name)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Start runs sbx run -d to start a stopped sandbox (detached, no attach).
func Start(name string) (string, error) {
	cmd := command.Background("sbx", "run", "-d", name)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Stop runs sbx stop to stop a sandbox without removing it.
func Stop(name string) (string, error) {
	cmd := command.Background("sbx", "stop", name)
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

var generatedNamePart = regexp.MustCompile(`[^a-z0-9-]+`)

// GeneratedName is a stable, repo-specific name for a new sandbox. A readable
// path basename is followed by a digest of the canonical absolute repo path,
// so two unrelated repositories named "project" cannot adopt each other.
func GeneratedName(repoPath, agent string) string {
	if repoPath == "" || agent == "" {
		return ""
	}
	path, err := filepath.Abs(repoPath)
	if err != nil {
		return ""
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	label := safeGeneratedPart(filepath.Base(path), 30)
	actor := safeGeneratedPart(agent, 16)
	digest := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s-%s-%x", label, actor, digest[:6])
}

func safeGeneratedPart(value string, limit int) string {
	value = generatedNamePart.ReplaceAllString(strings.ToLower(value), "-")
	value = strings.Trim(value, "-")
	if len(value) > limit {
		value = strings.TrimRight(value[:limit], "-")
	}
	if value == "" {
		return "repo"
	}
	return value
}

// Candidates contains only an explicitly stored association and the
// repo-specific generated name. Bare/full repository names are ambiguous and
// must never be auto-adopted from the daemon's name-only listing.
func Candidates(storedName, repoName, repoPath, agent string) []string {
	_ = repoName // retained in the API for callers with stored legacy entries.
	generated := GeneratedName(repoPath, agent)
	if storedName == "" {
		if generated == "" {
			return nil
		}
		return []string{generated}
	}
	if generated == "" || generated == storedName {
		return []string{storedName}
	}
	return []string{storedName, generated}
}

// MatchStatus returns the first candidate that exists in statusMap along with
// its status. ok=false means none matched.
func MatchStatus(statusMap map[string]Status, candidates []string) (name string, status Status, ok bool) {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if s, found := statusMap[c]; found {
			return c, s, true
		}
	}
	return "", StatusNotFound, false
}

// CommandString joins args into a single shell command line. Every argument
// is shell-quoted so the result is safe to hand to `bash`/`sh -c`.
func CommandString(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = ShellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// Create runs sbx create as a background process and returns when it completes.
// Returns the combined stdout+stderr output and any error.
func Create(args []string) (string, error) {
	cmd := command.Background(args[0], args[1:]...)
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
	statuses, _ := ListStatuses()
	return statuses
}

// ListStatuses distinguishes an unavailable daemon from a missing sandbox.
// Creation must not interpret discovery errors as permission to create anew.
func ListStatuses() (map[string]Status, error) {
	return ListStatusesContext(context.Background())
}

func ListStatusesContext(ctx context.Context) (map[string]Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, "sbx", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	var result struct {
		Sandboxes []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sandboxes"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse sandbox listing: %w", err)
	}
	m := make(map[string]Status, len(result.Sandboxes))
	for _, s := range result.Sandboxes {
		if s.Status == "running" {
			m[s.Name] = StatusRunning
		} else {
			m[s.Name] = StatusStopped
		}
	}
	return m, nil
}

// CheckStatus returns the status of a sandbox by name.
// Runs "sbx ls --json" and looks for a matching entry.
func CheckStatus(name string) Status {
	cmd := command.Background("sbx", "ls", "--json")
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
	return VersionContext(context.Background())
}

func VersionContext(ctx context.Context) VersionInfo {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, "sbx", "version")
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
	return PreflightContext(context.Background())
}

// PreflightContext bounds the daemon probe and accepts caller cancellation.
func PreflightContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sbx readiness check: %w", err)
	}
	if !Available() {
		return fmt.Errorf("sbx CLI not found in PATH — install it from https://docs.docker.com/ai/sandboxes/")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, "sbx", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("sbx readiness check: %w", ctx.Err())
		}
		output := strings.TrimSpace(string(out))
		return fmt.Errorf("sbx not ready — run 'sbx ls' in a terminal to complete setup (auth, network policy).\nsbx output: %s", output)
	}
	return nil
}
