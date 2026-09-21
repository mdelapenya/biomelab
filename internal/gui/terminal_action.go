package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

type terminalTarget struct {
	repo                       *repoEntry
	path, mode, sandbox, agent string
}

type terminalDependencies struct {
	open        func(string, string, string) error
	isCurrent   func(terminal.Info, string) bool
	activatePID func(int32, int32, terminal.Kind) bool
	activateApp func(terminal.Kind) bool
	dispatch    func(func())
}

func targetForTerminal(re *repoEntry, path string) terminalTarget {
	target := terminalTarget{repo: re, path: path}
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
			open: ops.OpenTerminal, isCurrent: terminal.SessionExists,
			activatePID: ops.ActivateTerminalByPID, activateApp: ops.ActivateTerminalApp, dispatch: fyne.Do,
		}
	}
	deps := *a.terminalDeps
	target := targetForTerminal(re, wt.Path)
	isSandbox := target.mode == "sandbox" && target.sandbox != ""
	var existing *terminal.Info
	if terms := re.state.Terminals[wt.Path]; !isSandbox && len(terms) > 0 {
		term := terms[0]
		existing = &term
	}
	now := time.Now()
	for key, until := range a.terminalActions {
		if !until.IsZero() && !now.Before(until) {
			delete(a.terminalActions, key)
		}
	}
	if until, pending := a.terminalActions[target]; pending && (until.IsZero() || existing == nil) {
		return
	}
	if a.terminalActions == nil {
		a.terminalActions = make(map[terminalTarget]time.Time)
	}
	a.terminalActions[target] = time.Time{} // zero means work is still running
	dir, command, identifier := wt.Path, "", wt.Branch
	if isSandbox {
		args := sandbox.RunAttachArgs(target.sandbox)
		if !wt.IsMain {
			args = sandbox.ExecAgentArgs(target.sandbox, wt.Path, target.agent)
		}
		dir, command, identifier = "", sandbox.CommandString(args), ""
	}
	go func() {
		var err error
		if existing != nil {
			if !deps.isCurrent(*existing, wt.Path) {
				err = fmt.Errorf("terminal session changed; refresh the worktree and try again")
			} else if !deps.activatePID(existing.ShellPID, existing.RootPID, existing.Kind) && !deps.activateApp(existing.Kind) {
				err = fmt.Errorf("could not activate %s; switch to the terminal manually", existing.Kind)
			}
		} else {
			err = deps.open(dir, command, identifier)
		}
		deps.dispatch(func() {
			delete(a.terminalActions, target)
			if !a.hasTerminalTarget(target) {
				return
			}
			if err == nil && existing == nil {
				// Allow the next process-detection tick to observe the new shell
				// before accepting another open request. No automatic retry.
				a.terminalActions[target] = time.Now().Add(localRefreshInterval)
			}
			if err != nil && targetForTerminal(re, wt.Path) == target {
				re.state.StatusMessage = err.Error()
				re.state.StatusIsError = true
				if a.activeRepo() == re && re.dashboard != nil {
					re.dashboard.Rebuild()
				}
			}
		})
	}()
}

func (a *App) hasTerminalTarget(target terminalTarget) bool {
	for _, re := range a.repos {
		if re != target.repo || targetForTerminal(re, target.path) != target {
			continue
		}
		for _, wt := range re.state.Worktrees {
			if wt.Path == target.path {
				return true
			}
		}
	}
	return false
}
