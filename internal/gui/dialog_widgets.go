package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Custom Focusable widgets used by dialogs.
//
// Fyne's built-in widgets don't all bind the keys users expect inside a modal:
// Entry doesn't dismiss on Escape, Button only fires on Space (not Enter),
// Select doesn't accept Enter to confirm. These wrappers add the missing keys
// so dialogs can be operated entirely from the keyboard once focused.

// dialogEntry is a single-line text Entry that dismisses its parent dialog on
// Escape. Enter submission is wired via the standard Entry.OnSubmitted hook.
type dialogEntry struct {
	widget.Entry
	onEscape func()
}

func newDialogEntry(onEscape func()) *dialogEntry {
	e := &dialogEntry{onEscape: onEscape}
	e.ExtendBaseWidget(e)
	return e
}

func (e *dialogEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape {
		if e.onEscape != nil {
			e.onEscape()
		}
		return
	}
	e.Entry.TypedKey(key)
}

// dialogSelect is a Select that confirms on Enter and dismisses on Escape.
type dialogSelect struct {
	widget.Select
	onEnter  func()
	onEscape func()
}

func newDialogSelect(options []string, onEnter, onEscape func()) *dialogSelect {
	s := &dialogSelect{onEnter: onEnter, onEscape: onEscape}
	s.Options = options
	s.ExtendBaseWidget(s)
	return s
}

func (s *dialogSelect) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		if s.onEscape != nil {
			s.onEscape()
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if s.onEnter != nil {
			s.onEnter()
		} else {
			s.Select.TypedKey(key)
		}
	default:
		s.Select.TypedKey(key)
	}
}

// dialogCheck is a Check that dismisses its parent dialog on Escape. Fyne
// sends keys to a focused Check instead of the canvas, so a dialog-level key
// capture cannot handle Escape while a kit checkbox has focus.
type dialogCheck struct {
	widget.Check
	onEscape func()
}

func newDialogCheck(text string, onChanged func(bool), onEscape func()) *dialogCheck {
	c := &dialogCheck{onEscape: onEscape}
	c.Text = text
	c.OnChanged = onChanged
	c.ExtendBaseWidget(c)
	return c
}

func (c *dialogCheck) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape {
		if c.onEscape != nil {
			c.onEscape()
		}
		return
	}
	c.Check.TypedKey(key)
}

// dialogButton is a Button that triggers on Enter (in addition to Space) and
// dismisses its dialog on Escape.
type dialogButton struct {
	widget.Button
	onEscape func()
}

func newDialogButton(text string, onTap, onEscape func()) *dialogButton {
	b := &dialogButton{onEscape: onEscape}
	b.Text = text
	b.OnTapped = onTap
	b.ExtendBaseWidget(b)
	return b
}

func (b *dialogButton) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		if b.onEscape != nil {
			b.onEscape()
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		b.Tapped(nil)
	default:
		b.Button.TypedKey(key)
	}
}

// dialogKeyCapture is an invisible focusable widget used by confirm-only
// dialogs (where Fyne renders the OK/Cancel buttons internally) to translate
// Enter into a confirm action and Escape into a dismiss action.
type dialogKeyCapture struct {
	widget.BaseWidget
	onEnter  func()
	onEscape func()
}

func newDialogKeyCapture(onEnter, onEscape func()) *dialogKeyCapture {
	w := &dialogKeyCapture{onEnter: onEnter, onEscape: onEscape}
	w.ExtendBaseWidget(w)
	return w
}

func (w *dialogKeyCapture) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (w *dialogKeyCapture) FocusGained()     {}
func (w *dialogKeyCapture) FocusLost()       {}
func (w *dialogKeyCapture) TypedRune(_ rune) {}

func (w *dialogKeyCapture) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if w.onEnter != nil {
			w.onEnter()
		}
	case fyne.KeyEscape:
		if w.onEscape != nil {
			w.onEscape()
		}
	}
}

// focusInDialog focuses the given widget on the parent canvas. The focus call
// is deferred via fyne.Do so it runs on the next event-loop tick — that is
// after the current key press has finished dispatching. Without the defer, a
// printable shortcut like 'f' would open the dialog, focus the Entry, and
// then GLFW's char callback would deliver the same 'f' to the focused Entry.
// Deferring keeps canvas.Focused() nil for the char callback so handleRune's
// dialogOpen guard swallows it.
func focusInDialog(parent fyne.Window, target fyne.Focusable) {
	if parent == nil || target == nil {
		return
	}
	fyne.Do(func() {
		parent.Canvas().Focus(target)
	})
}
