package ops

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/regent"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// CreateWorktreeResult is the outcome of creating a worktree.
type CreateWorktreeResult struct {
	BranchName string
	Err        error
}

// ErrorMessage returns a user-facing error message, or "" if Err is nil.
func (r CreateWorktreeResult) ErrorMessage() string {
	if r.Err == nil {
		return ""
	}
	return r.Err.Error()
}

// CreateWorktree creates a new linked worktree for the given branch and
// bootstraps the re_gent audit log on it (a no-op when rgt isn't installed).
// Init failures don't surface to the caller — the worktree itself is the
// product; rgt is a sidecar capability that the user can wire up later.
func CreateWorktree(repo *git.Repository, branchName string) CreateWorktreeResult {
	if err := repo.CreateWorktree(branchName); err != nil {
		return CreateWorktreeResult{BranchName: branchName, Err: err}
	}
	if path := worktreePathForBranch(repo, branchName); path != "" {
		_ = regent.EnsureInit(path)
	}
	return CreateWorktreeResult{BranchName: branchName}
}

// worktreePathForBranch resolves the on-disk path of a freshly-created
// worktree by re-listing and matching on branch name. Returns "" when no
// match is found — the caller treats this as "skip the optional follow-up
// step" rather than as an error.
func worktreePathForBranch(repo *git.Repository, branchName string) string {
	wts, err := repo.ListWorktrees()
	if err != nil {
		return ""
	}
	for _, wt := range wts {
		if wt.Branch == branchName {
			return wt.Path
		}
	}
	return ""
}

// RemoveWorktree removes a linked worktree by branch name.
func RemoveWorktree(repo *git.Repository, name string) error {
	return repo.RemoveWorktree(name)
}

// FetchPRResult is the outcome of fetching a PR.
type FetchPRResult struct {
	BranchName string
	WtPath     string
	Err        error
}

// FetchPR fetches a PR into a new worktree.
func FetchPR(repo *git.Repository, input string) FetchPRResult {
	ref, err := github.ParsePRRef(input)
	if err != nil {
		return FetchPRResult{Err: err}
	}

	headBranch, err := github.ValidatePR(repo.Root(), ref)
	if err != nil {
		return FetchPRResult{Err: err}
	}

	remoteURL := ""
	if ref.Repo != "" {
		remoteURL = "https://github.com/" + ref.Repo + ".git"
	}

	wtPath, err := repo.FetchPR(ref.Number, headBranch, remoteURL)
	return FetchPRResult{BranchName: headBranch, WtPath: wtPath, Err: err}
}

// Pull fetches all remotes and merges from origin.
func Pull(repo *git.Repository) error {
	return repo.Pull()
}

// SendPRResult is the outcome of pushing + creating a PR.
type SendPRResult struct {
	URL string
	Err error
}

// SendPR pushes a branch to a remote and creates a PR. title, when
// non-empty, becomes the PR title (overriding the usual commit-subject
// derivation). bodyFile, when non-empty, must be an absolute path to a
// markdown file whose contents will replace the default commit-derived
// description.
func SendPR(repo *git.Repository, prProv provider.PRProvider, branch string, remote git.RemoteInfo, title, bodyFile string) SendPRResult {
	if err := repo.Push(remote.Name, branch); err != nil {
		return SendPRResult{Err: fmt.Errorf("push: %w", err)}
	}
	pr, err := prProv.CreatePR(repo.Root(), branch, remote.Repo, title, bodyFile)
	if err != nil {
		return SendPRResult{Err: err}
	}
	return SendPRResult{URL: pr.URL}
}

// PushBranch pushes a branch to a remote without creating a PR.
// Used when a PR already exists and the user wants to push new commits.
func PushBranch(repo *git.Repository, branch string, remote git.RemoteInfo) error {
	return repo.Push(remote.Name, branch)
}

// editorAppNames maps CLI command names to macOS application names.
// Used as a fallback when the CLI isn't in PATH (e.g., launched from Spotlight).
var editorAppNames = map[string]string{
	"code":     "Visual Studio Code",
	"cursor":   "Cursor",
	"zed":      "Zed",
	"windsurf": "Windsurf",
	"goland":   "GoLand",
	"idea":     "IntelliJ IDEA",
	"pycharm":  "PyCharm",
}

// OpenEditor opens the worktree directory in the configured editor.
// Uses $BIOME_EDITOR if set, otherwise defaults to "code" (VS Code).
//
// When launched as a GUI app (Spotlight/Finder), the shell PATH is minimal
// and CLI tools like "code" aren't found. On macOS, falls back to
// "open -a <AppName>" which finds the app regardless of PATH.
func OpenEditor(dir string) error {
	editor := os.Getenv("BIOME_EDITOR")
	if editor == "" {
		editor = "code"
	}

	// Try the CLI command directly.
	cmd := exec.Command(editor, dir)
	if err := cmd.Start(); err == nil {
		return nil
	}

	// Fallback on macOS: use "open -a <AppName>" which doesn't need PATH.
	if runtime.GOOS == "darwin" {
		appName := editor
		if mapped, ok := editorAppNames[editor]; ok {
			appName = mapped
		}
		return exec.Command("open", "-a", appName, dir).Start()
	}

	return fmt.Errorf("%s: command not found", editor)
}

// OpenTerminal opens a terminal window for the given directory or command.
// If identifier is non-empty, the terminal title is set to "biomelab: <identifier>".
func OpenTerminal(dir, command, identifier string) error {
	if identifier != "" {
		return terminal.OpenWithTitle(dir, command, identifier)
	}
	return terminal.Open(dir, command)
}

// ActivateTerminalByPID looks up the shell's TTY and brings the owning
// terminal window to the foreground. Returns true if successful.
func ActivateTerminalByPID(shellPID, rootPID int32, kind terminal.Kind) bool {
	return terminal.ActivateByPID(shellPID, rootPID, kind)
}

// ActivateTerminalApp brings a terminal emulator application to the foreground
// without targeting a specific window. Use as a last-resort fallback.
func ActivateTerminalApp(kind terminal.Kind) bool {
	return terminal.ActivateApp(kind)
}
