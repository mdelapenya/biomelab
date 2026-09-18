package gui

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/ops"
)

var issueDialogSize = fyne.NewSize(800, 620)

type issueInputDialog struct {
	dialog.Dialog
	entry *dialogEntry
}

func showIssueInput(parent fyne.Window, onClosed func(), onSubmit func(string) error, onAccepted func()) issueInputDialog {
	var d dialog.Dialog
	var once sync.Once
	done := func() { once.Do(onClosed) }
	errLabel := widget.NewLabel("")
	errLabel.Wrapping = fyne.TextWrapWord
	entry := newDialogEntry(func() { d.Hide() })
	entry.SetPlaceHolder("82 or owner/repo#82")
	submit := func() {
		if err := onSubmit(entry.Text); err != nil {
			errLabel.SetText(err.Error())
			return
		}
		d.Hide()
		// Hide returns after the old overlay and its app-owned dialog state
		// are cleaned up, so the next stage can safely claim that ownership.
		onAccepted()
	}
	entry.OnSubmitted = func(string) { submit() }
	lookup := newDialogButton("Look Up", submit, func() { d.Hide() })
	cancel := newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	prompt := widget.NewLabel("Enter a GitHub issue number for this repository, or owner/repo#number:")
	prompt.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		prompt,
		entry,
		errLabel,
		container.NewHBox(lookup, cancel),
	)
	d = dialog.NewCustomWithoutButtons("Create Worktree from Issue", content, parent)
	d.SetOnClosed(done)
	d.Resize(fyne.NewSize(580, 240))
	d.Show()
	return issueInputDialog{Dialog: d, entry: entry}
}

func (a *App) showIssueError(err error) {
	var d dialog.Dialog
	done := a.openDialog()
	message := widget.NewLabel(err.Error())
	message.Wrapping = fyne.TextWrapWord
	closeButton := newDialogButton("Close", func() { d.Hide() }, func() { d.Hide() })
	d = dialog.NewCustomWithoutButtons("Issue Worktree Error", container.NewVBox(message, closeButton), a.window)
	var once sync.Once
	d.SetOnClosed(func() { once.Do(done) })
	d.Resize(fyne.NewSize(580, 220))
	a.activeDialog = d
	d.Show()
	fyne.Do(func() {
		if a.dialogOpen && a.activeDialog == d {
			a.window.Canvas().Focus(closeButton)
		}
	})
}

func showIssueLoading(parent fyne.Window, issueRef string, onClosed, onCancel func()) dialog.Dialog {
	var d dialog.Dialog
	var once sync.Once
	done := func() { once.Do(onClosed) }
	cancel := newDialogButton("Cancel", func() {
		onCancel()
		d.Hide()
	}, func() {
		onCancel()
		d.Hide()
	})
	lookupLabel := widget.NewLabel("Looking up GitHub issue " + issueRef + "…")
	lookupLabel.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		lookupLabel,
		widget.NewProgressBarInfinite(),
		cancel,
	)
	d = dialog.NewCustomWithoutButtons("Loading Issue", content, parent)
	d.SetOnClosed(func() {
		onCancel()
		done()
	})
	d.Resize(fyne.NewSize(500, 180))
	d.Show()
	return d
}

type issuePreviewDialog struct {
	dialog.Dialog
	branch *dialogEntry
	create *dialogButton
	cancel *dialogButton
	error  *widget.Label
	status *widget.Label
}

func showIssuePreview(parent fyne.Window, issue github.IssueInfo, sourceRepo, destination, currentHead string,
	onClosed func(), onCreate func(string) error) issuePreviewDialog {
	var d dialog.Dialog
	var once sync.Once
	done := func() { once.Do(onClosed) }

	branch := newDialogEntry(func() { d.Hide() })
	branch.SetText(ops.SuggestedIssueBranch(issue))
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	body := issue.Body
	if strings.TrimSpace(body) == "" {
		body = "(No issue description)"
	}
	bodyLabel := widget.NewLabel(body)
	bodyLabel.Wrapping = fyne.TextWrapWord
	bodyScroll := container.NewVScroll(bodyLabel)
	bodyScroll.SetMinSize(fyne.NewSize(740, 100))
	wrapped := func(text string) *widget.Label {
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		return label
	}
	boundedLine := func(text string) *container.Scroll {
		line := container.NewHScroll(widget.NewLabel(text))
		line.SetMinSize(fyne.NewSize(740, 36))
		return line
	}
	titleLabel := wrapped(fmt.Sprintf("#%d  %s", issue.Number, issue.Title))
	title := container.NewVScroll(titleLabel)
	title.SetMinSize(fyne.NewSize(740, 55))

	metadata := container.NewVBox(
		title,
		wrapped("State: "+issue.State),
		boundedLine("Source repository: "+sourceRepo),
	)
	if u, err := url.Parse(issue.URL); err == nil {
		metadata.Add(widget.NewHyperlink(fmt.Sprintf("Open issue #%d on GitHub", issue.Number), u))
	} else {
		metadata.Add(widget.NewLabel(issue.URL))
	}

	var create, cancel *dialogButton
	submit := func() {
		errorLabel.SetText("")
		if err := onCreate(branch.Text); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		branch.Disable()
		create.Disable()
		cancel.SetText("Close")
		status.SetText("Creating worktree and saving issue context… You may close this dialog; creation will continue.")
	}
	branch.OnSubmitted = func(string) { submit() }
	create = newDialogButton("Create", submit, func() { d.Hide() })
	cancel = newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	details := container.NewVBox(
		metadata,
		widget.NewSeparator(),
		widget.NewLabel("Issue description:"),
		bodyScroll,
		widget.NewSeparator(),
		boundedLine("Destination repository: "+destination),
		boundedLine("Base: the main checkout's current local HEAD when Create is pressed (currently "+currentHead+")."),
		wrapped("Issue context, progress notes, and agent instructions will be added. Existing tracked instruction files may change."),
		wrapped("Branch (letters, digits, and hyphens; 100 characters maximum):"),
		branch,
		errorLabel,
		status,
	)
	content := container.NewBorder(nil, container.NewHBox(create, cancel), nil, nil, details)
	d = dialog.NewCustomWithoutButtons("Create Worktree from GitHub Issue", content, parent)
	d.SetOnClosed(done)
	d.Resize(issueDialogSize)
	d.Show()
	return issuePreviewDialog{Dialog: d, branch: branch, create: create, cancel: cancel, error: errorLabel, status: status}
}
