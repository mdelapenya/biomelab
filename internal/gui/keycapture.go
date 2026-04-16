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
