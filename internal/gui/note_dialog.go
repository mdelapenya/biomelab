package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/notes"
)

var noteWindowInitialSize = fyne.NewSize(720, 500)

// noteEntry is a multi-line Entry that invokes onEscape on Escape, so the
// caller can close its container (a dialog or a standalone window) without
// Fyne's default focus manager swallowing the key.
type noteEntry struct {
	widget.Entry
	onEscape func()
}

func newNoteEntry(initial string, onEscape func()) *noteEntry {
	e := &noteEntry{onEscape: onEscape}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.SetPlaceHolder("Write notes in Markdown — # headings, **bold**, *italic*, [links](url), lists, `code`, ```code blocks```")
	e.SetText(initial)
	e.ExtendBaseWidget(e)
	return e
}

func (e *noteEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape {
		if e.onEscape != nil {
			e.onEscape()
		}
		return
	}
	e.Entry.TypedKey(key)
}

// openNoteDialog opens the note editor as a standalone window so the user
// can drag-resize it for long notes. The window has an H-split editor +
// live markdown preview, plus Save / Cancel / Delete buttons. Saving an
// empty note deletes the file; Cmd/Ctrl+S also saves; Esc cancels. The
// window is non-modal: the main window stays usable while the editor is
// open (useful for referencing a card while writing).
func (a *App) openNoteDialog(wt git.Worktree) {
	// Re-focus an existing editor for this worktree rather than spawning
	// another window. Repeated 'm' presses or Review clicks just raise
	// the open editor.
	if existing, ok := a.noteWindows[wt.Path]; ok {
		existing.RequestFocus()
		return
	}
	origin := a.noteRepoForPath(wt.Path)

	initial, bodyExists, _ := notes.Read(wt.Path)
	initialTitle, titleExists, _ := notes.ReadTitle(wt.Path)
	noteExists := bodyExists || titleExists

	w := a.fyneApp.NewWindow("Note — " + wt.Branch)
	if a.noteWindows == nil {
		a.noteWindows = make(map[string]fyne.Window)
	}
	a.noteWindows[wt.Path] = w
	w.SetOnClosed(func() {
		delete(a.noteWindows, wt.Path)
	})

	preview := widget.NewRichTextFromMarkdown(initial)
	preview.Wrapping = fyne.TextWrapWord
	previewScroll := container.NewVScroll(preview)

	entry := newNoteEntry(initial, func() { w.Close() })
	entry.OnChanged = func(s string) {
		preview.ParseMarkdown(s)
	}

	titleEntry := newDialogEntry(func() { w.Close() })
	titleEntry.SetPlaceHolder("Conventional Commits title — feat(scope): description")
	titleEntry.SetText(initialTitle)

	titleLabel := monoText("# PR title", colorBranch, true)
	titleLabel.TextSize = scaledSize(11)
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Hide()
	titleSection := container.NewVBox(titleLabel, titleEntry, errorLabel)

	editorLabel := monoText("✎ Markdown", colorBranch, true)
	editorLabel.TextSize = scaledSize(11)
	previewLabel := monoText("👁 Preview", colorBranch, true)
	previewLabel.TextSize = scaledSize(11)

	editorPane := container.NewBorder(editorLabel, nil, nil, nil, entry)
	previewPane := container.NewBorder(previewLabel, nil, nil, nil, previewScroll)

	split := container.NewHSplit(editorPane, previewPane)
	split.Offset = 0.5

	saveAndClose := func() {
		titleErr := notes.WriteTitle(wt.Path, titleEntry.Text)
		var bodyErr error
		if strings.TrimSpace(entry.Text) == "" {
			bodyErr = notes.Delete(wt.Path)
		} else {
			bodyErr = notes.Write(wt.Path, entry.Text)
		}
		var message string
		switch {
		case titleErr != nil:
			message = "Note save failed (title): " + titleErr.Error()
		case bodyErr != nil:
			message = "Note save failed (body): " + bodyErr.Error()
		}
		a.rebuildNoteRepo(origin)
		if message != "" {
			errorLabel.SetText(message)
			errorLabel.Show()
			a.setRepoStatus(origin, message, true)
			return
		}
		a.setRepoStatus(origin, "Note saved", false)
		w.Close()
	}

	saveBtn := widget.NewButton("Save", saveAndClose)
	saveBtn.Importance = widget.HighImportance
	cancelBtn := widget.NewButton("Cancel", func() { w.Close() })

	rightButtons := container.NewHBox(cancelBtn, saveBtn)
	var leftSide fyne.CanvasObject
	if noteExists {
		deleteBtn := widget.NewButton("Delete note", func() {
			dialog.ShowConfirm(
				"Delete note?",
				"This permanently removes the title and description for "+wt.Branch+".",
				func(confirmed bool) {
					if !confirmed {
						return
					}
					bodyErr := notes.Delete(wt.Path)
					titleErr := notes.DeleteTitle(wt.Path)
					a.rebuildNoteRepo(origin)
					var message string
					switch {
					case bodyErr != nil:
						message = "Note delete failed: " + bodyErr.Error()
					case titleErr != nil:
						message = "Note delete failed (title): " + titleErr.Error()
					}
					if message != "" {
						errorLabel.SetText(message)
						errorLabel.Show()
						a.setRepoStatus(origin, message, true)
						return
					}
					a.setRepoStatus(origin, "Note deleted", false)
					w.Close()
				},
				w,
			)
		})
		deleteBtn.Importance = widget.LowImportance
		leftSide = deleteBtn
	}
	bottomRow := container.NewBorder(nil, nil, leftSide, rightButtons, nil)

	content := container.NewBorder(titleSection, bottomRow, nil, nil, split)
	w.SetContent(content)
	w.Resize(noteWindowInitialSize)
	w.CenterOnScreen()

	// Cmd/Ctrl+S also saves. Canvas-level shortcut fires even when the
	// entry has focus because Cmd+S has a non-zero modifier and Entry
	// doesn't bind it.
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}, func(_ fyne.Shortcut) {
		saveAndClose()
	})

	w.Show()
	// When the editor is opened from another window's dialog overlay
	// (e.g. the Review button in showSendPRConfirm), macOS can leave the
	// new window behind the parent. Explicitly request focus.
	w.RequestFocus()
	w.Canvas().Focus(entry)
}

// noteRepoForPath resolves the worktree that owns a non-modal editor at
// open time. Its callbacks keep this identity even if another repo becomes
// active while the editor remains open.
func (a *App) noteRepoForPath(path string) *repoEntry {
	if re := a.activeRepo(); re != nil && re.state != nil {
		for _, wt := range re.state.Worktrees {
			if wt.Path == path {
				return re
			}
		}
	}
	for _, re := range a.repos {
		if re.state == nil {
			continue
		}
		for _, wt := range re.state.Worktrees {
			if wt.Path == path {
				return re
			}
		}
	}
	return nil
}

func (a *App) rebuildNoteRepo(re *repoEntry) {
	if a.hasRepoEntry(re) && re.dashboard != nil {
		re.dashboard.Rebuild()
	}
}
