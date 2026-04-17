package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// setupKeyHandlers registers global key handlers on the canvas.
//
// We use desktop.Canvas.SetOnKeyDown for ALL key events (including Tab).
// This fires BEFORE Fyne's internal Tab/shortcut consumption (window.go:700),
// so Tab reaches our handler.
//
// For character input (runes), we use Canvas.SetOnTypedRune which fires
// when no widget has focus (which is always true since we have no Focusable widgets).
func setupKeyHandlers(c fyne.Canvas, onKey func(fyne.KeyName), onRune func(rune)) {
	// desktop.Canvas.SetOnKeyDown fires for every key PRESS before
	// Fyne's focus/shortcut/tab interception. This is the only way
	// to receive Tab and Escape events (both are consumed by Fyne
	// before reaching SetOnTypedKey).
	//
	// We do NOT also set SetOnTypedKey because both fire for the
	// initial press — that would double-fire the handler, causing
	// navigation to skip every other card.
	if dc, ok := c.(desktop.Canvas); ok {
		dc.SetOnKeyDown(func(ev *fyne.KeyEvent) {
			onKey(ev.Name)
		})
	}

	// SetOnTypedRune fires for character input when no widget has focus.
	c.SetOnTypedRune(func(r rune) {
		onRune(r)
	})
}

// registerZoomShortcuts adds Ctrl+Plus, Ctrl+Minus, Ctrl+0 for font scaling.
// These use Canvas.AddShortcut with KeyModifierControl which works because
// Fyne's triggersShortcut requires modifier != 0 (window.go:817).
func registerZoomShortcuts(c fyne.Canvas, t *biomeTheme, a fyne.App, onZoom func()) {
	zoomInFn := func(_ fyne.Shortcut) {
		t.ZoomIn()
		a.Settings().SetTheme(t)
		if onZoom != nil {
			onZoom()
		}
	}
	zoomOutFn := func(_ fyne.Shortcut) {
		t.ZoomOut()
		a.Settings().SetTheme(t)
		if onZoom != nil {
			onZoom()
		}
	}
	zoomResetFn := func(_ fyne.Shortcut) {
		t.ZoomReset()
		a.Settings().SetTheme(t)
		if onZoom != nil {
			onZoom()
		}
	}

	// Register for both Ctrl (Linux/Windows) and Super/Cmd (macOS).
	// Also register Ctrl+Shift+= since + is Shift+= on most keyboards.
	for _, mod := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: mod}, zoomInFn)
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: mod | fyne.KeyModifierShift}, zoomInFn)
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPlus, Modifier: mod}, zoomInFn)
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyMinus, Modifier: mod}, zoomOutFn)
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.Key0, Modifier: mod}, zoomResetFn)
	}
}

// registerThemeToggleShortcut binds Ctrl+T (Linux/Windows) and Cmd+T (macOS)
// to the provided callback. AddShortcut requires a non-zero modifier.
func registerThemeToggleShortcut(c fyne.Canvas, onToggle func()) {
	handler := func(_ fyne.Shortcut) {
		if onToggle != nil {
			onToggle()
		}
	}
	for _, mod := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyT, Modifier: mod}, handler)
	}
}

// registerTerminalKeyShortcuts forwards modified arrow/backspace combos to
// the embedded terminal panel as the escape sequences zsh/bash/readline
// expect. fyne-io/terminal's TypedKey path drops modifier info, so
// Option+Left arrives as plain `\e[D` and the user's `bindkey '\eb'
// backward-word` (or equivalent) never fires. We intercept at the canvas
// level and write the sequence directly to the active terminal's PTY.
//
// Bindings chosen to match macOS Terminal.app / iTerm2 "natural text
// editing" defaults, which is what most zsh/bash configs assume:
//
//	Alt+Left        → ESC b   (backward-word)
//	Alt+Right       → ESC f   (forward-word)
//	Alt+Backspace   → ESC DEL (kill previous word)
//	Cmd+Left        → ESC OH  (beginning of line)
//	Cmd+Right       → ESC OF  (end of line)
//	Cmd+Backspace   → ^U      (kill to beginning of line)
//
// Fires only when the terminal panel has focus; otherwise the event falls
// through to dashboard handlers (currently a no-op for these combos).
func registerTerminalKeyShortcuts(c fyne.Canvas, a *App) {
	write := func(seq []byte) func(fyne.Shortcut) {
		return func(_ fyne.Shortcut) {
			if a.focus != focusTerminal {
				return
			}
			t := a.termPanel.Active()
			if t == nil {
				return
			}
			_, _ = t.Write(seq)
		}
	}

	type binding struct {
		mod fyne.KeyModifier
		key fyne.KeyName
		seq []byte
	}
	esc := byte(0x1b)
	del := byte(0x7f)
	bindings := []binding{
		// Word motion — Option on macOS, Alt on Linux/Windows
		{fyne.KeyModifierAlt, fyne.KeyLeft, []byte{esc, 'b'}},
		{fyne.KeyModifierAlt, fyne.KeyRight, []byte{esc, 'f'}},
		{fyne.KeyModifierAlt, fyne.KeyBackspace, []byte{esc, del}},

		// Line motion — Cmd on macOS; Super on Linux for symmetry
		{fyne.KeyModifierSuper, fyne.KeyLeft, []byte{esc, 'O', 'H'}},
		{fyne.KeyModifierSuper, fyne.KeyRight, []byte{esc, 'O', 'F'}},
		{fyne.KeyModifierSuper, fyne.KeyBackspace, []byte{0x15}}, // ^U
	}
	for _, b := range bindings {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: b.key, Modifier: b.mod}, write(b.seq))
	}
}
