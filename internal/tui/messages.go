package tui

import (
	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/github"
)

// refreshMsg carries updated worktree and agent data.
type refreshMsg struct {
	worktrees []git.Worktree
	agents    agent.DetectionResult
	prs       github.PRResult
	hasPRs    bool // true only when a network refresh attempted PR lookup
	err       error
	fetchErr  error
}

// worktreeCreatedMsg is sent after a worktree is successfully created.
type worktreeCreatedMsg struct {
	branchName string
	err        error
}

// worktreeRemovedMsg is sent after a worktree is removed.
type worktreeRemovedMsg struct {
	err error
}

// prFetchedMsg is sent after a PR has been fetched into a new worktree.
type prFetchedMsg struct {
	branchName string
	wtPath     string // actual worktree directory (may differ from branchName if sanitized)
	err        error
}

// warpOpenedMsg is sent after attempting to open a Warp panel.
type warpOpenedMsg struct {
	err error
}

// pullMsg is sent after a git pull completes.
type pullMsg struct {
	err error
}

// editorOpenedMsg is sent after attempting to open an editor.
type editorOpenedMsg struct {
	err error
}

// worktreeRepairedMsg is sent after a worktree repair completes.
type worktreeRepairedMsg struct {
	output string // per-worktree repair details from git, empty if nothing needed repair
	err    error
}

// tickMsg triggers a network refresh (fetch + PRs).
type tickMsg struct{}

// localTickMsg triggers a local-only refresh (dirty status + agent detection, no network).
type localTickMsg struct{}

// ghCheckMsg carries the result of the gh CLI pre-flight check.
type ghCheckMsg struct {
	avail github.GHAvailability
}
