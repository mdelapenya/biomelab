package tui

import (
	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/provider"
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
	repoPath       string // identifies which repo this message belongs to
	source         refreshSource
	worktrees      []git.Worktree
	agents         agent.DetectionResult
	ides           ide.DetectionResult
	prs            provider.PRResult
	hasPRs         bool   // true only when a network refresh attempted PR lookup
	err            error
	fetchErr       error
	sandboxStatus  int                // sandbox.Status value for active mode
	allSbxStatuses map[string]int    // all sandbox name → status (for tree dots)
	sbxClientVer   string            // sbx client version
	sbxServerVer   string            // sbx server version
}

// worktreeCreatedMsg is sent after a worktree is successfully created.
type worktreeCreatedMsg struct {
	repoPath   string
	branchName string
	err        error
	sbxOutput  string // output from sbx create (empty for regular worktrees)
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

// sandboxStartedMsg is sent after sbx run -d (start) completes.
type sandboxStartedMsg struct {
	repoPath    string
	sandboxName string
	err         error
}

// sandboxStoppedCmdMsg is sent after sbx stop completes.
type sandboxStoppedCmdMsg struct {
	repoPath    string
	sandboxName string
	err         error
}

// sandboxRemovedMsg is sent after sbx rm completes.
type sandboxRemovedMsg struct {
	repoPath    string
	sandboxName string
	output      string
	err         error
}

// sandboxCreatedFromCardMsg is sent after sbx create completes from the main card.
type sandboxCreatedFromCardMsg struct {
	repoPath    string
	sandboxName string
	output      string
	err         error
}

// enrollSandboxRequestMsg is sent by the model when a non-sandbox repo
// wants to enroll in sandbox mode from the main card.
type enrollSandboxRequestMsg struct {
	repoPath string
	mode     config.ModeEntry
}

// repoValidatedMsg is sent after a repo path has been validated
// but before the user selects regular/sandbox mode.
type repoValidatedMsg struct {
	path string
	name string
	repo *git.Repository
	err  error
}
