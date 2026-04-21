package gui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

type focusPanel int

const (
	focusRight focusPanel = iota
	focusLeft
)

// handleKeyName processes special key events (arrows, Enter, Esc, Tab).
// Called from both desktop.Canvas.SetOnKeyDown (for Tab, before Fyne intercepts)
// and Canvas.SetOnTypedKey (for repeated keys).
func (a *App) handleKeyName(key fyne.KeyName) {
	// Escape dismisses open dialogs via their Hide() method.
	// This triggers Fyne's proper cleanup (unlike manual overlay removal
	// which corrupts the canvas state and breaks SetOnKeyDown).
	if a.dialogOpen && key == fyne.KeyEscape {
		if a.activeDialog != nil {
			a.activeDialog.Hide()
			a.activeDialog = nil
		}
		return
	}
	if a.dialogOpen {
		return
	}

	// Special keys.
	switch key {
	case fyne.KeyUp:
		a.navigateUp()
		return
	case fyne.KeyDown:
		a.navigateDown()
		return
	case fyne.KeyLeft:
		a.navigateLeft()
		return
	case fyne.KeyRight:
		a.navigateRight()
		return
	case fyne.KeyReturn:
		a.handleEnter()
		return
	case fyne.KeyEscape:
		a.handleEscape()
		return
	case fyne.KeyTab:
		a.toggleFocus()
		return
	}

	// Letter keys — handled here instead of SetOnTypedRune because
	// onTypedRune only fires when canvas.Focused()==nil, which can
	// silently break after dialog dismissal. SetOnKeyDown always fires.
	// View toggle is global across both panels.
	if key == fyne.KeyG {
		a.toggleView()
		return
	}

	if a.focus == focusLeft {
		switch key {
		case fyne.KeyJ:
			a.navigateDown()
		case fyne.KeyK:
			a.navigateUp()
		case fyne.KeyA:
			a.handleAddRepo()
		case fyne.KeyN:
			a.handleAddSandboxMode()
		case fyne.KeyX:
			a.handleRemoveMode()
		}
		return
	}

	switch key {
	case fyne.KeyJ:
		a.navigateDown()
	case fyne.KeyK:
		a.navigateUp()
	case fyne.KeyH:
		a.navigateLeft()
	case fyne.KeyL:
		a.navigateRight()
	case fyne.KeyR:
		a.handleRefresh()
	case fyne.KeyC:
		a.handleCreate()
	case fyne.KeyD:
		a.handleDeleteOrRemoveSandbox()
	case fyne.KeyF:
		a.handleFetchPR()
	case fyne.KeyP:
		a.handlePull() // Shift+P (send PR) handled in handleRune via 'P'
	case fyne.KeyE:
		a.handleOpenEditor()
	case fyne.KeyS:
		a.handleStartSandbox() // Shift+S (stop) handled in handleRune via 'S'
	case fyne.KeyN:
		a.handleCreateOrEnrollSandbox()
	}
}

// handleRune processes character key events that need case sensitivity.
// SetOnKeyDown can't distinguish 's' from 'S' (no modifier info in KeyEvent),
// so Shift+S (stop sandbox) and Shift+P (send PR) are handled here.
// Called from canvas.SetOnTypedRune — only fires when canvas.Focused()==nil.
func (a *App) handleRune(r rune) {
	if a.dialogOpen {
		return
	}
	switch r {
	case 'S':
		a.handleStopSandbox()
	case 'P':
		a.handleSendPR()
	}
}

// openDialog marks a dialog as open and returns a cleanup function.
// The cleanup MUST be called when the dialog closes (confirm OR cancel).
func (a *App) openDialog() func() {
	a.dialogOpen = true
	return func() {
		a.dialogOpen = false
		a.activeDialog = nil
		a.window.Canvas().Unfocus()
	}
}

// --- Navigation ---
//
// The linked worktree cards are in a grid (container.NewGridWrap).
// Up/Down jump by the number of columns (moving vertically in the grid).
// Left/Right move by 1 (moving horizontally within a row).
// This matches the TUI behavior (model.go:883-913).

// gridColumns returns the number of columns in the linked cards grid.
// Computed from the dashboard slot width and the card cell width, matching
// the formula used by flexGridLayout so keyboard navigation tracks the
// actual on-screen layout.
func (a *App) gridColumns() int {
	if a.dashSlot == nil {
		return 2 // safe default
	}
	w := a.dashSlot.Size().Width
	if w <= 0 {
		return 2
	}
	minW := cardCellSize().Width
	if w <= minW {
		return 1
	}
	padding := theme.Padding()
	cols := int(math.Floor(float64(w+padding) / float64(minW+padding)))
	if cols < 1 {
		cols = 1
	}
	return cols
}

func (a *App) navigateUp() {
	if a.focus == focusLeft {
		a.navigateRepoPanelUp()
		return
	}
	if a.dashboard == nil {
		return
	}
	if a.dashboard.state.ViewMode == ViewKanban {
		a.navigateKanbanUp()
		return
	}
	s := a.dashboard.state
	if s.SelectedCard == 0 {
		return // already at main
	}
	cols := a.gridColumns()
	linkedIdx := s.SelectedCard - 1 // 0-based within linked grid
	if linkedIdx-cols >= 0 {
		s.SelectedCard -= cols // move up one row
	} else {
		s.SelectedCard = 0 // first row → go to main
	}
	a.dashboard.Rebuild()
	a.dashboard.EnsureVisible()
}

func (a *App) navigateDown() {
	if a.focus == focusLeft {
		a.navigateRepoPanelDown()
		return
	}
	if a.dashboard == nil {
		return
	}
	if a.dashboard.state.ViewMode == ViewKanban {
		a.navigateKanbanDown()
		return
	}
	s := a.dashboard.state
	if s.SelectedCard == 0 {
		// From main → first linked card.
		if len(s.Worktrees) > 1 {
			s.SelectedCard = 1
			a.dashboard.Rebuild()
		}
		return
	}
	cols := a.gridColumns()
	last := len(s.Worktrees) - 1
	if s.SelectedCard+cols <= last {
		s.SelectedCard += cols // move down one row
	} else if s.SelectedCard < last {
		// No card directly below — jump to the last card in the next row.
		s.SelectedCard = last
	} else {
		return // already on last row
	}
	a.dashboard.Rebuild()
	a.dashboard.EnsureVisible()
}

func (a *App) navigateLeft() {
	if a.focus != focusRight || a.dashboard == nil {
		return
	}
	if a.dashboard.state.ViewMode == ViewKanban {
		a.navigateKanbanLeft()
		return
	}
	s := a.dashboard.state
	if s.SelectedCard > 1 {
		s.SelectedCard--
		a.dashboard.Rebuild()
		a.dashboard.EnsureVisible()
	} else if s.SelectedCard == 1 {
		s.SelectedCard = 0 // back to main
		a.dashboard.Rebuild()
	}
}

func (a *App) navigateRight() {
	if a.focus != focusRight || a.dashboard == nil {
		return
	}
	if a.dashboard.state.ViewMode == ViewKanban {
		a.navigateKanbanRight()
		return
	}
	s := a.dashboard.state
	if s.SelectedCard == 0 {
		if len(s.Worktrees) > 1 {
			s.SelectedCard = 1
			a.dashboard.Rebuild()
			a.dashboard.EnsureVisible()
		}
		return
	}
	if s.SelectedCard < len(s.Worktrees)-1 {
		s.SelectedCard++
		a.dashboard.Rebuild()
		a.dashboard.EnsureVisible()
	}
}

// --- View toggle ---

func (a *App) toggleView() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	if re.state.ViewMode == ViewKanban {
		re.state.ViewMode = ViewGrid
	} else {
		re.state.ViewMode = ViewKanban
	}
	if a.dashboard != nil {
		a.dashboard.Rebuild()
		if re.state.ViewMode == ViewGrid {
			a.dashboard.EnsureVisible()
		}
	}
}

// --- Kanban navigation ---

func (a *App) navigateKanbanUp() {
	s := a.dashboard.state
	if s.SelectedCard == 0 {
		return
	}
	stages := a.dashboard.KanbanStages()
	col := a.dashboard.KanbanColumnOf(s.SelectedCard)
	row := a.dashboard.KanbanRowOf(s.SelectedCard, stages)
	if row > 0 {
		s.SelectedCard = stages[col][row-1]
	} else {
		s.SelectedCard = 0 // top of column → back to main
	}
	a.dashboard.Rebuild()
}

func (a *App) navigateKanbanDown() {
	s := a.dashboard.state
	stages := a.dashboard.KanbanStages()
	if s.SelectedCard == 0 {
		// From main, enter the first non-empty column.
		for _, colCards := range stages {
			if len(colCards) > 0 {
				s.SelectedCard = colCards[0]
				a.dashboard.Rebuild()
				return
			}
		}
		return
	}
	col := a.dashboard.KanbanColumnOf(s.SelectedCard)
	row := a.dashboard.KanbanRowOf(s.SelectedCard, stages)
	if row < len(stages[col])-1 {
		s.SelectedCard = stages[col][row+1]
		a.dashboard.Rebuild()
	}
}

func (a *App) navigateKanbanLeft() {
	s := a.dashboard.state
	if s.SelectedCard <= 0 {
		return
	}
	stages := a.dashboard.KanbanStages()
	col := a.dashboard.KanbanColumnOf(s.SelectedCard)
	row := a.dashboard.KanbanRowOf(s.SelectedCard, stages)
	for c := col - 1; c >= 0; c-- {
		if len(stages[c]) > 0 {
			targetRow := row
			if targetRow >= len(stages[c]) {
				targetRow = len(stages[c]) - 1
			}
			s.SelectedCard = stages[c][targetRow]
			a.dashboard.Rebuild()
			return
		}
	}
}

func (a *App) navigateKanbanRight() {
	s := a.dashboard.state
	stages := a.dashboard.KanbanStages()
	if s.SelectedCard == 0 {
		// From main, enter the first non-empty column.
		for _, colCards := range stages {
			if len(colCards) > 0 {
				s.SelectedCard = colCards[0]
				a.dashboard.Rebuild()
				return
			}
		}
		return
	}
	col := a.dashboard.KanbanColumnOf(s.SelectedCard)
	row := a.dashboard.KanbanRowOf(s.SelectedCard, stages)
	for c := col + 1; c < 4; c++ {
		if len(stages[c]) > 0 {
			targetRow := row
			if targetRow >= len(stages[c]) {
				targetRow = len(stages[c]) - 1
			}
			s.SelectedCard = stages[c][targetRow]
			a.dashboard.Rebuild()
			return
		}
	}
}

func (a *App) toggleFocus() {
	if a.focus == focusRight {
		a.focus = focusLeft
	} else {
		a.focus = focusRight
	}
	if a.dashboard != nil {
		a.dashboard.Rebuild()
	}
}

func (a *App) handleEnter() {
	if a.focus == focusLeft {
		a.focus = focusRight
		if a.dashboard != nil {
			a.dashboard.Rebuild()
		}
		return
	}

	re := a.activeRepo()
	if re == nil {
		return
	}
	idx, ok := a.selectedWorktree()
	if !ok {
		return
	}
	wt := re.state.Worktrees[idx]

	mode := re.state.ActiveMode
	if mode != nil && mode.Type == "sandbox" && mode.SandboxName != "" {
		args := sandbox.RunWithBranchArgs(mode.SandboxName, wt.Branch)
		cmd := sandbox.CommandString(args)
		go func() { _ = ops.OpenTerminal("", cmd) }()
	} else {
		go func() { _ = ops.OpenTerminal(wt.Path, "") }()
	}
}

func (a *App) handleEscape() {
	if a.focus == focusLeft {
		a.focus = focusRight
		if a.dashboard != nil {
			a.dashboard.Rebuild()
		}
	}
}

func (a *App) navigateRepoPanelUp() {
	if len(a.repos) == 0 {
		return
	}
	re := a.repos[a.active]
	mi := re.group.ActiveMode
	if mi > 0 {
		a.switchMode(a.active, mi-1)
	} else if a.active > 0 {
		prev := a.repos[a.active-1].group
		a.switchMode(a.active-1, len(prev.Modes)-1)
	}
}

func (a *App) navigateRepoPanelDown() {
	if len(a.repos) == 0 {
		return
	}
	re := a.repos[a.active]
	mi := re.group.ActiveMode
	if mi < len(re.group.Modes)-1 {
		a.switchMode(a.active, mi+1)
	} else if a.active < len(a.repos)-1 {
		a.switchMode(a.active+1, 0)
	}
}

// --- Helpers ---

func (a *App) activeRepo() *repoEntry {
	if a.active < 0 || a.active >= len(a.repos) {
		return nil
	}
	return a.repos[a.active]
}

func (a *App) selectedWorktree() (int, bool) {
	re := a.activeRepo()
	if re == nil || re.state == nil {
		return -1, false
	}
	idx := re.state.SelectedCard
	if idx < 0 || idx >= len(re.state.Worktrees) {
		return -1, false
	}
	return idx, true
}

func (a *App) setStatus(msg string, isErr bool) {
	re := a.activeRepo()
	if re == nil {
		return
	}
	re.state.StatusMessage = msg
	re.state.StatusIsError = isErr
	if a.dashboard != nil {
		a.dashboard.Rebuild()
	}
}

// --- Worktree operations ---

func (a *App) handleRefresh() {
	if a.refreshMgr != nil {
		a.refreshMgr.TriggerNetwork()
	}
}

func (a *App) handleCreate() {
	re := a.activeRepo()
	if re == nil || re.state.SelectedCard != 0 {
		return
	}
	done := a.openDialog()
	a.activeDialog = showBranchInput(a.window, done, func(name string) {
		mode := re.state.ActiveMode
		go func() {
			var result ops.CreateWorktreeResult
			if mode != nil && mode.Type == "sandbox" && mode.SandboxName != "" {
				result = ops.CreateSandboxWorktree(mode.SandboxName, name)
			} else {
				result = ops.CreateWorktree(re.repo, name)
			}
			fyne.Do(func() {
				if result.Err != nil {
					a.setStatus(result.Err.Error(), true)
				}
				a.refreshMgr.TriggerQuick()
			})
		}()
	})
}

func (a *App) handleDeleteOrRemoveSandbox() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	idx := re.state.SelectedCard

	// Main card + sandbox mode → remove sandbox.
	if idx == 0 {
		mode := re.state.ActiveMode
		if mode == nil || mode.Type != "sandbox" || mode.SandboxName == "" {
			return
		}
		if re.state.SandboxStatus == sandbox.StatusNotFound {
			return
		}
		done := a.openDialog()
		a.activeDialog = showConfirmRemoveSandbox(a.window, mode.SandboxName, done, func() {
			go func() {
				result := ops.RemoveSandbox(mode.SandboxName)
				fyne.Do(func() {
					if result.Err != nil {
						a.setStatus(result.Err.Error(), true)
					}
					a.refreshMgr.TriggerLocal()
				})
			}()
		})
		return
	}

	// Linked card → delete worktree.
	if idx >= len(re.state.Worktrees) {
		return
	}
	wt := re.state.Worktrees[idx]
	done := a.openDialog()
	a.activeDialog = showConfirmDelete(a.window, wt.Branch, done, func() {
		go func() {
			err := ops.RemoveWorktree(re.repo, wt.Branch)
			fyne.Do(func() {
				if err != nil {
					a.setStatus(err.Error(), true)
				} else if re.state.SelectedCard >= len(re.state.Worktrees)-1 {
					re.state.SelectedCard = max(0, re.state.SelectedCard-1)
				}
				a.refreshMgr.TriggerQuick()
			})
		}()
	})
}

func (a *App) handleFetchPR() {
	re := a.activeRepo()
	if re == nil || re.state.SelectedCard != 0 {
		return
	}
	done := a.openDialog()
	a.activeDialog = showFetchPRInput(a.window, done, func(input string) {
		mode := re.state.ActiveMode
		go func() {
			var result ops.FetchPRResult
			if mode != nil && mode.Type == "sandbox" && mode.SandboxName != "" {
				result = ops.FetchPRSandbox(re.repo, input, mode.SandboxName)
			} else {
				result = ops.FetchPR(re.repo, input)
			}
			fyne.Do(func() {
				if result.Err != nil {
					a.setStatus(result.Err.Error(), true)
				}
				a.refreshMgr.TriggerQuick()
			})
		}()
	})
}

func (a *App) handlePull() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	a.setStatus("Pulling...", false)
	go func() {
		err := ops.Pull(re.repo)
		fyne.Do(func() {
			if err != nil {
				a.setStatus("Pull failed: "+err.Error(), true)
			} else {
				a.setStatus("Pull complete", false)
			}
			a.refreshMgr.TriggerLocal()
		})
	}()
}

func (a *App) handleOpenEditor() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	idx, ok := a.selectedWorktree()
	if !ok {
		return
	}
	wt := re.state.Worktrees[idx]
	go func() {
		err := ops.OpenEditor(wt.Path)
		if err != nil {
			fyne.Do(func() {
				a.setStatus("Editor failed: "+err.Error(), true)
			})
		}
	}()
}

// --- Send PR (multi-phase) ---

func (a *App) handleSendPR() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	idx := re.state.SelectedCard
	if idx <= 0 || idx >= len(re.state.Worktrees) {
		return
	}
	wt := re.state.Worktrees[idx]

	if wt.Detached {
		a.setStatus("Cannot create PR from detached HEAD", true)
		return
	}
	if re.state.CLIAvail != provider.CLIAvailable {
		a.setStatus("CLI tool required for PR creation", true)
		return
	}
	if re.state.Provider == provider.ProviderUnknown {
		a.setStatus("Unsupported git provider for PR creation", true)
		return
	}

	remotes, err := re.repo.ListRemotes()
	if err != nil || len(remotes) == 0 {
		a.setStatus("No remotes configured", true)
		return
	}

	hasStash, _ := re.repo.HasStash()
	needsWarning := wt.IsDirty || hasStash

	var existingPR *provider.PRInfo
	if re.state.PRs != nil {
		existingPR = re.state.PRs[wt.Branch]
	}

	done := a.openDialog()
	a.sendPRFlow(re, wt.Branch, remotes, needsWarning, wt.IsDirty, hasStash, existingPR, done)
}

func (a *App) sendPRFlow(re *repoEntry, branch string, remotes []git.RemoteInfo, needsWarning, dirty, hasStash bool, existingPR *provider.PRInfo, done func()) {
	if needsWarning {
		a.activeDialog = showSendPRDirtyWarning(a.window, branch, dirty, hasStash, done, func() {
			a.sendPRSelectRemote(re, branch, remotes, existingPR, done)
		})
		return
	}
	a.sendPRSelectRemote(re, branch, remotes, existingPR, done)
}

func (a *App) sendPRSelectRemote(re *repoEntry, branch string, remotes []git.RemoteInfo, existingPR *provider.PRInfo, done func()) {
	if len(remotes) > 1 {
		a.activeDialog = showSendPRRemoteSelection(a.window, remotes, done, func(idx int) {
			a.sendPRConfirm(re, branch, remotes[idx], existingPR, done)
		})
		return
	}
	a.sendPRConfirm(re, branch, remotes[0], existingPR, done)
}

func (a *App) sendPRConfirm(re *repoEntry, branch string, remote git.RemoteInfo, existingPR *provider.PRInfo, done func()) {
	a.activeDialog = showSendPRConfirm(a.window, branch, remote, existingPR, done, func() {
		pushOnly := existingPR != nil
		if pushOnly {
			a.setStatus("Pushing...", false)
		} else {
			a.setStatus("Pushing and creating PR...", false)
		}
		go func() {
			if pushOnly {
				err := ops.PushBranch(re.repo, branch, remote)
				fyne.Do(func() {
					if err != nil {
						a.setStatus("Push failed: "+err.Error(), true)
					} else {
						a.setStatus("Pushed successfully", false)
					}
					a.refreshMgr.TriggerNetwork()
				})
			} else {
				result := ops.SendPR(re.repo, re.prProv, branch, remote)
				fyne.Do(func() {
					if result.Err != nil {
						a.setStatus("PR creation failed: "+result.Err.Error(), true)
					} else {
						a.setStatus("PR created: "+result.URL, false)
					}
					a.refreshMgr.TriggerNetwork()
				})
			}
		}()
	})
}

// --- Sandbox operations ---

func (a *App) handleStartSandbox() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	mode := re.state.ActiveMode
	if mode == nil || mode.Type != "sandbox" || mode.SandboxName == "" || re.state.SandboxStatus != sandbox.StatusStopped {
		return
	}
	go func() {
		result := ops.StartSandbox(mode.SandboxName)
		fyne.Do(func() {
			if result.Err != nil {
				a.setStatus(result.Err.Error(), true)
			}
			a.refreshMgr.TriggerLocal()
		})
	}()
}

func (a *App) handleStopSandbox() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	mode := re.state.ActiveMode
	if mode == nil || mode.Type != "sandbox" || mode.SandboxName == "" || re.state.SandboxStatus != sandbox.StatusRunning {
		return
	}
	go func() {
		result := ops.StopSandbox(mode.SandboxName)
		fyne.Do(func() {
			if result.Err != nil {
				a.setStatus(result.Err.Error(), true)
			}
			a.refreshMgr.TriggerLocal()
		})
	}()
}

func (a *App) handleCreateOrEnrollSandbox() {
	re := a.activeRepo()
	if re == nil || re.state.SelectedCard != 0 {
		return
	}
	mode := re.state.ActiveMode

	// Sandbox mode + not found → create sandbox.
	if mode != nil && mode.Type == "sandbox" && mode.SandboxName != "" {
		if re.state.SandboxStatus != sandbox.StatusNotFound {
			return
		}
		done := a.openDialog()
		a.activeDialog = showConfirmCreateSandbox(a.window, mode.SandboxName, mode.Agent, re.group.Path, done, func() {
			a.doCreateSandboxWithPreflight(re, mode.SandboxName, mode.Agent, re.group.Path)
		})
		return
	}

	// Non-sandbox → enroll by asking for agent name.
	done := a.openDialog()
	showAgentInput(a.window, done, func(agentName string) {
		sbxName := sandbox.SanitizeName(re.group.Name, agentName)
		newMode := config.ModeEntry{Type: "sandbox", SandboxName: sbxName, Agent: agentName}
		go func() {
			err := sandbox.Preflight()
			fyne.Do(func() {
				if err != nil {
					a.setStatus("Sandbox not ready — run: sbx ls", true)
					return
				}
				cfg, _ := config.Load(a.configPath)
				if cfg.Add(re.group.Path, re.group.Name, newMode) {
					_ = config.Save(a.configPath, cfg)
				}
				re.group.Modes = append(re.group.Modes, newMode)
				re.state.ActiveMode = &newMode
				re.group.ActiveMode = len(re.group.Modes) - 1
				re.refreshMgr.SetSandboxCandidates(
					sandbox.Candidates(sbxName, re.repo.RepoName(), re.repo.Root(), agentName),
				)
				if a.repoPanel != nil {
					a.repoPanel.groups = a.collectGroups()
					a.repoPanel.SetActive(a.active, re.group.ActiveMode)
					a.repoPanel.rebuildList()
				}
				a.refreshMgr.TriggerLocal()
				a.dashboard.Rebuild()
			})
		}()
	})
}

func (a *App) handleAddSandboxMode() {
	re := a.activeRepo()
	if re == nil {
		return
	}
	done := a.openDialog()
	showAgentInput(a.window, done, func(agentName string) {
		sbxName := sandbox.SanitizeName(re.group.Name, agentName)
		newMode := config.ModeEntry{Type: "sandbox", SandboxName: sbxName, Agent: agentName}
		go func() {
			err := sandbox.Preflight()
			fyne.Do(func() {
				if err != nil {
					a.setStatus("Sandbox not ready — run: sbx ls", true)
					return
				}
				cfg, _ := config.Load(a.configPath)
				if cfg.Add(re.group.Path, re.group.Name, newMode) {
					_ = config.Save(a.configPath, cfg)
				}
				re.group.Modes = append(re.group.Modes, newMode)
				if a.repoPanel != nil {
					a.repoPanel.groups = a.collectGroups()
					a.repoPanel.rebuildList()
				}
				a.switchMode(a.active, len(re.group.Modes)-1)
			})
		}()
	})
}

func (a *App) doCreateSandboxWithPreflight(re *repoEntry, sbxName, sbxAgent, repoPath string) {
	go func() {
		err := sandbox.Preflight()
		if err != nil {
			fyne.Do(func() { a.setStatus("Sandbox not ready — run: sbx ls", true) })
			return
		}
		args := sandbox.CreateArgs(sbxName, sbxAgent, repoPath)
		result := ops.CreateSandbox(args)
		fyne.Do(func() {
			if result.Err != nil {
				a.setStatus(result.Err.Error(), true)
			}
			a.refreshMgr.TriggerLocal()
		})
	}()
}

// --- Config management ---

func (a *App) handleAddRepo() {
	done := a.openDialog()
	showAddRepoInput(a.window, done, func(path string) {
		repoRoot, err := git.RepoRoot(path)
		if err != nil {
			dialog.ShowError(err, a.window)
			return
		}
		repo, err := git.OpenRepository(repoRoot)
		if err != nil {
			dialog.ShowError(err, a.window)
			return
		}

		done2 := a.openDialog()
		showModeSelection(a.window, done2, func() {
			// Regular mode.
			a.addRepoToConfig(repoRoot, repo.RepoName(), config.ModeEntry{Type: "regular"})
		}, func() {
			// Sandbox mode.
			done3 := a.openDialog()
			showAgentInput(a.window, done3, func(agentName string) {
				sbxName := sandbox.SanitizeName(repo.RepoName(), agentName)
				mode := config.ModeEntry{Type: "sandbox", SandboxName: sbxName, Agent: agentName}
				go func() {
					err := sandbox.Preflight()
					fyne.Do(func() {
						if err != nil {
							a.setStatus("Sandbox not ready — run: sbx ls", true)
							return
						}
						a.addRepoToConfig(repoRoot, repo.RepoName(), mode)
					})
				}()
			})
		})
	})
}

func (a *App) addRepoToConfig(repoRoot, repoName string, mode config.ModeEntry) {
	cfg, _ := config.Load(a.configPath)
	if !cfg.Add(repoRoot, repoName, mode) {
		dialog.ShowInformation("Repository Added", repoName+" is already registered with this mode.", a.window)
		return
	}
	_ = config.Save(a.configPath, cfg)

	// Mirror the persisted entry so in-memory modes match config semantics
	// (e.g., sandbox mode replacing a prior regular mode).
	idx := cfg.IndexOf(repoRoot)
	if idx < 0 {
		return
	}
	persisted := cfg.Repos[idx]

	// Case 1: repo is already in the UI — just sync its modes and switch to
	// the newly added one.
	for gi, re := range a.repos {
		if re.group.Path == repoRoot {
			re.group.Modes = persisted.Modes
			if a.repoPanel != nil {
				a.repoPanel.groups = a.collectGroups()
				a.repoPanel.rebuildList()
			}
			newModeIdx := len(persisted.Modes) - 1
			a.switchMode(gi, newModeIdx)
			return
		}
	}

	// Case 2: brand new repo — build an entry and append it.
	re := a.buildRepoEntry(persisted)
	if re == nil {
		a.setStatus("failed to open "+repoName, true)
		return
	}

	wasEmpty := len(a.repos) == 0
	a.repos = append(a.repos, re)

	// Transitioning from empty state requires a full layout swap because
	// repoPanel, dashboard, title bar, and dashSlot don't exist yet.
	if wasEmpty {
		a.window.SetContent(a.buildMainLayout())
		return
	}

	if a.repoPanel != nil {
		a.repoPanel.groups = a.collectGroups()
		a.repoPanel.rebuildList()
	}
	a.switchMode(len(a.repos)-1, 0)
}

func (a *App) handleRemoveMode() {
	if a.active < 0 || a.active >= len(a.repos) {
		return
	}
	re := a.repos[a.active]
	if re.group == nil || len(re.group.Modes) == 0 {
		return
	}
	mi := re.group.ActiveMode
	if mi < 0 || mi >= len(re.group.Modes) {
		return
	}
	mode := re.group.Modes[mi]

	modeLabel := "host"
	if mode.Agent != "" {
		modeLabel = mode.Agent
	}

	done := a.openDialog()
	a.activeDialog = showConfirmRemoveMode(a.window, re.group.Name, modeLabel, mode.Type == "sandbox", done, func() {
		cfg, _ := config.Load(a.configPath)
		cfg.RemoveMode(re.group.Path, mode)

		// DL-019: if last sandbox removed, convert to regular.
		if mode.Type == "sandbox" && cfg.IndexOf(re.group.Path) < 0 {
			cfg.Add(re.group.Path, re.group.Name, config.ModeEntry{Type: "regular"})
		}
		_ = config.Save(a.configPath, cfg)

		dialog.ShowInformation("Mode Removed", modeLabel+" removed.\nRestart biomelab to update.", a.window)
	})
}

func (a *App) collectGroups() []*RepoGroup {
	groups := make([]*RepoGroup, len(a.repos))
	for i, re := range a.repos {
		groups[i] = re.group
	}
	return groups
}
