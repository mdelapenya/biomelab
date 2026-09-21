package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

type terminalTarget struct {
	repo                       *repoEntry
	path, mode, sandbox, agent string
}

type terminalDependencies struct {
	open     func(string, string, string) (*terminal.Session, error)
	find     func([]string, string, []*terminal.Session) (*terminal.Session, error)
	activate func(*terminal.Session) (bool, error)
	dispatch func(func())
}

func targetForTerminal(re *repoEntry, path string) terminalTarget {
	target := terminalTarget{repo: re, path: path, mode: "regular"}
	if mode := re.state.ActiveMode; mode != nil {
		target.mode, target.sandbox, target.agent = mode.Type, mode.SandboxName, mode.Agent
	}
	return target
}

// openOrActivateTerminal is called only by a deliberate terminal action. All
// controller state and status updates belong to the UI thread; workers get copies.
func (a *App) openOrActivateTerminal(re *repoEntry, wt git.Worktree) {
	if a.terminalDeps == nil {
		a.terminalDeps = &terminalDependencies{
			open: terminal.OpenTracked, find: terminal.FindSession,
			activate: func(s *terminal.Session) (bool, error) { return s.Activate() }, dispatch: fyne.Do,
		}
	}
	deps := *a.terminalDeps
	target := targetForTerminal(re, wt.Path)
	for key, pending := range a.terminalActions {
		if pending && key.repo == target.repo && key.path == target.path {
			return
		}
	}
	if a.terminalActions == nil {
		a.terminalActions = make(map[terminalTarget]bool)
	}
	if a.terminalSessions == nil {
		a.terminalSessions = make(map[terminalTarget]*terminal.Session)
	}
	for key, session := range a.terminalSessions {
		if !a.hasTerminalTarget(key) {
			session.Cleanup()
			delete(a.terminalSessions, key)
		}
	}
	a.terminalActions[target] = true
	existing := a.terminalSessions[target]
	var claimed []*terminal.Session
	for key, session := range a.terminalSessions {
		if key != target {
			claimed = append(claimed, session)
		}
	}
	var paths []string
	for _, entry := range a.repos {
		for _, tree := range entry.state.Worktrees {
			paths = append(paths, tree.Path)
		}
	}
	isSandbox := target.mode == "sandbox" && target.sandbox != ""
	dir, command, identifier := wt.Path, "", wt.Branch
	if isSandbox {
		args := sandbox.RunAttachArgs(target.sandbox)
		if !wt.IsMain {
			args = sandbox.ExecAgentArgs(target.sandbox, wt.Path, target.agent)
		}
		dir, command, identifier = "", sandbox.CommandString(args), ""
	}
	go func() {
		session, err := reuseOrOpenTerminal(deps, existing, claimed, paths, wt.Path, isSandbox, dir, command, identifier)
		deps.dispatch(func() {
			delete(a.terminalActions, target)
			if !a.hasTerminalTarget(target) {
				session.Cleanup()
				return
			}
			if session == nil {
				delete(a.terminalSessions, target)
			} else {
				a.terminalSessions[target] = session
			}
			if targetForTerminal(re, wt.Path) != target {
				return
			}
			if err == nil && re.terminalError != "" && re.terminalErrorTarget == target {
				if re.state.StatusIsError && re.state.StatusMessage == re.terminalError {
					re.state.StatusMessage, re.state.StatusIsError = "", false
					if a.activeRepo() == re && re.dashboard != nil {
						re.dashboard.Rebuild()
					}
				}
				re.terminalError = ""
			}
			if err != nil {
				re.terminalErrorTarget, re.terminalError = target, err.Error()
				re.state.StatusMessage = err.Error()
				re.state.StatusIsError = true
				if a.activeRepo() == re && re.dashboard != nil {
					re.dashboard.Rebuild()
				}
				if session.RecoveryNeeded() && a.activeRepo() == re && !a.dialogOpen && a.window != nil {
					a.offerTerminalRecovery(target, session)
				}
			}
		})
	}()
}

func (a *App) offerTerminalRecovery(target terminalTarget, session *terminal.Session) {
	cleanup := a.openDialog()
	dlg := dialog.NewConfirm("Terminal did not register",
		"Check whether the terminal opened and close it before forgetting this session. Forgetting lets the next Enter try again and can create a duplicate if the old terminal is still open.",
		func(forget bool) {
			cleanup()
			if forget && a.terminalSessions[target] == session && !a.terminalActions[target] {
				delete(a.terminalSessions, target)
				session.Cleanup()
			}
		}, a.window)
	dlg.SetConfirmText("Forget session")
	dlg.SetDismissText("Keep waiting")
	a.activeDialog = dlg
	dlg.Show()
}

func reuseOrOpenTerminal(deps terminalDependencies, existing *terminal.Session, claimed []*terminal.Session, paths []string, path string, sandboxMode bool, dir, command, identifier string) (*terminal.Session, error) {
	if existing != nil {
		alive, err := deps.activate(existing)
		if alive || err != nil {
			return existing, err
		}
		existing.Cleanup()
	}
	if !sandboxMode {
		found, err := deps.find(paths, path, claimed)
		if err != nil {
			return nil, err
		}
		if found != nil {
			alive, err := deps.activate(found)
			if alive || err != nil {
				return found, err
			}
			found.Cleanup()
		}
	}
	return deps.open(dir, command, identifier)
}

func (a *App) hasTerminalTarget(target terminalTarget) bool {
	for _, re := range a.repos {
		if re != target.repo {
			continue
		}
		if re.group != nil {
			validMode := false
			for _, mode := range re.group.Modes {
				if mode.Type == target.mode && mode.SandboxName == target.sandbox && mode.Agent == target.agent {
					validMode = true
					break
				}
			}
			if !validMode {
				return false
			}
		}
		for _, wt := range re.state.Worktrees {
			if wt.Path == target.path {
				return true
			}
		}
	}
	return false
}
