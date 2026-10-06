package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestDeleteConfirmationIdentifiesWorktreeAndDefaultsToCancel(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	for _, action := range []string{"Enter", "Escape", "selected identity Escape", "explicit Delete"} {
		t.Run(action, func(t *testing.T) {
			w := fa.NewWindow("delete confirmation")
			defer w.Close()
			w.SetContent(container.NewStack())
			w.Resize(fyne.NewSize(1000, 740))
			w.Show()
			a := &App{window: w}
			const branch = "user/important-work"
			path := "/Users/example/worktrees/" + strings.Repeat("complete-path-", 12)
			confirmed, closed := 0, 0
			done := a.openDialog()
			a.activeDialog = showConfirmDelete(w, branch, path, func() { closed++; done() }, func() { confirmed++ })
			focused, ok := w.Canvas().Focused().(*dialogButton)
			if !ok || focused.Text != "Cancel" {
				t.Fatalf("destructive confirmation defaults to %T instead of Cancel", w.Canvas().Focused())
			}
			var identity, warning bool
			var identityFocus fyne.Focusable
			var remove *dialogButton
			walkPolish(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
				if entry, ok := o.(*noteEntry); ok && entry.Text == "Branch: "+branch+"\nPath: "+path {
					identity = entry.Disabled() && entry.TextStyle.Monospace && entry.Wrapping == fyne.TextWrapBreak
					identityFocus = entry
				}
				if label, ok := o.(*widget.Label); ok {
					if label.Text == "Branch: "+branch+"\nPath: "+path {
						identity = label.Selectable && label.TextStyle.Monospace && label.Wrapping == fyne.TextWrapBreak
						walkPolish(label, func(child fyne.CanvasObject) {
							if focus, ok := child.(fyne.Focusable); ok {
								identityFocus = focus
							}
						})
					}
					warning = warning || strings.Contains(label.Text, "ignored files and task notes")
				}
				if button, ok := o.(*dialogButton); ok && button.Text == "Delete worktree" {
					remove = button
				}
			})
			if !identity || !warning || remove == nil {
				t.Fatal("confirmation omitted complete identity or ignored-data warning")
			}
			c := &keyboardCanvas{Canvas: w.Canvas()}
			if action == "explicit Delete" {
				w.Canvas().Focus(remove)
			}
			if action == "selected identity Escape" {
				w.Canvas().Focus(identityFocus)
				if identityFocus == nil || w.Canvas().Focused() != identityFocus {
					t.Fatal("complete worktree identity cannot receive selection focus")
				}
				selectModalContext(t, w, identityFocus)
				selected := identityFocus.(interface{ SelectedText() string }).SelectedText()
				if selected == "" {
					t.Fatal("worktree identity lost text selection")
				}
				identityFocus.(fyne.Shortcutable).TypedShortcut(&fyne.ShortcutCopy{Clipboard: fa.Clipboard()})
				if fa.Clipboard().Content() != selected {
					t.Fatal("worktree identity lost selection copy")
				}
			}
			key := fyne.KeyReturn
			if action == "Escape" || action == "selected identity Escape" {
				key = fyne.KeyEscape
			}
			c.press(key)
			if closed != 1 || a.dialogOpen || a.activeDialog != nil || confirmed != btoi(action == "explicit Delete") {
				t.Fatalf("action=%s closed=%d confirmed=%d modal=%v", action, closed, confirmed, a.dialogOpen)
			}
		})
	}
}
