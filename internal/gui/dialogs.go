package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/kits"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

var dialogMinSize = fyne.NewSize(450, 200)
var kitsDialogSize = fyne.NewSize(600, 480)

// All dialog functions return the dialog so the caller can store it for Escape dismissal.

func showConfirmDelete(parent fyne.Window, branch string, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	msg := "Delete worktree '" + branch + "'?\n\nThis removes the directory, branch, and metadata."

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	content := container.NewStack(widget.NewLabel(msg), keyCap)

	d = dialog.NewCustomConfirm("Delete Worktree", "Yes", "No", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showConfirmCreateSandbox(parent fyne.Window, sbxName, sbxAgent, repoPath string, kitURLs []string, onDone func(), onConfirm func()) dialog.Dialog {
	var d *dialog.ConfirmDialog
	args := sandbox.CreateArgs(sbxName, sbxAgent, repoPath, kitURLs)
	cmd := sandbox.CommandString(args)

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	body := container.NewVBox(
		widget.NewLabel("Create sandbox? This may take a few minutes."),
		widget.NewLabel("Command:"),
	)
	command := container.NewHScroll(monoText(cmd, colorSelected, false))
	command.SetMinSize(fyne.NewSize(760, 48))
	body.Add(command)
	content := container.NewStack(body, keyCap)

	d = dialog.NewCustomConfirm("Create Sandbox", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
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
		widget.NewLabel("Remove sandbox? This stops and deletes all containers."),
		widget.NewLabel("Command:"),
		monoText(cmd, colorRed, false),
	)
	content := container.NewStack(body, keyCap)

	d = dialog.NewCustomConfirm("Remove Sandbox", "Remove", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
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
	content := container.NewStack(widget.NewLabel(msg), keyCap)

	d = dialog.NewCustomConfirm("Remove Mode", "Yes", "No", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
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
		monoText("Send PR for: "+branch, colorBranch, true),
		widget.NewSeparator(),
	)
	for _, w := range warnings {
		body.Add(monoText("⚠ "+w, colorYellow, false))
	}
	body.Add(widget.NewLabel("\nProceed anyway?"))

	keyCap := newDialogKeyCapture(
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	content := container.NewStack(body, keyCap)

	d = dialog.NewCustomConfirm("Uncommitted Changes", "Continue", "Cancel", content, func(ok bool) {
		if !ok {
			onDone()
			return
		}
		onProceed()
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}

func showSendPRRemoteSelection(parent fyne.Window, remotes []git.RemoteInfo, onDone func(), onSelect func(idx int)) dialog.Dialog {
	var d dialog.Dialog

	content := container.NewVBox(
		widget.NewLabel("Select a remote to push to:"),
		widget.NewSeparator(),
	)

	var firstBtn *dialogButton
	for i, r := range remotes {
		idx := i
		label := fmt.Sprintf("%s  (%s)", r.Name, r.Repo)
		btn := newDialogButton(label, func() {
			d.Hide()
			onSelect(idx)
		}, func() { d.Hide() })
		if firstBtn == nil {
			firstBtn = btn
		}
		content.Add(btn)
	}

	d = dialog.NewCustom("Select Remote", "Cancel", content, parent)
	d.SetOnClosed(func() {
		onDone()
	})
	d.Resize(dialogMinSize)
	d.Show()
	if firstBtn != nil {
		focusInDialog(parent, firstBtn)
	}
	return d
}

// showKitsDialog renders a multi-select picker for sandbox kits, split into
// Agents and Mixins. Mixins are pre-filtered for the sandbox's agent by the
// caller — this function only renders.
//
// onSubmit fires with the selected kits in the order they were rendered.
func showKitsDialog(
	parent fyne.Window,
	sandboxName, agent string,
	agents, mixins []kits.Kit,
	onDone func(),
	onSubmit func(selected []kits.Kit),
) dialog.Dialog {
	var d *dialog.ConfirmDialog
	checks := make([]*dialogCheck, 0, len(agents)+len(mixins))
	all := make([]kits.Kit, 0, len(agents)+len(mixins))

	body := container.NewVBox(
		monoText(
			fmt.Sprintf("Install kits into %s (agent: %s)", sandboxName, agent),
			colorBranch, true,
		),
		widget.NewSeparator(),
	)

	addSection := func(title string, list []kits.Kit, emptyHint string) {
		header := monoText(title, colorBlue, true)
		body.Add(header)
		if len(list) == 0 {
			hint := monoText(emptyHint, colorDimGray, false)
			hint.TextSize = scaledSize(11)
			body.Add(hint)
			body.Add(widget.NewSeparator())
			return
		}
		for _, k := range list {
			checks = append(checks, newDialogCheck(displayLabel(k), nil, func() { d.Hide() }))
			all = append(all, k)
			body.Add(checks[len(checks)-1])
			if k.Description != "" {
				desc := monoText("    "+k.Description, colorGray, false)
				desc.TextSize = scaledSize(10)
				body.Add(desc)
			}
		}
		body.Add(widget.NewSeparator())
	}

	if len(agents) > 0 {
		addSection("Agents", agents, "(none available)")
	}
	addSection(fmt.Sprintf("Mixins for %s", agent), mixins, "(no mixins compatible with this agent)")

	// keyCap captures Escape (cancel) globally inside the dialog. Enter is
	// NOT bound to confirm because Space toggles the focused checkbox and
	// users may want to toggle several before submitting — Enter would
	// race with that. They click "Install" or Tab to it and press Space.
	keyCap := newDialogKeyCapture(
		func() {}, // no-op on Enter — see comment above
		func() { d.Hide() },
	)
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(kitsDialogSize)
	content := container.NewStack(keyCap, scroll)

	d = dialog.NewCustomConfirm("Choose Kits", "Continue", "Cancel", content, func(ok bool) {
		onDone()
		if !ok {
			return
		}
		var selected []kits.Kit
		for i, c := range checks {
			if c.Checked {
				selected = append(selected, all[i])
			}
		}
		onSubmit(selected)
	}, parent)
	d.Resize(kitsDialogSize)
	d.Show()
	// Focus the first checkbox if any exist, so the user can Space-toggle
	// immediately. Fall back to the key-capture overlay so Escape still
	// dismisses even with no checkboxes.
	if len(checks) > 0 {
		focusInDialog(parent, checks[0])
	} else {
		focusInDialog(parent, keyCap)
	}
	return d
}

// displayLabel returns the user-visible label for a kit row.
func displayLabel(k kits.Kit) string {
	name := k.DisplayName
	if name == "" {
		name = k.Name
	}
	return fmt.Sprintf("%s  (%s)", name, k.Name)
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
		body.Add(monoText(fmt.Sprintf("PR #%d already exists: %s", existingPR.Number, existingPR.Title), colorBlue, false))
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabel("Push new commits to update?"))
	} else {
		title = "Create Pull Request"
		action = "Create PR"
		body.Add(widget.NewLabel("Create a new PR:"))
	}

	body.Add(widget.NewSeparator())
	body.Add(monoText("Branch: "+branch, colorBranch, true))
	body.Add(monoText("Remote: "+remote.Name+" ("+remote.Repo+")", colorGray, false))

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
	content := container.NewStack(keyCap, body)

	d = dialog.NewCustomConfirm(title, action, "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			useNotes := noteCheck != nil && noteCheck.Checked
			onConfirm(useNotes)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, keyCap)
	return d
}
