package gui

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/kits"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// beginSandboxSetup owns the complete project-panel creation flow. No mode is
// saved until the sandbox has been created (or an existing one discovered).
func (a *App) beginSandboxSetup(repoPath, repoName string) {
	if !a.requireSbxInstalled() {
		return
	}
	done := a.openDialog()
	a.activeDialog = showAgentInput(a.window, done, func(agent string, addKits bool) {
		mode := config.ModeEntry{
			Type: "sandbox", Agent: agent,
			SandboxName: sandbox.SanitizeName(repoName, agent),
		}
		// Use a previously reconciled name when retrying an enrolled mode.
		for _, re := range a.repos {
			if re.group.Path == repoPath {
				for _, existing := range re.group.Modes {
					if existing.Type == "sandbox" && existing.Agent == agent {
						mode = existing
						break
					}
				}
			}
		}
		confirm := func(selected []kits.Kit) {
			confirmDone := a.openDialog()
			a.activeDialog = showConfirmCreateSandbox(a.window, mode.SandboxName, agent, repoPath, kitURLs(selected), confirmDone, func() {
				a.createProjectSandbox(repoPath, repoName, mode, selected)
			})
		}
		if addKits {
			a.loadSetupKits(mode, confirm)
		} else {
			confirm(nil)
		}
	})
}

// loadSetupKits is reached only after an explicit Yes. The loading dialog
// keeps callbacks from opening a picker over another workflow; cancellation
// cancels the catalog requests and suppresses their eventual completion.
func (a *App) loadSetupKits(mode config.ModeEntry, onSubmit func([]kits.Kit)) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	done := a.openDialog()
	loading := dialog.NewCustom("Loading Kits", "Cancel", widget.NewLabel("Loading available kits…"), a.window)
	loading.SetOnClosed(func() {
		cancel()
		done()
	})
	a.activeDialog = loading
	loading.Show()
	go func() {
		// The agent was chosen already; only mixins can be layered onto it.
		_, mixins, err := kits.FetchAvailable(ctx)
		fyne.Do(func() {
			if ctx.Err() == context.Canceled {
				return
			}
			loading.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("load kits: %w", err), a.window)
				return
			}
			pickerDone := a.openDialog()
			a.activeDialog = showKitsDialog(a.window, mode.SandboxName, mode.Agent, nil,
				kits.FilterMixinsForAgent(mixins, mode.Agent), pickerDone, onSubmit)
		})
	}()
}

func (a *App) createProjectSandbox(repoPath, repoName string, mode config.ModeEntry, selected []kits.Kit) {
	if a.creatingSandboxes == nil {
		a.creatingSandboxes = make(map[string]bool)
	}
	if a.creatingSandboxes[mode.SandboxName] {
		dialog.ShowInformation("Creating Sandbox", mode.SandboxName+" is already being created.", a.window)
		return
	}
	a.creatingSandboxes[mode.SandboxName] = true
	a.setProjectStatus(repoPath, "Creating "+mode.SandboxName+"…", false)
	go func() {
		name, created, err := ops.EnsureSandbox(repoName, repoPath, mode.SandboxName, mode.Agent, kitURLs(selected))
		fyne.Do(func() {
			delete(a.creatingSandboxes, mode.SandboxName)
			if err != nil {
				a.setProjectStatus(repoPath, ops.FirstNonEmptyLine(err.Error()), true)
				dialog.ShowError(err, a.window)
				return
			}
			mode.SandboxName = name
			if created {
				// A non-nil empty list clears metadata from a deleted sandbox.
				mode.Kits = buildKitInstalls(selected)
			}
			if !a.addRepoToConfig(repoPath, repoName, mode) {
				a.setProjectStatus(repoPath, "Sandbox "+name+" exists, but its registration failed. Retry to register it.", true)
				return
			}
			message := "Using existing sandbox " + name
			if created {
				message = "Created " + name
			}
			a.setProjectStatus(repoPath, message, false)
			for _, re := range a.repos {
				if re.group.Path == repoPath {
					re.refreshMgr.TriggerLocal()
					break
				}
			}
		})
	}()
}

// A background operation's status belongs to its original project even if
// the user navigates elsewhere while the CLI is running.
func (a *App) setProjectStatus(repoPath, message string, isError bool) {
	for _, re := range a.repos {
		if re.group.Path == repoPath {
			re.state.StatusMessage = message
			re.state.StatusIsError = isError
			re.dashboard.Rebuild()
			return
		}
	}
}
