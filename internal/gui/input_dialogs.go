package gui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

const sbxInstallURL = "https://docs.docker.com/ai/sandboxes/"

func showBranchInput(parent fyne.Window, onDone func(), onSubmit func(name string)) dialog.Dialog {
	var d *dialog.ConfirmDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.SetPlaceHolder("branch-name")
	entry.OnSubmitted = func(_ string) { d.Confirm() }

	content := container.NewVBox(
		widget.NewLabel("Create a new worktree:"),
		entry,
	)

	d = dialog.NewCustomConfirm("Create Worktree", "Create", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, entry)
	return d
}

func showFetchPRInput(parent fyne.Window, onDone func(), onSubmit func(input string)) dialog.Dialog {
	var d *dialog.ConfirmDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.SetPlaceHolder("123 or owner/repo#123")
	entry.OnSubmitted = func(_ string) { d.Confirm() }

	content := container.NewVBox(
		widget.NewLabel("Fetch a pull request:"),
		entry,
	)

	d = dialog.NewCustomConfirm("Fetch PR", "Fetch", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, entry)
	return d
}

func showAddRepoInput(parent fyne.Window, onDone func(), onSubmit func(path string)) dialog.Dialog {
	var d *dialog.ConfirmDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.SetPlaceHolder("/path/to/repository")
	entry.OnSubmitted = func(_ string) { d.Confirm() }

	content := container.NewVBox(
		widget.NewLabel("Add a repository:"),
		entry,
	)

	d = dialog.NewCustomConfirm("Add Repository", "Add", "Cancel", content, func(ok bool) {
		onDone()
		if ok && entry.Text != "" {
			onSubmit(entry.Text)
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, entry)
	return d
}

func showModeSelection(parent fyne.Window, onDone func(), onRegular func(), onSandbox func()) dialog.Dialog {
	var d dialog.Dialog

	sbxAvailable := sandbox.Available()
	sbxBtn := newDialogButton("Sandbox (recommended)", func() {
		d.Hide()
		onSandbox()
	}, func() { d.Hide() })

	regBtn := newDialogButton("Regular (host)", func() {
		d.Hide()
		onRegular()
	}, func() { d.Hide() })

	content := container.NewVBox(
		widget.NewLabel("Select mode:"),
		sbxBtn,
	)

	if !sbxAvailable {
		sbxBtn.Disable()
		installURL, _ := url.Parse(sbxInstallURL)
		content.Add(container.NewHBox(
			widget.NewLabel("sbx CLI not found in PATH —"),
			widget.NewHyperlink("install sbx", installURL),
		))
	}

	content.Add(regBtn)

	d = dialog.NewCustom("Select Mode", "Cancel", content, parent)
	d.SetOnClosed(func() {
		onDone()
	})
	d.Resize(dialogMinSize)
	d.Show()

	// Focus the recommended option when available so Enter accepts it; fall
	// back to the regular button when sandbox is disabled.
	if sbxAvailable {
		focusInDialog(parent, sbxBtn)
	} else {
		focusInDialog(parent, regBtn)
	}
	return d
}

var agentOptions = []string{"claude", "codex", "copilot", "docker-agent", "gemini", "kiro", "opencode", "shell"}

func showAgentInput(parent fyne.Window, onDone func(), onSubmit func(agent string, addKits bool)) dialog.Dialog {
	var d *dialog.ConfirmDialog

	sel := newDialogSelect(agentOptions,
		func() { d.Confirm() },
		func() { d.Hide() },
	)
	sel.PlaceHolder = "Select agent..."
	addKits := newDialogSelect([]string{"No", "Yes"}, nil, func() { d.Hide() })
	addKits.SetSelected("No")

	content := container.NewVBox(
		widget.NewLabel("Agent for sandbox:"),
		sel,
		widget.NewLabel("Do you want to add kits?"),
		addKits,
	)

	d = dialog.NewCustomConfirm("New Sandbox", "Continue", "Cancel", content, func(ok bool) {
		onDone()
		if ok && sel.Selected != "" {
			onSubmit(sel.Selected, addKits.Selected == "Yes")
		}
	}, parent)
	d.Resize(dialogMinSize)
	d.Show()
	focusInDialog(parent, sel)
	return d
}
