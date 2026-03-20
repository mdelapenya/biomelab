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

// tickMsg triggers a periodic refresh.
type tickMsg struct{}
