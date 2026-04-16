package ops

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// CreateWorktreeResult is the outcome of creating a worktree.
type CreateWorktreeResult struct {
	BranchName string
	SbxOutput  string
	Err        error
}

// CreateWorktree creates a new linked worktree for the given branch.
func CreateWorktree(repo *git.Repository, branchName string) CreateWorktreeResult {
	err := repo.CreateWorktree(branchName)
	return CreateWorktreeResult{BranchName: branchName, Err: err}
}

// CreateSandboxWorktree creates a worktree inside an existing sandbox.
func CreateSandboxWorktree(sandboxName, branch string) CreateWorktreeResult {
	args := sandbox.RunDetachedWithBranchArgs(sandboxName, branch)
	out, err := sandbox.RunDetached(args)
	return CreateWorktreeResult{BranchName: branch, SbxOutput: out, Err: err}
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

// FetchPRSandbox fetches a PR ref and creates the worktree inside a sandbox.
func FetchPRSandbox(repo *git.Repository, input, sandboxName string) FetchPRResult {
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

	if err := repo.FetchPRRef(ref.Number, headBranch, remoteURL); err != nil {
		return FetchPRResult{Err: err}
	}

	args := sandbox.RunDetachedWithBranchArgs(sandboxName, headBranch)
	out, err := sandbox.RunDetached(args)
	if err != nil {
		return FetchPRResult{Err: fmt.Errorf("sbx worktree: %w: %s", err, out)}
	}

	return FetchPRResult{BranchName: headBranch}
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

// SendPR pushes a branch to a remote and creates a PR.
func SendPR(repo *git.Repository, prProv provider.PRProvider, branch string, remote git.RemoteInfo) SendPRResult {
	if err := repo.Push(remote.Name, branch); err != nil {
		return SendPRResult{Err: fmt.Errorf("push: %w", err)}
	}
	pr, err := prProv.CreatePR(repo.Root(), branch, remote.Repo)
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
func OpenTerminal(dir, command string) error {
	return terminal.Open(dir, command)
}
