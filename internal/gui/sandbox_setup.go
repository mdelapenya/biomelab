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
		confirm := func(selected []kits.Kit) {
			chosenAgent := agent
			for _, k := range selected {
				if k.Kind == kits.KindSandbox {
					chosenAgent = k.Name
					break
				}
			}
			mode := config.ModeEntry{Type: "sandbox", Agent: chosenAgent,
				SandboxName: sandbox.SanitizeName(repoName, chosenAgent)}
			// Preserve a reconciled name when registering an existing mode.
			for _, re := range a.repos {
				if re.group.Path == repoPath {
					for _, existing := range re.group.Modes {
						if existing.Type == "sandbox" && existing.Agent == chosenAgent {
							mode = existing
							break
						}
					}
				}
			}
			baseRef, mixinRefs := kitSelectionRefs(selected)
			confirmDone := a.openDialog()
			a.activeDialog = showConfirmCreateSandboxWithKit(a.window, mode.SandboxName, chosenAgent, baseRef, repoPath, mixinRefs, confirmDone, func() {
				a.createProjectSandbox(repoPath, repoName, mode, selected)
			})
		}
		if addKits {
			a.loadSetupKits(repoName, confirm)
		} else {
			confirm(nil)
		}
	})
}

// loadSetupKits is reached only after an explicit Yes. The loading dialog
// keeps callbacks from opening a picker over another workflow; cancellation
// cancels the catalog requests and suppresses their eventual completion.
func (a *App) loadSetupKits(repoName string, onSubmit func([]kits.Kit)) {
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
		sandboxKits, mixins, err := kits.FetchAvailable(ctx)
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
			a.activeDialog = showKitsDialog(a.window, repoName, sandboxKits, mixins, pickerDone, onSubmit)
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
		baseRef, mixinRefs := kitSelectionRefs(selected)
		name, created, err := ops.EnsureSandboxWithKit(repoName, repoPath, mode.SandboxName, mode.Agent, baseRef, mixinRefs)
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
