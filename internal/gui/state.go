package gui

import (
	"slices"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
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
	// ViewList shows compact worktree rows beside the selected-item inspector.
	ViewList
)

// RepoState holds all UI-relevant state for a single repo+mode.
type RepoState struct {
	// Domain data from business logic.
	Worktrees        []git.Worktree
	Agents           agent.DetectionResult
	IDEs             ide.DetectionResult
	Terminals        terminal.DetectionResult
	PRs              provider.PRResult
	CLIAvail         provider.CLIAvailability
	HasCLIAvail      bool
	Provider         provider.Provider
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

	// LastAppliedGen is the highest repo generation we've already applied
	// from a refresh result. Refresh results carrying a lower generation
	// are dropped to avoid a long-running refresh resurrecting a worktree
	// the user just deleted (or hiding one they just created).
	LastAppliedGen uint64
}

// SandboxCardInfo holds sandbox display data for a single card.
type SandboxCardInfo struct {
	Name          string
	Status        sandbox.Status
	Agent         string
	ClientVersion string
	ServerVersion string
	Kits          []config.KitInstall
}

// Apply updates the state from a refresh result. Returns false when the
// result's snapshot generation predates the last applied snapshot — this
// is the staleness guard that keeps a long-running refresh from
// resurrecting a worktree the user just deleted. Errors always pass
// through so users still see them.
//
// Pure state mutation: no UI side-effects, no AfterFunc, no fyne calls.
// Dashboard.ApplyRefresh rebuilds the view and preserves visible selection.
func (s *RepoState) Apply(result ops.RefreshResult) bool {
	if result.Err == nil && result.Generation < s.LastAppliedGen {
		return false
	}
	if result.Generation > s.LastAppliedGen {
		s.LastAppliedGen = result.Generation
	}
	if result.Err != nil {
		s.StatusMessage = ops.FirstNonEmptyLine(result.Err.Error())
		s.StatusIsError = true
		return true
	}
	if result.Worktrees != nil {
		s.SetWorktrees(result.Worktrees)
		s.prunePRs()
	}
	if result.Agents != nil {
		s.Agents = result.Agents
	}
	if result.IDEs != nil {
		s.IDEs = result.IDEs
	}
	if result.Terminals != nil {
		s.Terminals = result.Terminals
	}
	if result.HasCLIAvail {
		s.CLIAvail = result.CLIAvail
		s.HasCLIAvail = true
	}
	if result.HasPRs {
		if result.HasPRLookups {
			if s.PRs == nil {
				s.PRs = make(provider.PRResult)
			}
			for branch, lookup := range result.PRLookups {
				if lookup.Err != nil {
					continue // retain the last known status on a transient failure
				}
				if lookup.Info == nil {
					delete(s.PRs, branch) // confirmed no PR for this branch
				} else {
					s.PRs[branch] = lookup.Info
				}
			}
			s.prunePRs()
		} else {
			s.PRs = result.PRs // legacy providers expose only a full snapshot
		}
		s.LastNetworkRefresh = time.Now()
		s.NetFlash = true
	} else {
		s.LastLocalRefresh = time.Now()
		s.LocalFlash = true
	}
	// Update sandbox status only when the refresh actually checked it.
	if result.HasSbxStatus {
		s.SandboxStatus = result.SandboxStatus
	}
	if result.SbxClientVer != "" {
		s.SbxClientVersion = result.SbxClientVer
	}
	if result.SbxServerVer != "" {
		s.SbxServerVersion = result.SbxServerVer
	}
	// NOTE: StatusMessage is intentionally NOT cleared on a successful
	// refresh. Refreshes happen every couple of seconds; clearing here
	// wipes user-action messages ("Created X", "Pull complete", CLI
	// errors) almost immediately. Status stays until another setStatus
	// call or an Esc dismisses it.
	return true
}

func (s *RepoState) prunePRs() {
	if len(s.PRs) == 0 {
		return
	}
	branches := make(map[string]bool, len(s.Worktrees))
	for _, wt := range s.Worktrees {
		branches[wt.Branch] = true
	}
	for branch := range s.PRs {
		if !branches[branch] {
			delete(s.PRs, branch)
		}
	}
}

// SetWorktrees stores worktrees and sorts linked ones alphabetically by branch.
// This ensures deterministic rendering order that matches navigation order.
func (s *RepoState) SetWorktrees(wts []git.Worktree) {
	selectedPath := ""
	if s.SelectedCard >= 0 && s.SelectedCard < len(s.Worktrees) {
		selectedPath = s.Worktrees[s.SelectedCard].Path
	}
	wts = append([]git.Worktree(nil), wts...)
	if len(wts) > 1 {
		slices.SortFunc(wts[1:], func(a, b git.Worktree) int {
			return strings.Compare(strings.ToLower(a.Branch), strings.ToLower(b.Branch))
		})
	}
	s.Worktrees = wts
	if len(wts) == 0 {
		s.SelectedCard = 0
		return
	}
	if selectedPath != "" {
		for i, wt := range wts {
			if wt.Path == selectedPath {
				s.SelectedCard = i
				return
			}
		}
	}
	// The selected worktree disappeared. Stay near its former position
	// without leaving a selection that points beyond the refreshed list.
	s.SelectedCard = min(max(s.SelectedCard, 0), len(wts)-1)
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
