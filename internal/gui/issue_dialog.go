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
	errLabel.Importance = widget.DangerImportance
	entry := newDialogEntry(func() { d.Hide() })
	entry.TextStyle.Monospace = true
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
	lookup.Importance = widget.HighImportance
	cancel := newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	prompt := widget.NewLabel("Enter a GitHub issue number for this repository, or owner/repo#number:")
	prompt.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(nil, dialogFooter(cancel, lookup), nil, nil, dialogGroup(prompt, entry, errLabel))
	d = dialog.NewCustomWithoutButtons("Create Worktree from Issue", content, parent)
	d.SetOnClosed(done)
	d.Resize(boundedDialogSize(parent, fyne.NewSize(580, 240)))
	d.Show()
	return issueInputDialog{Dialog: d, entry: entry}
}

func (a *App) showIssueError(err error) {
	if err != nil {
		a.showInformation("Issue Worktree Error", err.Error())
	}
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
	content := container.NewBorder(nil, dialogFooter(cancel), nil, nil, dialogBusy(lookupLabel.Text))
	d = dialog.NewCustomWithoutButtons("Loading Issue", content, parent)
	d.SetOnClosed(func() {
		onCancel()
		done()
	})
	d.Resize(boundedDialogSize(parent, fyne.NewSize(500, 180)))
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
	branch.TextStyle.Monospace = true
	branch.SetText(ops.SuggestedIssueBranch(issue))
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance
	errorLabel.Hide()
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Hide()

	body := issue.Body
	if strings.TrimSpace(body) == "" {
		body = "(No issue description)"
	}
	bodyLabel := widget.NewLabel(body)
	bodyLabel.Wrapping = fyne.TextWrapWord
	boundedLine := func(text string) *noteEntry {
		return dialogReadOnlyText(text, fyne.TextWrapBreak, func() { d.Hide() })
	}
	wrapped := dialogText
	title := dialogHeading(fmt.Sprintf("#%d  %s", issue.Number, issue.Title))

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
		errorLabel.Hide()
		if err := onCreate(branch.Text); err != nil {
			errorLabel.SetText(err.Error())
			errorLabel.Show()
			return
		}
		branch.Disable()
		create.Disable()
		cancel.SetText("Close")
		status.SetText("Creating worktree and saving issue context… You may close this dialog; creation will continue.")
		status.Show()
	}
	branch.OnSubmitted = func(string) { submit() }
	create = newDialogButton("Create", submit, func() { d.Hide() })
	create.Importance = widget.HighImportance
	cancel = newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	details := container.NewVBox(
		metadata,
		widget.NewSeparator(),
		dialogHeading("Issue description"),
		bodyLabel,
		widget.NewSeparator(),
		boundedLine("Destination repository: "+destination),
		boundedLine("Base: the main checkout's current local HEAD when Create is pressed (currently "+currentHead+")."),
		wrapped("Issue context, progress notes, and agent instructions will be added. Existing tracked instruction files may change."),
	)
	branchSection := dialogGroup(dialogHeading("Branch name"), dialogHint("Letters, digits, and hyphens; 100 characters maximum."), branch, errorLabel, status)
	footer := container.NewVBox(branchSection, dialogFooter(cancel, create))
	content := container.NewBorder(nil, footer, nil, nil, container.NewVScroll(dialogSection(details)))
	d = dialog.NewCustomWithoutButtons("Create Worktree from GitHub Issue", content, parent)
	d.SetOnClosed(done)
	d.Resize(boundedDialogSize(parent, issueDialogSize))
	d.Show()
	return issuePreviewDialog{Dialog: d, branch: branch, create: create, cancel: cancel, error: errorLabel, status: status}
}
