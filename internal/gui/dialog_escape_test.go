package gui

import (
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
)

func selectableDialogText(root fyne.CanvasObject, text string) fyne.Focusable {
	var found fyne.Focusable
	walkPolish(root, func(o fyne.CanvasObject) {
		if entry, ok := o.(*noteEntry); ok && entry.Disabled() && entry.Text == text {
			found = entry
		}
		if label, ok := o.(*widget.Label); ok && label.Selectable && label.Text == text {
			walkPolish(label, func(child fyne.CanvasObject) {
				if focus, ok := child.(fyne.Focusable); ok {
					found = focus
				}
			})
		}
	})
	return found
}

func selectModalContext(t *testing.T, w fyne.Window, focus fyne.Focusable) {
	t.Helper()
	point := fyne.PointEvent{Position: fyne.NewPos(1, 1)}
	mouse := desktop.MouseEvent{PointEvent: point, Button: desktop.MouseButtonPrimary}
	// test.DoubleTap unconditionally unfocuses disabled controls. Native
	// desktop selection instead lets Entry.MouseDown request read-only focus.
	focus.(desktop.Mouseable).MouseDown(&mouse)
	focus.(desktop.Mouseable).MouseUp(&mouse)
	focus.(fyne.DoubleTappable).DoubleTapped(&point)
	if w.Canvas().Focused() != focus {
		t.Fatalf("selection lost actual context focus: %T", w.Canvas().Focused())
	}
}

func TestModalSelectableContextEscapePreservesCopyAndCancelsOnce(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	issue := func(w fyne.Window, done, confirm func()) dialog.Dialog {
		return showIssuePreview(w, github.IssueInfo{Number: 1, Title: "Owned fixture", Body: "Context", State: "OPEN"}, "owner/repo", "/owned/repo", "main", done, func(string) error { confirm(); return nil })
	}
	send := func(w fyne.Window, done, confirm func()) dialog.Dialog {
		return showSendPRConfirm(w, "owned", git.RemoteInfo{Name: "origin", Repo: "owner/repo"}, nil, false, done, func(bool) { confirm() }, nil)
	}
	for _, tc := range []struct {
		name, text string
		show       func(fyne.Window, func(), func()) dialog.Dialog
	}{
		{"dirty warning branch", "Send PR for: owned", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showSendPRDirtyWarning(w, "owned", true, false, done, confirm)
		}},
		{"send PR branch", "Branch: owned", send},
		{"send PR remote", "Remote: origin (owner/repo)", send},
		{"issue source", "Source repository: owner/repo", issue},
		{"issue destination", "Destination repository: /owned/repo", issue},
		{"issue base", "Base: the main checkout's current local HEAD when Create is pressed (currently main).", issue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fa.NewWindow("owned selectable context")
			defer w.Close()
			w.SetContent(container.NewStack())
			w.Resize(fyne.NewSize(900, 700))
			w.Show()
			a := &App{window: w}
			closed, confirmed := 0, 0
			done := a.openDialog()
			a.activeDialog = tc.show(w, func() { closed++; done() }, func() { confirmed++ })
			focus := selectableDialogText(w.Canvas().Overlays().Top(), tc.text)
			if focus == nil {
				t.Fatal("complete selectable context missing")
			}
			w.Canvas().Focus(focus)
			if w.Canvas().Focused() != focus {
				t.Fatal("context cannot receive selection focus")
			}
			selectModalContext(t, w, focus)
			selected := focus.(interface{ SelectedText() string }).SelectedText()
			if selected == "" {
				t.Fatal("context lost text selection")
			}
			focus.(fyne.Shortcutable).TypedShortcut(&fyne.ShortcutCopy{Clipboard: fa.Clipboard()})
			if fa.Clipboard().Content() != selected {
				t.Fatal("context lost selection copy")
			}
			if entry, ok := focus.(*noteEntry); ok {
				entry.TypedRune('x')
				fa.Clipboard().SetContent("replacement")
				entry.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fa.Clipboard()})
				if entry.Text != tc.text {
					t.Fatal("read-only context accepted edits")
				}
			}
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyEscape)
			if closed != 1 || confirmed != 0 || a.dialogOpen || a.activeDialog != nil || w.Canvas().Overlays().Top() != nil {
				t.Fatalf("context Escape closed=%d confirmed=%d modal=%v", closed, confirmed, a.dialogOpen)
			}
		})
	}
}

func TestDialogTechnicalPreservesFullScrollableValueAtZoom(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		t.Run(string(variant), func(t *testing.T) {
			th := newBiomeTheme(variant)
			th.ZoomIn()
			fa.Settings().SetTheme(th)
			w := fa.NewWindow("narrow technical context")
			defer w.Close()
			text := "Branch: " + strings.Repeat("complete-branch/", 30) + "final-component"
			scroll := dialogTechnical(text, func() { w.Close() })
			w.SetContent(scroll)
			w.Resize(fyne.NewSize(640, 80))
			w.Show()
			entry := scroll.Content.(*noteEntry)
			if entry.MultiLine || entry.Size().Width <= scroll.Size().Width || scroll.Size().Width > 640 || scroll.Size().Height > 80 {
				t.Fatalf("technical value lost compact horizontal scroll: entry=%v viewport=%v", entry.Size(), scroll.Size())
			}
			scroll.ScrollToOffset(fyne.NewPos(entry.Size().Width-scroll.Size().Width, 0))
			if scroll.Offset.X <= 0 {
				t.Fatal("end of full technical value is not reachable by horizontal scrolling")
			}
			entry.TypedShortcut(&fyne.ShortcutSelectAll{})
			entry.TypedShortcut(&fyne.ShortcutCopy{Clipboard: fa.Clipboard()})
			if entry.Text != text || fa.Clipboard().Content() != text {
				t.Fatal("technical value was truncated or copied incompletely")
			}
		})
	}
}

type confirmationFixture struct {
	name string
	show func(fyne.Window, func(), func()) dialog.Dialog
}

func confirmationFixtures() []confirmationFixture {
	return []confirmationFixture{
		{"create sandbox", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showConfirmCreateSandbox(w, "owned", "shell", "/owned/repo", nil, done, confirm)
		}},
		{"remove sandbox", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showConfirmRemoveSandbox(w, "owned", done, confirm)
		}},
		{"remove mode", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showConfirmRemoveMode(w, "owned", "host", false, done, confirm)
		}},
		{"dirty warning", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showSendPRDirtyWarning(w, "owned", true, false, done, confirm)
		}},
		{"send PR", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showSendPRConfirm(w, "owned", git.RemoteInfo{Name: "origin"}, nil, false, done, func(bool) { confirm() }, nil)
		}},
		{"register existing", func(w fyne.Window, done, confirm func()) dialog.Dialog {
			return showConfirmRegisterExistingSandbox(w, "owned", "/owned/repo", done, confirm, func() {})
		}},
	}
}

func TestConfirmationEscapeFromDefaultAndEitherFooter(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, fixture := range confirmationFixtures() {
		for _, focus := range []string{"default", "cancel", "confirm"} {
			t.Run(fixture.name+"/"+focus, func(t *testing.T) {
				w := fa.NewWindow("confirmation")
				defer w.Close()
				w.SetContent(container.NewStack())
				w.Resize(fyne.NewSize(900, 700))
				w.Show()
				a := &App{window: w}
				closed, confirmed := 0, 0
				done := a.openDialog()
				d := fixture.show(w, func() { closed++; done() }, func() { confirmed++ })
				a.activeDialog = d
				confirm := d.(*keyboardConfirmDialog)
				switch focus {
				case "cancel":
					w.Canvas().Focus(confirm.cancel)
				case "confirm":
					w.Canvas().Focus(confirm.confirm)
				}
				c := &keyboardCanvas{Canvas: w.Canvas()}
				c.press(fyne.KeyEscape)
				if closed != 1 || confirmed != 0 || a.dialogOpen || a.activeDialog != nil || w.Canvas().Overlays().Top() != nil {
					t.Fatalf("Escape closed=%d confirmed=%d modal=%v", closed, confirmed, a.dialogOpen)
				}
			})
		}
	}
}

func TestConfirmationInitialEnterKeepsExistingResponse(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, fixture := range confirmationFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			w := fa.NewWindow("initial Enter")
			defer w.Close()
			closed, confirmed := 0, 0
			fixture.show(w, func() { closed++ }, func() { confirmed++ })
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyReturn)
			wantClosed := 1
			if fixture.name == "dirty warning" {
				wantClosed = 0 // continuation retains ownership for the next stage
			}
			if closed != wantClosed || confirmed != 1 || w.Canvas().Overlays().Top() != nil {
				t.Fatalf("Enter closed=%d confirmed=%d", closed, confirmed)
			}
		})
	}
}

func TestKeyboardConfirmPreservesButtonActivationAndCallbackOrder(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, action := range []string{"confirm Space", "confirm click", "cancel Space", "cancel click", "Escape"} {
		t.Run(action, func(t *testing.T) {
			w := fa.NewWindow("response order")
			defer w.Close()
			var events []string
			d := newKeyboardConfirm("Decision", "Yes", "No", widget.NewLabel("owned"), func(ok bool) {
				if ok {
					events = append(events, "confirmed")
				} else {
					events = append(events, "canceled")
				}
			}, w)
			d.SetOnClosed(func() { events = append(events, "closed") })
			d.Show()
			// Standard footer Enter stays inert; the existing key capture
			// still controls the dialog's initial Enter confirmation.
			w.Canvas().Focus(d.confirm)
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyReturn)
			if len(events) != 0 || w.Canvas().Overlays().Top() == nil {
				t.Fatal("footer gained unexpected Enter activation")
			}
			switch action {
			case "confirm Space":
				c.press(fyne.KeySpace)
			case "confirm click":
				test.Tap(d.confirm)
			case "cancel Space":
				w.Canvas().Focus(d.cancel)
				c.press(fyne.KeySpace)
			case "cancel click":
				test.Tap(d.cancel)
			case "Escape":
				c.press(fyne.KeyEscape)
			}
			response := "canceled"
			if action == "confirm Space" || action == "confirm click" {
				response = "confirmed"
			}
			if !reflect.DeepEqual(events, []string{response, "closed"}) {
				t.Fatalf("response order=%v", events)
			}
		})
	}
}

func TestSendPREscapeFromNotesControlsDoesNotPublishOrReview(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, focus := range []string{"checkbox", "Review notes"} {
		t.Run(focus, func(t *testing.T) {
			w := fa.NewWindow("notes confirmation")
			defer w.Close()
			a := &App{window: w}
			closed, confirmed, reviewed := 0, 0, 0
			done := a.openDialog()
			a.activeDialog = showSendPRConfirm(w, "owned", git.RemoteInfo{Name: "origin"}, nil, true, func() { closed++; done() }, func(bool) { confirmed++ }, func() { reviewed++ })
			var target fyne.Focusable
			walkPolish(w.Canvas().Overlays().Top().(*widget.PopUp).Content, func(o fyne.CanvasObject) {
				if check, ok := o.(*dialogCheck); ok && focus == "checkbox" {
					target = check
				}
				if button, ok := o.(*escapeButton); ok && button.Text == focus {
					target = button
				}
			})
			if target == nil {
				t.Fatalf("notes control %q missing", focus)
			}
			w.Canvas().Focus(target)
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyEscape)
			if closed != 1 || confirmed != 0 || reviewed != 0 || a.dialogOpen || w.Canvas().Overlays().Top() != nil {
				t.Fatal("notes control Escape confirmed, reviewed, or retained the modal")
			}
		})
	}
}

func TestDismissFooterEscapeKeepsExistingChoicesUnsubmitted(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, flow := range []string{"mode selection", "remote selection", "loading kits"} {
		t.Run(flow, func(t *testing.T) {
			w := fa.NewWindow("dismiss")
			defer w.Close()
			closed, submitted := 0, 0
			done := func() { closed++ }
			switch flow {
			case "mode selection":
				showModeSelection(w, done, func() { submitted++ }, func() { submitted++ })
			case "remote selection":
				showSendPRRemoteSelection(w, []git.RemoteInfo{{Name: "origin"}}, done, func(int) { submitted++ })
			case "loading kits":
				d := newKeyboardDismiss("Loading Kits", "Cancel", dialogBusy("Loading…"), w)
				d.SetOnClosed(done)
				d.Show()
			}
			var cancel *escapeButton
			walkSetupContent(w.Canvas().Overlays().Top().(*widget.PopUp).Content, func(o fyne.CanvasObject) {
				if button, ok := o.(*escapeButton); ok && button.Text == "Cancel" {
					cancel = button
				}
			})
			if cancel == nil {
				t.Fatal("dismiss footer missing")
			}
			w.Canvas().Focus(cancel)
			c := &keyboardCanvas{Canvas: w.Canvas()}
			c.press(fyne.KeyEscape)
			if closed != 1 || submitted != 0 || w.Canvas().Overlays().Top() != nil {
				t.Fatal("Cancel Escape submitted a choice or retained the dialog")
			}
		})
	}
}
