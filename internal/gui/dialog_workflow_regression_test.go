package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/git"
	"os"
	"path/filepath"
	"testing"
)

func TestSendPRRemoteSelectionRetainsModalGuardThroughConfirmation(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("send PR")
	defer w.Close()
	a := &App{window: w}
	done := a.openDialog()
	selection := showSendPRRemoteSelection(w, []git.RemoteInfo{{Name: "origin"}, {Name: "upstream"}}, done, func(int) {
		a.activeDialog = showSendPRConfirm(w, "feature", git.RemoteInfo{Name: "origin"}, nil, false, done, func(bool) {}, nil)
	})
	a.activeDialog = selection
	button, ok := w.Canvas().Focused().(*dialogButton)
	if !ok {
		t.Fatalf("remote button was not focused: %T", w.Canvas().Focused())
	}
	button.Tapped(nil)
	if !a.dialogOpen || a.activeDialog == nil || a.activeDialog == selection {
		t.Fatal("remote selection cleared modal ownership before final confirmation")
	}
	a.handleKeyName(fyne.KeyE)
	if !a.dialogOpen {
		t.Fatal("global shortcut escaped through the final confirmation")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.dialogOpen {
		t.Fatal("final confirmation did not release modal guard")
	}
}

func findNoteControls(root fyne.CanvasObject) (*noteEntry, *widget.Button) {
	var entry *noteEntry
	var save *widget.Button
	var visit func(fyne.CanvasObject)
	visit = func(obj fyne.CanvasObject) {
		if e, ok := obj.(*noteEntry); ok {
			entry = e
		}
		if b, ok := obj.(*widget.Button); ok && b.Text == "Save" {
			save = b
		}
		if c, ok := obj.(*fyne.Container); ok {
			for _, child := range c.Objects {
				visit(child)
			}
		}
		if split, ok := obj.(*container.Split); ok {
			visit(split.Leading)
			visit(split.Trailing)
		}
	}
	visit(root)
	return entry, save
}

func TestNoteSaveFailureKeepsEditorOpen(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".biomelab"), []byte("blocks draft directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := git.Worktree{Path: root, Branch: "feature"}
	origin := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	other := &repoEntry{state: &RepoState{}}
	a := &App{fyneApp: fa, repos: []*repoEntry{origin, other}, active: 0}
	a.openNoteDialog(wt)
	w := a.noteWindows[root]
	defer w.Close()
	entry, save := findNoteControls(w.Content())
	if entry == nil || save == nil {
		t.Fatal("note editor controls missing")
	}
	entry.SetText("unsaved draft")
	save.Tapped(nil)
	if a.noteWindows[root] != w || entry.Text != "unsaved draft" {
		t.Fatal("failed save closed the editor or discarded its text")
	}
}
