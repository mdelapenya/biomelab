package gui

import (
	"slices"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// ViewMode controls how the linked worktrees are displayed.
type ViewMode int

const (
	// ViewKanban groups worktrees into columns by PR lifecycle stage.
	// It is the default view (zero value).
	ViewKanban ViewMode = iota
	// ViewGrid is the responsive card grid layout.
	ViewGrid
)

// RepoState holds all UI-relevant state for a single repo+mode.
type RepoState struct {
	// Domain data from business logic.
	Worktrees     []git.Worktree
	Agents        agent.DetectionResult
	IDEs          ide.DetectionResult
	Terminals     terminal.DetectionResult
	PRs           provider.PRResult
	CLIAvail      provider.CLIAvailability
	Provider      provider.Provider
	SandboxStatus    sandbox.Status
	SbxClientVersion string
	SbxServerVersion string
	ActiveMode       *config.ModeEntry

	// UI state.
	ViewMode           ViewMode
	SelectedCard       int
	LastLocalRefresh   time.Time
	LastNetworkRefresh time.Time
	LocalFlash         bool // true briefly after a local refresh (✓ indicator)
	NetFlash           bool // true briefly after a network refresh (✓ indicator)
	StatusMessage      string
	StatusIsError      bool
}

// SandboxCardInfo holds sandbox display data for a single card.
type SandboxCardInfo struct {
	Name          string
	Status        sandbox.Status
	Agent         string
	ClientVersion string
	ServerVersion string
}

// SetWorktrees stores worktrees and sorts linked ones alphabetically by branch.
// This ensures deterministic rendering order that matches navigation order.
func (s *RepoState) SetWorktrees(wts []git.Worktree) {
	if len(wts) > 1 {
		slices.SortFunc(wts[1:], func(a, b git.Worktree) int {
			return strings.Compare(strings.ToLower(a.Branch), strings.ToLower(b.Branch))
		})
	}
	s.Worktrees = wts
}

// MainWorktree returns the first (main) worktree, or nil if none.
func (s *RepoState) MainWorktree() *git.Worktree {
	if len(s.Worktrees) == 0 {
		return nil
	}
	return &s.Worktrees[0]
}

// LinkedWorktrees returns all worktrees except the main one.
func (s *RepoState) LinkedWorktrees() []git.Worktree {
	if len(s.Worktrees) <= 1 {
		return nil
	}
	return s.Worktrees[1:]
}
