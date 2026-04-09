package tui

import (
	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/ide"
	"github.com/mdelapenya/gwaim/internal/provider"
)

// refreshSource identifies which refresh path produced a refreshMsg.
type refreshSource int

const (
	refreshSourceQuick   refreshSource = iota // fast branch-only load, no flash
	refreshSourceLocal                        // periodic local refresh (dirty + agents)
	refreshSourceNetwork                      // periodic network refresh (fetch + PRs)
)

// refreshMsg carries updated worktree and agent data.
type refreshMsg struct {
	repoPath  string // identifies which repo this message belongs to
	source    refreshSource
	worktrees []git.Worktree
	agents    agent.DetectionResult
	ides      ide.DetectionResult
	prs       provider.PRResult
	hasPRs    bool // true only when a network refresh attempted PR lookup
	err       error
	fetchErr  error
}

// worktreeCreatedMsg is sent after a worktree is successfully created.
type worktreeCreatedMsg struct {
	repoPath   string
	branchName string
	err        error
}

// worktreeRemovedMsg is sent after a worktree is removed.
type worktreeRemovedMsg struct {
	repoPath string
	err      error
}

// prFetchedMsg is sent after a PR has been fetched into a new worktree.
type prFetchedMsg struct {
	repoPath   string
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
	repoPath string
	err      error
}

// editorOpenedMsg is sent after attempting to open an editor.
type editorOpenedMsg struct {
	err error
}

// cardRefreshMsg carries updated state for a single worktree card.
type cardRefreshMsg struct {
	repoPath  string
	wtPath    string // worktree path identifying the card
	worktrees []git.Worktree
	agents    agent.DetectionResult
	ides      ide.DetectionResult
	prs       provider.PRResult
	hasPRs    bool
	err       error
	fetchErr  error
}

// tickMsg triggers a network refresh (fetch + PRs).
type tickMsg struct {
	repoPath string
}

// localTickMsg triggers a local-only refresh (dirty status + agent detection, no network).
type localTickMsg struct {
	repoPath string
}

// localFlashDoneMsg clears the local-refresh ✓ indicator after its display window.
type localFlashDoneMsg struct {
	repoPath string
}

// netFlashDoneMsg clears the network-refresh ✓ indicator after its display window.
type netFlashDoneMsg struct {
	repoPath string
}

// cliCheckMsg carries the result of the CLI pre-flight check.
type cliCheckMsg struct {
	repoPath string
	avail    provider.CLIAvailability
}
