package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/notes"
)

type noteShortcutDriver struct {
	fyne.Driver
	modifiers fyne.KeyModifier
}

func (d noteShortcutDriver) HasSecondaryDisplay() bool { return false }

func (d noteShortcutDriver) CurrentKeyModifiers() fyne.KeyModifier { return d.modifiers }
func (d noteShortcutDriver) CreateSplashWindow() fyne.Window       { return d.CreateWindow("splash") }

type noteShortcutApp struct {
	fyne.App
	driver fyne.Driver
}

func (a noteShortcutApp) Driver() fyne.Driver { return a.driver }

func useNoteShortcutModifiers(t *testing.T, modifiers fyne.KeyModifier) {
	t.Helper()
	app := fyne.CurrentApp()
	fyne.SetCurrentApp(noteShortcutApp{App: app, driver: noteShortcutDriver{Driver: app.Driver(), modifiers: modifiers}})
	t.Cleanup(func() { fyne.SetCurrentApp(app) })
}

func noteKeyboardFixture(t *testing.T) (*App, fyne.Window, *noteEntry, fyne.Focusable) {
	t.Helper()
	fa := test.NewApp()
	t.Cleanup(fa.Quit)
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := git.Worktree{Path: root, Branch: "scratch"}
	a := &App{fyneApp: fa, repos: []*repoEntry{{state: &RepoState{Worktrees: []git.Worktree{wt}}}}}
	a.openNoteDialog(wt)
	w := a.noteWindows[root]
	t.Cleanup(func() {
		if a.noteWindows[root] == w {
			w.Close()
		}
	})
	body, _ := findNoteControls(w.Content())
	var title fyne.Focusable
	walkPolish(w.Content(), func(o fyne.CanvasObject) {
		switch e := o.(type) {
		case *noteEntry:
			if !e.MultiLine {
				e.SetText("feat: scratch notes")
				title = e
			}
		case *dialogEntry:
			e.SetText("feat: scratch notes")
			title = e
		}
	})
	if body == nil || title == nil {
		t.Fatal("note title/body missing")
	}
	body.SetText("# Scratch\n\nExact Markdown **body**")
	return a, w, body, title
}

func noteWorktreeContext(t *testing.T, w fyne.Window) *noteEntry {
	t.Helper()
	var context *noteEntry
	walkPolish(w.Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*noteEntry); ok && e.Disabled() && strings.HasPrefix(e.Text, "Branch: ") {
			context = e
		}
	})
	if context == nil {
		t.Fatal("read-only worktree context missing")
	}
	return context
}

func TestNoteSaveShortcutFromFocusedFields(t *testing.T) {
	for _, field := range []string{"Markdown", "title", "context", "canvas"} {
		for _, modifier := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
			t.Run(fmt.Sprintf("%s/%d", field, modifier), func(t *testing.T) {
				a, w, body, title := noteKeyboardFixture(t)
				path := a.repos[0].state.Worktrees[0].Path
				switch field {
				case "Markdown":
					w.Canvas().Focus(body)
				case "title":
					w.Canvas().Focus(title)
				case "context":
					useNoteShortcutModifiers(t, modifier)
					w.Canvas().Focus(noteWorktreeContext(t, w))
				case "canvas":
					w.Canvas().Unfocus()
				}
				c := &keyboardCanvas{Canvas: w.Canvas(), modifiers: modifier}
				c.press(fyne.KeyS)
				if a.noteWindows[path] != nil {
					t.Fatal("focused Save shortcut did not close successful editor")
				}
				text, ok, err := notes.Read(path)
				if err != nil || !ok || text != body.Text+"\n" {
					t.Fatalf("saved body=%q exists=%v error=%v", text, ok, err)
				}
				text, ok, err = notes.ReadTitle(path)
				if err != nil || !ok || text != "feat: scratch notes" {
					t.Fatalf("saved title=%q exists=%v error=%v", text, ok, err)
				}
			})
		}
	}
}

func TestNoteFocusedEditingAndEscapePreserved(t *testing.T) {
	a, w, body, title := noteKeyboardFixture(t)
	path := a.repos[0].state.Worktrees[0].Path
	clipboard := a.fyneApp.Clipboard()
	for _, field := range []fyne.Focusable{body, title} {
		w.Canvas().Focus(field)
		c := &keyboardCanvas{Canvas: w.Canvas(), modifiers: fyne.KeyModifierShortcutDefault}
		c.press(fyne.KeyA)
		entry := field.(fyne.Shortcutable)
		entry.TypedShortcut(&fyne.ShortcutCopy{Clipboard: clipboard})
		if clipboard.Content() == "" {
			t.Fatal("focused select-all/copy lost text")
		}
		before := clipboard.Content()
		field.TypedRune('x')
		entry.TypedShortcut(&fyne.ShortcutUndo{})
		entry.TypedShortcut(&fyne.ShortcutSelectAll{})
		entry.TypedShortcut(&fyne.ShortcutCopy{Clipboard: clipboard})
		if clipboard.Content() != before {
			t.Fatal("focused undo did not restore original text")
		}
	}
	c := &keyboardCanvas{Canvas: w.Canvas()}
	c.press(fyne.KeyEscape)
	if a.noteWindows[path] != nil || notes.Exists(path) {
		t.Fatal("Escape failed to cancel unsaved editor")
	}
}

func TestNoteEscapeFromContextTraversalAndUnfocusedCanvas(t *testing.T) {
	for _, route := range []string{"context", "previousFromTitle", "canvas"} {
		t.Run(route, func(t *testing.T) {
			a, w, _, title := noteKeyboardFixture(t)
			path := a.repos[0].state.Worktrees[0].Path
			switch route {
			case "context":
				context := noteWorktreeContext(t, w)
				w.Canvas().Focus(context)
				if w.Canvas().Focused() != context {
					t.Fatal("worktree context could not receive selection focus")
				}
			case "previousFromTitle":
				w.Canvas().Focus(title)
				w.Canvas().FocusPrevious()
				if w.Canvas().Focused() == nil || w.Canvas().Focused() == title {
					t.Fatal("reverse traversal did not leave title")
				}
			case "canvas":
				w.Canvas().Unfocus()
			}
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyEscape)
			if a.noteWindows[path] != nil || notes.Exists(path) {
				t.Fatal("Escape did not discard unsaved notes from context/traversal/canvas")
			}
			if _, exists, err := notes.ReadTitle(path); err != nil || exists {
				t.Fatalf("Escape wrote note title: exists=%v error=%v", exists, err)
			}
		})
	}
}

func TestNoteWorktreeContextPreservesSelectionCopyAndRejectsEdits(t *testing.T) {
	a, w, _, _ := noteKeyboardFixture(t)
	context := noteWorktreeContext(t, w)
	w.Canvas().Focus(context)
	expected := "Branch: scratch\nPath: " + a.repos[0].state.Worktrees[0].Path
	if context.Text != expected {
		t.Fatal("worktree context text changed")
	}
	test.DoubleTap(context)
	selected := context.SelectedText()
	if selected == "" || !strings.Contains(expected, selected) {
		t.Fatal("read-only worktree context lost mouse text selection")
	}
	clipboard := a.fyneApp.Clipboard()
	context.TypedShortcut(&fyne.ShortcutCopy{Clipboard: clipboard})
	if clipboard.Content() != selected {
		t.Fatal("read-only worktree context lost selection copy")
	}
	context.TypedRune('x')
	for _, key := range []fyne.KeyName{fyne.KeyBackspace, fyne.KeyDelete, fyne.KeyReturn} {
		context.TypedKey(&fyne.KeyEvent{Name: key})
	}
	context.TypedShortcut(&fyne.ShortcutCut{Clipboard: clipboard})
	clipboard.SetContent("attempted replacement")
	context.TypedShortcut(&fyne.ShortcutPaste{Clipboard: clipboard})
	if context.Text != expected {
		t.Fatal("read-only worktree context accepted editing input")
	}
}

func TestNoteContextSaveKeyDownRequiresExactModifier(t *testing.T) {
	for _, modifier := range []fyne.KeyModifier{0, fyne.KeyModifierShift, fyne.KeyModifierControl | fyne.KeyModifierAlt, fyne.KeyModifierSuper | fyne.KeyModifierShift, fyne.KeyModifierControl | fyne.KeyModifierSuper} {
		t.Run(fmt.Sprint(modifier), func(t *testing.T) {
			a, w, _, _ := noteKeyboardFixture(t)
			useNoteShortcutModifiers(t, modifier)
			context := noteWorktreeContext(t, w)
			context.KeyDown(&fyne.KeyEvent{Name: fyne.KeyS})
			path := a.repos[0].state.Worktrees[0].Path
			if a.noteWindows[path] != w || notes.Exists(path) {
				t.Fatal("plain or modified S unexpectedly saved read-only context")
			}
		})
	}
}

func TestNoteSaveShortcutFailureKeepsDraftAndOrigin(t *testing.T) {
	a, w, body, _ := noteKeyboardFixture(t)
	origin := a.repos[0]
	path := origin.state.Worktrees[0].Path
	if err := os.WriteFile(filepath.Join(path, ".biomelab"), []byte("blocks save"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := &repoEntry{state: &RepoState{}}
	a.repos = append(a.repos, other)
	a.active = 1
	w.Canvas().Focus(body)
	c := &keyboardCanvas{Canvas: w.Canvas(), modifiers: fyne.KeyModifierSuper}
	c.press(fyne.KeyS)
	if a.noteWindows[path] != w || body.Text != "# Scratch\n\nExact Markdown **body**" || !strings.Contains(origin.state.StatusMessage, "Note save failed") || other.state.StatusMessage != "" {
		t.Fatal("failed keyboard Save lost draft/window or originating repo ownership")
	}
}

func noteFooterControl(t *testing.T, w fyne.Window, text string) fyne.Focusable {
	t.Helper()
	var found fyne.Focusable
	walkPolish(w.Content(), func(o fyne.CanvasObject) {
		switch button := o.(type) {
		case *dialogButton:
			if button.Text == text {
				found = button
			}
		case *escapeButton:
			if button.Text == text {
				found = button
			}
		}
	})
	if found == nil {
		t.Fatalf("note footer %q missing", text)
	}
	return found
}

func TestNoteFooterEscapeDiscardsDraftWithoutChangingSavedNotes(t *testing.T) {
	for _, control := range []string{"Save", "Cancel", "Delete note"} {
		t.Run(control, func(t *testing.T) {
			a, w, _, _ := noteKeyboardFixture(t)
			wt := a.repos[0].state.Worktrees[0]
			if err := notes.Write(wt.Path, "saved body"); err != nil {
				t.Fatal(err)
			}
			if err := notes.WriteTitle(wt.Path, "feat: saved title"); err != nil {
				t.Fatal(err)
			}
			w.Close()
			a.openNoteDialog(wt)
			w = a.noteWindows[wt.Path]
			walkPolish(w.Content(), func(o fyne.CanvasObject) {
				if entry, ok := o.(*noteEntry); ok && !entry.Disabled() {
					entry.SetText("unsaved draft")
				}
			})
			footer := noteFooterControl(t, w, control)
			w.Canvas().Focus(footer)
			c := &keyboardCanvas{Canvas: w.Canvas()}
			if control != "Save" {
				// Escape support must not add Enter activation to the
				// ordinary Cancel/Delete controls.
				c.press(fyne.KeyReturn)
				if a.noteWindows[wt.Path] != w || w.Canvas().Overlays().Top() != nil {
					t.Fatal("Enter changed ordinary footer activation")
				}
			}
			c.press(fyne.KeyEscape)
			if a.noteWindows[wt.Path] != nil {
				t.Fatal("focused footer Escape left the editor open")
			}
			body, ok, err := notes.Read(wt.Path)
			if err != nil || !ok || body != "saved body\n" {
				t.Fatalf("Escape changed saved body: %q %v", body, err)
			}
			title, ok, err := notes.ReadTitle(wt.Path)
			if err != nil || !ok || title != "feat: saved title" {
				t.Fatalf("Escape changed saved title: %q %v", title, err)
			}
		})
	}
}

func TestNoteFocusedSaveFooterActivatesAndPersists(t *testing.T) {
	for _, action := range []string{"Enter", "click", "Cmd+S"} {
		t.Run(action, func(t *testing.T) {
			a, w, body, _ := noteKeyboardFixture(t)
			path := a.repos[0].state.Worktrees[0].Path
			save := noteFooterControl(t, w, "Save")
			w.Canvas().Focus(save)
			c := &keyboardCanvas{Canvas: w.Canvas()}
			switch action {
			case "Enter":
				c.press(fyne.KeyReturn)
			case "click":
				test.Tap(save.(fyne.Tappable))
			case "Cmd+S":
				c.modifiers = fyne.KeyModifierSuper
				c.press(fyne.KeyS)
			}
			if a.noteWindows[path] != nil {
				t.Fatal("focused Save did not close successful editor")
			}
			saved, ok, err := notes.Read(path)
			if err != nil || !ok || saved != body.Text+"\n" {
				t.Fatalf("Save failed to persist body: %q %v", saved, err)
			}
			title, ok, err := notes.ReadTitle(path)
			if err != nil || !ok || title != "feat: scratch notes" {
				t.Fatalf("Save failed to persist title: %q %v", title, err)
			}
		})
	}
}
