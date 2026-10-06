package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

var dialogMinSize = fyne.NewSize(450, 200)
var kitsDialogSize = fyne.NewSize(780, 640)

// Fyne removes every overlay above a dismissed older popup. A still-open
// message can re-show itself through the normal dialog lifecycle without
// treating this framework removal as the user acknowledging the message.
type messageDialog struct {
	dialog.Dialog
	parent            fyne.Window
	overlay           fyne.CanvasObject
	previous          *messageDialog
	closed, restoring bool
	restoreFocus      func()
}

func (m *messageDialog) restoreOverlay() {
	if m.closed {
		return
	}
	if m.previous != nil {
		m.previous.restoreOverlay()
	}
	for _, overlay := range m.parent.Canvas().Overlays().List() {
		if overlay == m.overlay {
			return
		}
	}
	m.restoring = true
	m.Hide()
	m.Show()
	m.restoring = false
	if m.restoreFocus != nil {
		m.restoreFocus()
	}
}

// Messages participate in modal ownership even when an operation finishes
// while another dialog is open. The existing dialog stays underneath; closing
// this message restores it only while its overlay is still present.
func (a *App) showError(err error) {
	if err != nil {
		a.showWindowError(a.window, err)
	}
}

func (a *App) showInformation(title, message string) {
	a.showWindowInformation(a.window, title, message)
}

func (a *App) showWindowError(parent fyne.Window, err error) {
	if err != nil {
		a.showWindowInformation(parent, "Error", err.Error())
	}
}

func (a *App) showWindowInformation(parent fyne.Window, title, message string) {
	if parent == nil {
		return
	}
	previous, previousOpen := a.activeDialog, a.dialogOpen
	previousGeneration := a.dialogGeneration
	previousOverlay := parent.Canvas().Overlays().Top()
	previousFocus := parent.Canvas().Focused()
	tracked := parent == a.window
	var d dialog.Dialog
	closeButton := newDialogButton("Close", func() { d.Hide() }, func() { d.Hide() })
	closeButton.Importance = widget.HighImportance
	body := container.NewVScroll(dialogSection(dialogText(message)))
	d = dialog.NewCustomWithoutButtons(title, container.NewBorder(nil, dialogFooter(closeButton), nil, nil, body), parent)
	owned := &messageDialog{Dialog: d, parent: parent}
	if tracked {
		owned.previous, _ = previous.(*messageDialog)
	}
	d.SetOnClosed(func() {
		if owned.restoring {
			return
		}
		owned.closed = true
		if !tracked || a.activeDialog != owned {
			if tracked {
				if newer, ok := a.activeDialog.(*messageDialog); ok {
					newer.restoreOverlay()
				}
			}
			return
		}
		a.activeDialog, a.dialogOpen = nil, false
		for _, overlay := range parent.Canvas().Overlays().List() {
			if overlay == previousOverlay && previous != nil {
				a.activeDialog, a.dialogOpen = previous, previousOpen
				a.dialogGeneration = previousGeneration
				break
			}
		}
		parent.Canvas().Unfocus()
		if a.dialogOpen && previousFocus != nil {
			parent.Canvas().Focus(previousFocus)
		}
	})
	if tracked {
		a.dialogGeneration++
		a.activeDialog, a.dialogOpen = owned, true
	}
	d.Resize(boundedDialogSize(parent, fyne.NewSize(480, 240)))
	d.Show()
	messageOverlay := parent.Canvas().Overlays().Top()
	owned.overlay = messageOverlay
	owned.restoreFocus = func() {
		if !tracked || a.activeDialog == owned {
			parent.Canvas().Focus(closeButton)
		}
	}
	fyne.Do(func() {
		if parent.Canvas().Overlays().Top() == messageOverlay && (!tracked || a.activeDialog == owned) {
			parent.Canvas().Focus(closeButton)
		}
	})
}

// All dialog functions return the dialog so the caller can store it for Escape dismissal.

func showConfirmDelete(parent fyne.Window, branch string, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	msg := "Delete worktree '" + branch + "'?\n\nThis removes the directory, branch, and metadata."

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	content := container.NewStack(container.NewVScroll(dialogSection(dialogText(msg))), keyCap)

	d = dialog.NewCustomConfirm("Delete Worktree", "Yes", "No", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showConfirmCreateSandbox(parent fyne.Window, sbxName, sbxAgent, repoPath string, kitURLs []string, onDone func(), onConfirm func()) dialog.Dialog {
	return showConfirmCreateSandboxWithKit(parent, sbxName, sbxAgent, "", repoPath, kitURLs, onDone, onConfirm)
}

func showConfirmCreateSandboxWithKit(parent fyne.Window, sbxName, sbxAgent, sandboxRef, repoPath string, mixinURLs []string, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	workload := sbxAgent
	if sandboxRef != "" {
		workload = sandboxRef
	}
	args := sandbox.CreateWithKitArgs(sbxName, workload, repoPath, mixinURLs)
	cmd := sandbox.CommandString(args)

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	body := container.NewVBox(
		dialogText("Create sandbox? This may take a few minutes."),
		dialogHeading("Command"),
	)
	command := dialogCommand(cmd)
	body.Add(command)
	content := container.NewStack(dialogSection(body), keyCap)

	d = dialog.NewCustomConfirm("Create Sandbox", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showConfirmRemoveSandbox(parent fyne.Window, sbxName string, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	args := sandbox.RemoveArgs(sbxName)
	cmd := sandbox.CommandString(args)

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	body := container.NewVBox(
		dialogText("Remove sandbox? This stops and deletes all containers."),
		dialogHeading("Command"),
		dialogCommand(cmd),
	)
	content := container.NewStack(container.NewVScroll(dialogSection(body)), keyCap)

	d = dialog.NewCustomConfirm("Remove Sandbox", "Remove", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showConfirmRemoveMode(parent fyne.Window, repoName, modeLabel string, isSandbox bool, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	var msg string
	if isSandbox {
		msg = fmt.Sprintf("Remove sandbox mode '%s' from %s?\n\nThe sandbox itself is not deleted.", modeLabel, repoName)
	} else {
		msg = fmt.Sprintf("Remove '%s' from %s?", modeLabel, repoName)
	}

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	content := container.NewStack(container.NewVScroll(dialogSection(dialogText(msg))), keyCap)

	d = dialog.NewCustomConfirm("Remove Mode", "Yes", "No", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

// --- Send PR dialogs ---

func showSendPRDirtyWarning(parent fyne.Window, branch string, dirty, hasStash bool, onDone func(), onProceed func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	var warnings []string
	if dirty {
		warnings = append(warnings, "Branch has uncommitted changes.")
	}
	if hasStash {
		warnings = append(warnings, "Branch has stashed changes.")
	}

	body := container.NewVBox(
		dialogTechnical("Send PR for: "+branch),
		widget.NewSeparator(),
	)
	for _, w := range warnings {
		body.Add(dialogText("⚠ " + w))
	}
	body.Add(dialogHeading("Proceed anyway?"))

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	content := container.NewStack(container.NewVScroll(dialogSection(body)), keyCap)

	d = dialog.NewCustomConfirm("Uncommitted Changes", "Continue", "Cancel", content, func(ok bool) {
		if !ok {
			onDone()
			return
		}
		onProceed()
	}, parent)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showSendPRRemoteSelection(parent fyne.Window, remotes []git.RemoteInfo, onDone func(), onSelect func(idx int)) dialog.Dialog {
	var d dialog.Dialog
	advancing := false

	content := container.NewVBox(
		widget.NewLabel("Select a remote to push to:"),
		widget.NewSeparator(),
	)

	var firstBtn *dialogButton
	for i, r := range remotes {
		idx := i
		label := fmt.Sprintf("%s  (%s)", r.Name, r.Repo)
		btn := newDialogButton(label, func() {
			// Hiding this stage fires OnClosed. The flow still owns a modal
			// dialog while the final confirmation replaces it.
			advancing = true
			d.Hide()
			onSelect(idx)
		}, func() { d.Hide() })
		if firstBtn == nil {
			firstBtn = btn
		}
		content.Add(btn)
	}

	d = dialog.NewCustom("Select Remote", "Cancel", container.NewVScroll(dialogSection(content)), parent)
	d.SetOnClosed(func() {
		if !advancing {
			onDone()
		}
	})
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	if firstBtn != nil {
		focusInDialog(parent, firstBtn)
	}
	return d
}

// showSendPRConfirm renders the final confirmation step of the PR send flow.
// When hasNotes is true and the action creates a new PR (existingPR == nil),
// a checkbox lets the user opt in to using their worktree notes — both the
// PR title (from .biomelab/pr-title.md) and description (from .biomelab/
// note.md) — instead of the commit-derived defaults, and a Review button
// opens the note editor for a final pass before sending. The editor is a
// non-modal window so this dialog stays put while the user reviews.
// Defaults to checked since the user took the trouble to prepare notes.
// onConfirm receives the checkbox state — callers should ignore it when no
// checkbox was rendered. onReview is invoked when the user clicks Review;
// callers pass nil when there are no notes to review.
func showSendPRConfirm(parent fyne.Window, branch string, remote git.RemoteInfo, existingPR *provider.PRInfo, hasNotes bool, onDone func(), onConfirm func(useNotes bool), onReview func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	var title, action string
	body := container.NewVBox()

	if existingPR != nil {
		title = "Push Commits"
		action = "Push"
		body.Add(dialogHeading(fmt.Sprintf("PR #%d already exists: %s", existingPR.Number, existingPR.Title)))
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabel("Push new commits to update?"))
	} else {
		title = "Create Pull Request"
		action = "Create PR"
		body.Add(widget.NewLabel("Create a new PR:"))
	}

	body.Add(widget.NewSeparator())
	body.Add(dialogTechnical("Branch: " + branch))
	body.Add(dialogTechnical("Remote: " + remote.Name + " (" + remote.Repo + ")"))

	var noteCheck *widget.Check
	if hasNotes && existingPR == nil {
		body.Add(widget.NewSeparator())
		noteCheck = widget.NewCheck("Use task notes for the PR title and description", nil)
		noteCheck.SetChecked(true)
		body.Add(noteCheck)
		reviewBtn := widget.NewButton("Review notes", func() {
			if onReview != nil {
				onReview()
			}
		})
		reviewBtn.Importance = widget.LowImportance
		body.Add(reviewBtn)
	}

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	// keyCap goes BENEATH body in the Stack so it doesn't intercept mouse
	// clicks meant for the checkbox / Review button. keyCap still receives
	// Enter/Escape via focus (set below) — hit-test and keyboard dispatch
	// are independent.
	content := container.NewStack(keyCap, container.NewVScroll(dialogSection(body)))

	d = dialog.NewCustomConfirm(title, action, "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			useNotes := noteCheck != nil && noteCheck.Checked
			onConfirm(useNotes)
		}
	}, parent)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}
