package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func showBranchInput(parent fyne.Window, onDone func(), onSubmit func(name string)) dialog.Dialog {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("branch-name")

	content := container.NewVBox(
		widget.NewLabel("Create a new worktree:"),
		entry,
	)

	d := dialog.NewCustomConfirm("Create Worktree", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showFetchPRInput(parent fyne.Window, onDone func(), onSubmit func(input string)) dialog.Dialog {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("123 or owner/repo#123")

	content := container.NewVBox(
		widget.NewLabel("Fetch a pull request:"),
		entry,
	)

	d := dialog.NewCustomConfirm("Fetch PR", "Fetch", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showAddRepoInput(parent fyne.Window, onDone func(), onSubmit func(path string)) dialog.Dialog {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("/path/to/repository")

	content := container.NewVBox(
		widget.NewLabel("Add a repository:"),
		entry,
	)

	d := dialog.NewCustomConfirm("Add Repository", "Add", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

func showModeSelection(parent fyne.Window, onDone func(), onRegular func(), onSandbox func()) dialog.Dialog {
	var d dialog.Dialog

	content := container.NewVBox(
		widget.NewLabel("Select mode:"),
		widget.NewButton("Sandbox (recommended)", func() {
			d.Hide()
			onSandbox()
		}),
		widget.NewButton("Regular (host)", func() {
			d.Hide()
			onRegular()
		}),
	)

	d = dialog.NewCustom("Select Mode", "Cancel", content, parent)
	d.SetOnClosed(func() {
		onDone()
	})
	d.Resize(dialogMinSize)
	d.Show()
	return d
}

var agentOptions = []string{"claude", "codex", "copilot", "docker-agent", "gemini", "kiro", "opencode", "shell"}

func showAgentInput(parent fyne.Window, onDone func(), onSubmit func(agent string)) dialog.Dialog {
	sel := widget.NewSelect(agentOptions, nil)
	sel.PlaceHolder = "Select agent..."

	content := container.NewVBox(
		widget.NewLabel("Agent for sandbox:"),
		sel,
	)

	d := dialog.NewCustomConfirm("Sandbox Agent", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok && sel.Selected != "" {
			onSubmit(sel.Selected)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	return d
}
