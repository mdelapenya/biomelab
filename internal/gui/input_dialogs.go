package gui

import (
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

const sbxInstallURL = "https://docs.docker.com/ai/sandboxes/"

func showBranchInput(parent fyne.Window, onDone func(), onSubmit func(name string)) dialog.Dialog {
	var d *dialog.CustomDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.TextStyle.Monospace = true
	entry.SetPlaceHolder("branch-name")
	submit := func() {
		name := strings.TrimSpace(entry.Text)
		if name == "" {
			return
		}
		d.Hide()
		onSubmit(name)
	}
	entry.OnSubmitted = func(_ string) { submit() }
	create := newDialogButton("Create", submit, func() { d.Hide() })
	create.Importance = widget.HighImportance
	create.Disable()
	entry.OnChanged = func(text string) {
		if strings.TrimSpace(text) == "" {
			create.Disable()
		} else {
			create.Enable()
		}
	}

	content := dialogGroup(
		dialogHeading("Branch name"),
		entry,
	)

	d = dialog.NewCustomWithoutButtons("Create Worktree", content, parent)
	d.SetButtons([]fyne.CanvasObject{newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() }), create})
	d.SetOnClosed(onDone)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, entry)
	return d
}

func showFetchPRInput(parent fyne.Window, onDone func(), onSubmit func(input string)) dialog.Dialog {
	var d *dialog.CustomDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.TextStyle.Monospace = true
	entry.SetPlaceHolder("123 or owner/repo#123")
	submit := func() {
		input := strings.TrimSpace(entry.Text)
		if input == "" {
			return
		}
		d.Hide()
		onSubmit(input)
	}
	entry.OnSubmitted = func(_ string) { submit() }
	fetch := newDialogButton("Fetch", submit, func() { d.Hide() })
	fetch.Importance = widget.HighImportance
	fetch.Disable()
	entry.OnChanged = func(text string) {
		if strings.TrimSpace(text) == "" {
			fetch.Disable()
		} else {
			fetch.Enable()
		}
	}

	content := dialogGroup(
		dialogHeading("Pull request number or reference"),
		entry,
	)

	d = dialog.NewCustomWithoutButtons("Fetch PR", content, parent)
	d.SetButtons([]fyne.CanvasObject{newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() }), fetch})
	d.SetOnClosed(onDone)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, entry)
	return d
}

func showAddRepoInput(parent fyne.Window, onDone func(), onSubmit func(path string)) dialog.Dialog {
	var d *dialog.CustomDialog

	entry := newDialogEntry(func() { d.Hide() })
	entry.TextStyle.Monospace = true
	entry.SetPlaceHolder("/path/to/repository")
	submit := func() {
		path := strings.TrimSpace(entry.Text)
		if path == "" {
			return
		}
		d.Hide()
		onSubmit(path)
	}
	entry.OnSubmitted = func(_ string) { submit() }
	add := newDialogButton("Add", submit, func() { d.Hide() })
	add.Importance = widget.HighImportance
	add.Disable()
	entry.OnChanged = func(text string) {
		if strings.TrimSpace(text) == "" {
			add.Disable()
		} else {
			add.Enable()
		}
	}

	content := dialogGroup(
		dialogHeading("Repository path"),
		entry,
	)

	d = dialog.NewCustomWithoutButtons("Add Repository", content, parent)
	d.SetButtons([]fyne.CanvasObject{newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() }), add})
	d.SetOnClosed(onDone)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
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

	sbxBtn.Importance = widget.HighImportance
	regBtn := newDialogButton("Regular (host)", func() {
		d.Hide()
		onRegular()
	}, func() { d.Hide() })

	content := container.NewVBox(
		dialogHeading("Select mode"),
		sbxBtn,
	)

	if !sbxAvailable {
		sbxBtn.Disable()
		installURL, _ := url.Parse(sbxInstallURL)
		content.Add(container.NewVBox(
			dialogText("sbx CLI not found in PATH"),
			widget.NewHyperlink("install sbx", installURL),
		))
	}

	content.Add(regBtn)

	d = newKeyboardDismiss("Select Mode", "Cancel", dialogSection(content), parent)
	d.SetOnClosed(func() {
		onDone()
	})
	d.Resize(boundedDialogSize(parent, dialogMinSize))
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
	var d *dialog.CustomDialog
	var sel, addKits *dialogSelect
	submit := func() {
		if sel.Selected == "" && addKits.Selected != "Yes" {
			return
		}
		d.Hide()
		onSubmit(sel.Selected, addKits.Selected == "Yes")
	}

	sel = newDialogSelect(agentOptions,
		submit,
		func() { d.Hide() },
	)
	sel.PlaceHolder = "Select agent..."
	addKits = newDialogSelect([]string{"No", "Yes"}, submit, func() { d.Hide() })
	addKits.SetSelected("No")
	continueButton := newDialogButton("Continue", submit, func() { d.Hide() })
	continueButton.Importance = widget.HighImportance
	continueButton.Disable()
	updateContinue := func(string) {
		if sel.Selected == "" && addKits.Selected != "Yes" {
			continueButton.Disable()
		} else {
			continueButton.Enable()
		}
	}
	sel.OnChanged = updateContinue
	addKits.OnChanged = updateContinue

	content := dialogGroup(
		dialogHeading("Built-in agent"),
		sel,
		dialogHint("Used when kits are skipped."),
		dialogHeading("Do you want to add kits?"),
		addKits,
	)

	d = dialog.NewCustomWithoutButtons("New Sandbox", content, parent)
	d.SetButtons([]fyne.CanvasObject{newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() }), continueButton})
	d.SetOnClosed(onDone)
	d.Resize(boundedDialogSize(parent, dialogMinSize))
	d.Show()
	focusInDialog(parent, sel)
	return d
}
