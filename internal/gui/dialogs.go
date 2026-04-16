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

// All dialog functions return the dialog so the caller can store it for Escape dismissal.

func showConfirmDelete(parent fyne.Window, branch string, onDone func(), onConfirm func()) dialog.Dialog {
	msg := "Delete worktree '" + branch + "'?\n\nThis removes the directory, branch, and metadata."
	d := dialog.NewConfirm("Delete Worktree", msg, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showConfirmCreateSandbox(parent fyne.Window, sbxName, sbxAgent, repoPath string, onDone func(), onConfirm func()) dialog.Dialog {
	args := sandbox.CreateArgs(sbxName, sbxAgent, repoPath)
	cmd := sandbox.CommandString(args)

	content := container.NewVBox(
		widget.NewLabel("Create sandbox? This may take a few minutes."),
		widget.NewLabel("Command:"),
		monoText(cmd, colorSelected, false),
	)

	d := dialog.NewCustomConfirm("Create Sandbox", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showConfirmRemoveSandbox(parent fyne.Window, sbxName string, onDone func(), onConfirm func()) dialog.Dialog {
	args := sandbox.RemoveArgs(sbxName)
	cmd := sandbox.CommandString(args)

	content := container.NewVBox(
		widget.NewLabel("Remove sandbox? This stops and deletes all containers."),
		widget.NewLabel("Command:"),
		monoText(cmd, colorRed, false),
	)

	d := dialog.NewCustomConfirm("Remove Sandbox", "Remove", "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showConfirmRemoveMode(parent fyne.Window, repoName, modeLabel string, isSandbox bool, onDone func(), onConfirm func()) dialog.Dialog {
	var msg string
	if isSandbox {
		msg = fmt.Sprintf("Remove sandbox mode '%s' from %s?\n\nThe sandbox itself is not deleted.", modeLabel, repoName)
	} else {
		msg = fmt.Sprintf("Remove '%s' from %s?", modeLabel, repoName)
	}
	d := dialog.NewConfirm("Remove Mode", msg, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

// --- Send PR dialogs ---

func showSendPRDirtyWarning(parent fyne.Window, branch string, dirty, hasStash bool, onDone func(), onProceed func()) dialog.Dialog {
	var warnings []string
	if dirty {
		warnings = append(warnings, "Branch has uncommitted changes.")
	}
	if hasStash {
		warnings = append(warnings, "Branch has stashed changes.")
	}

	content := container.NewVBox(
		monoText("Send PR for: "+branch, colorBranch, true),
		widget.NewSeparator(),
	)
	for _, w := range warnings {
		content.Add(monoText("⚠ "+w, colorYellow, false))
	}
	content.Add(widget.NewLabel("\nProceed anyway?"))

	d := dialog.NewCustomConfirm("Uncommitted Changes", "Continue", "Cancel", content, func(ok bool) {
		if !ok {
			onDone()
			return
		}
		onProceed()
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showSendPRRemoteSelection(parent fyne.Window, remotes []git.RemoteInfo, onDone func(), onSelect func(idx int)) dialog.Dialog {
	content := container.NewVBox(
		widget.NewLabel("Select a remote to push to:"),
		widget.NewSeparator(),
	)

	var d dialog.Dialog
	for i, r := range remotes {
		idx := i
		label := fmt.Sprintf("%s  (%s)", r.Name, r.Repo)
		btn := widget.NewButton(label, func() {
			d.Hide()
			onSelect(idx)
		})
		content.Add(btn)
	}

	d = dialog.NewCustom("Select Remote", "Cancel", content, parent)
	d.SetOnClosed(func() {
		onDone()
	})
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showSendPRConfirm(parent fyne.Window, branch string, remote git.RemoteInfo, existingPR *provider.PRInfo, onDone func(), onConfirm func()) dialog.Dialog {
	var title, action string
	content := container.NewVBox()

	if existingPR != nil {
		title = "Push Commits"
		action = "Push"
		content.Add(monoText(fmt.Sprintf("PR #%d already exists: %s", existingPR.Number, existingPR.Title), colorBlue, false))
		content.Add(widget.NewSeparator())
		content.Add(widget.NewLabel("Push new commits to update?"))
	} else {
		title = "Create Pull Request"
		action = "Create PR"
		content.Add(widget.NewLabel("Create a new PR:"))
	}

	content.Add(widget.NewSeparator())
	content.Add(monoText("Branch: "+branch, colorBranch, true))
	content.Add(monoText("Remote: "+remote.Name+" ("+remote.Repo+")", colorGray, false))

	d := dialog.NewCustomConfirm(title, action, "Cancel", content, func(ok bool) {
		onDone()
		if ok {
			onConfirm()
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}
