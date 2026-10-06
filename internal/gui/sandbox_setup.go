package gui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
			chosenAgent := sandboxAgentForSelection(agent, selected)
			mode := newSandboxMode(repoPath, chosenAgent)
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

func sandboxAgentForSelection(defaultAgent string, selected []kits.Kit) string {
	for _, k := range selected {
		if k.Kind == kits.KindSandbox {
			return k.Name
		}
	}
	return defaultAgent
}

func newSandboxMode(repoPath, agent string) config.ModeEntry {
	return config.ModeEntry{Type: "sandbox", Agent: agent, SandboxName: sandbox.GeneratedName(repoPath, agent)}
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
				var existing *ops.ExistingSandboxWithKitsError
				if errors.As(err, &existing) {
					// The selected kits cannot be applied to this sandbox. The
					// user must explicitly choose registration without them.
					a.setProjectStatus(repoPath, "Existing sandbox "+existing.Name+" needs registration confirmation", false)
					done := a.openDialog()
					a.activeDialog = showConfirmRegisterExistingSandbox(a.window, existing.Name, repoPath, done, func() {
						a.registerExistingProjectSandbox(repoPath, repoName, mode, existing.Name)
					}, func() {
						a.setProjectStatus(repoPath, "Sandbox registration canceled", false)
					})
					return
				}
				a.setProjectStatus(repoPath, ops.FirstNonEmptyLine(err.Error()), true)
				dialog.ShowError(err, a.window)
				return
			}
			a.completeProjectSandbox(repoPath, repoName, mode, name, created, selected)
		})
	}()
}

// showConfirmRegisterExistingSandbox is a separate modal decision because the
// original Create confirmation requested kit installation. Hiding or canceling
// it closes modal ownership without changing the repository configuration.
func showConfirmRegisterExistingSandbox(parent fyne.Window, name, repoPath string, onDone, onRegister, onCancel func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	keyCap := newDialogKeyCapture(func() { d.Confirm() }, func() { d.Hide() })
	content := container.NewStack(widget.NewLabel(
		"Sandbox "+name+" already exists for "+repoPath+". Register it without changing its kits?\n\nSelected kits will not be installed or recorded."), keyCap)
	d = dialog.NewCustomConfirm("Register Existing Sandbox", "Register Existing", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onRegister()
		} else {
			onCancel()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func (a *App) registerExistingProjectSandbox(repoPath, repoName string, mode config.ModeEntry, matchedName string) {
	if a.creatingSandboxes == nil {
		a.creatingSandboxes = make(map[string]bool)
	}
	if a.creatingSandboxes[matchedName] {
		dialog.ShowInformation("Registering Sandbox", matchedName+" is already being registered.", a.window)
		return
	}
	a.creatingSandboxes[matchedName] = true
	a.setProjectStatus(repoPath, "Registering existing sandbox "+matchedName+"…", false)
	go func() {
		name, err := ops.RegisterExistingSandbox(matchedName)
		fyne.Do(func() {
			delete(a.creatingSandboxes, matchedName)
			if err != nil {
				a.setProjectStatus(repoPath, ops.FirstNonEmptyLine(err.Error()), true)
				dialog.ShowError(err, a.window)
				return
			}
			a.completeProjectSandbox(repoPath, repoName, mode, name, false, nil)
		})
	}()
}

func (a *App) completeProjectSandbox(repoPath, repoName string, mode config.ModeEntry, name string, created bool, selected []kits.Kit) {
	mode = registeredSandboxMode(mode, name, created, selected)
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
}

func registeredSandboxMode(mode config.ModeEntry, name string, created bool, selected []kits.Kit) config.ModeEntry {
	mode.SandboxName = name
	if created {
		// Only a new creation proves that the selected kits were installed.
		// On reuse, retain only metadata already present in a trusted mode.
		mode.Kits = buildKitInstalls(selected)
	}
	return mode
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
