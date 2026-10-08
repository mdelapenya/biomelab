package terminal

import (
	"fmt"
	"runtime"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

var lastKeyTime = time.Now()

// TypedRune is called when the user types a visible character
func (t *Terminal) TypedRune(r rune) {
	lastKeyTime = time.Now()
	b := make([]byte, utf8.UTFMax)
	size := utf8.EncodeRune(b, r)
	_, _ = t.in.Write(b[:size])
}

// TypedKey will be called if a non-printable keyboard event occurs
func (t *Terminal) TypedKey(e *fyne.KeyEvent) {
	lastKeyTime = time.Now()
	if t.modifiedCursor(e.Name, t.modifiers()) {
		return
	}
	if e.Name == fyne.KeyTab && t.keyboardState.shiftPressed {
		_, _ = t.in.Write([]byte("\x1b[Z"))
		return
	}
	if t.keyboardState.shiftPressed {
		t.keyTypedWithShift(e)
		return
	}

	switch e.Name {
	case fyne.KeyReturn:
		_, _ = t.in.Write([]byte{'\r'})
	case fyne.KeyEnter:
		if t.newLineMode {
			_, _ = t.in.Write([]byte{'\r'})
			return
		}
		_, _ = t.in.Write([]byte{'\n'})
	case fyne.KeyTab:
		_, _ = t.in.Write([]byte{'\t'})
	case fyne.KeyF1:
		_, _ = t.in.Write([]byte{asciiEscape, 'O', 'P'})
	case fyne.KeyF2:
		_, _ = t.in.Write([]byte{asciiEscape, 'O', 'Q'})
	case fyne.KeyF3:
		_, _ = t.in.Write([]byte{asciiEscape, 'O', 'R'})
	case fyne.KeyF4:
		_, _ = t.in.Write([]byte{asciiEscape, 'O', 'S'})
	case fyne.KeyF5:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '5', '~'})
	case fyne.KeyF6:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '7', '~'})
	case fyne.KeyF7:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '8', '~'})
	case fyne.KeyF8:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '9', '~'})
	case fyne.KeyF9:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '0', '~'})
	case fyne.KeyF10:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '1', '~'})
	case fyne.KeyF11:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '3', '~'})
	case fyne.KeyF12:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '4', '~'})
	case fyne.KeyEscape:
		_, _ = t.in.Write([]byte{asciiEscape})
	case fyne.KeyBackspace:
		_, _ = t.in.Write([]byte{0x7f})
	case fyne.KeyDelete:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '3', '~'})
	case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight:
		t.typeCursorKey(e.Name)
	case fyne.KeyPageUp:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '5', '~'})
	case fyne.KeyPageDown:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '6', '~'})
	case fyne.KeyHome:
		t.typeCursorKey(e.Name)
	case fyne.KeyInsert:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '~'})
	case fyne.KeyEnd:
		t.typeCursorKey(e.Name)
	}
}

func (t *Terminal) keyTypedWithShift(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyF1:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '5', '~'})
	case fyne.KeyF2:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '6', '~'})
	case fyne.KeyF3:
		_, _ = t.in.Write([]byte{asciiEscape, 'O', 'R', ';', '2', '~'})
	case fyne.KeyF4:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', ';', '2', 'S'})
	case fyne.KeyF5:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '5', ';', '2', '~'})
	case fyne.KeyF6:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '7', ';', '2', '~'})
	case fyne.KeyF7:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '8', ';', '2', '~'})
	case fyne.KeyF8:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', '9', ';', '2', '~'})
	case fyne.KeyF9:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '0', ';', '2', '~'})
	case fyne.KeyF10:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '1', ';', '2', '~'})
	case fyne.KeyF11:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '3', ';', '2', '~'})
	case fyne.KeyF12:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', '4', ';', '2', '~'})
	case fyne.KeyPageUp:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '5', ';', '2', '~'})
	case fyne.KeyPageDown:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '6', ';', '2', '~'})
	case fyne.KeyHome:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', ';', '2', 'H'})
	case fyne.KeyInsert:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '2', ';', '2', '~'})
	case fyne.KeyDelete:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '3', ';', '2', '~'})
	case fyne.KeyEnd:
		_, _ = t.in.Write([]byte{asciiEscape, '[', '1', ';', '2', 'F'})
	case fyne.KeyUp:
		_, _ = t.in.Write([]byte{asciiEscape, '[', 'A', ';', '2'})
	case fyne.KeyDown:
		_, _ = t.in.Write([]byte{asciiEscape, '[', 'B', ';', '2'})
	case fyne.KeyLeft:
		_, _ = t.in.Write([]byte{asciiEscape, '[', 'D', ';', '2'})
	case fyne.KeyRight:
		_, _ = t.in.Write([]byte{asciiEscape, '[', 'C', ';', '2'})
	}
}

func (t *Terminal) trackKeyboardState(down bool, e *fyne.KeyEvent) {
	switch e.Name {
	case desktop.KeyShiftLeft:
		t.keyboardState.shiftPressed = down
	case desktop.KeyAltLeft:
		t.keyboardState.altPressed = down
	case desktop.KeyControlLeft:
		t.keyboardState.ctrlPressed = down
	case desktop.KeyShiftRight:
		t.keyboardState.shiftPressed = down
	case desktop.KeyAltRight:
		t.keyboardState.altPressed = down
	case desktop.KeyControlRight:
		t.keyboardState.ctrlPressed = down
	}
}

// KeyDown is called when we get a down key event
func (t *Terminal) KeyDown(e *fyne.KeyEvent) {
	t.trackKeyboardState(true, e)
}

// KeyUp is called when we get an up key event
func (t *Terminal) KeyUp(e *fyne.KeyEvent) {
	t.trackKeyboardState(false, e)
}

// FocusGained notifies the terminal that it has focus
func (t *Terminal) FocusGained() {
	t.focused = true
	t.Refresh()
}

// TypedShortcut handles key combinations, we pass them on to the tty.
func (t *Terminal) TypedShortcut(s fyne.Shortcut) {
	if ds, ok := s.(*desktop.CustomShortcut); ok {
		if ds.KeyName == fyne.KeySpace && ds.Modifier == fyne.KeyModifierControl|fyne.KeyModifierShift && t.OnReturnToWorkspace != nil {
			t.OnReturnToWorkspace()
			return
		}
		if t.modifiedCursor(ds.KeyName, ds.Modifier) {
			return
		}
		if ds.KeyName == fyne.KeyBackspace && ds.Modifier == fyne.KeyModifierAlt {
			_, _ = t.in.Write([]byte{27, 127})
			return
		}
		t.ShortcutHandler.TypedShortcut(s) // it's not clear how we can check if this consumed the event

		// handle CTRL+A to CTRL+_ and everything in-between
		if ds.Modifier == fyne.KeyModifierControl && len(ds.KeyName) > 0 {
			char := ds.KeyName[0]
			off := char - 'A' + 1
			switch {
			case ds.Key() == fyne.KeySpace:
				fallthrough
			case ds.Key() == "@":
				off = 0
				fallthrough
			case char >= 'A' && char <= '_':
				_, _ = t.in.Write([]byte{off})
			}
		}
		return
	}

	if runtime.GOOS == "darwin" {
		// do the default thing for macOS as they separate ctrl/cmd
		t.ShortcutHandler.TypedShortcut(s)
	} else {
		// we need to override the default ctrl-X/C/V/A for non-mac and do it ourselves
		switch sh := s.(type) {
		case *fyne.ShortcutCut:
			if sh.Secondary {
				// shift+del - cut is no-op
			} else {
				_, _ = t.in.Write([]byte{0x18})
			}
		case *fyne.ShortcutCopy:
			if sh.Secondary {
				t.copySelectedText(t.selectClipboard()) // ctrl+ins
			} else {
				_, _ = t.in.Write([]byte{0x3})
			}
		case *fyne.ShortcutPaste:
			if sh.Secondary {
				t.pasteText(t.selectClipboard()) // shift+ins
			} else {
				_, _ = t.in.Write([]byte{0x16})
			}
		case *fyne.ShortcutUndo:
			_, _ = t.in.Write([]byte{0x1a})
		case *fyne.ShortcutRedo:
			_, _ = t.in.Write([]byte{0x19})
		case *fyne.ShortcutSelectAll:
			_, _ = t.in.Write([]byte{0x1})
		}
	}
}

// FocusLost tells the terminal it no longer has focus
func (t *Terminal) FocusLost() {
	t.focused = false
	// the key up for anything held down will go to whatever is focused next
	t.keyboardState.shiftPressed = false
	t.keyboardState.ctrlPressed = false
	t.keyboardState.altPressed = false
	t.Refresh()
}

// Focused is used to determine if this terminal currently has focus
func (t *Terminal) Focused() bool {
	return t.focused
}

func (t *Terminal) typeCursorKey(key fyne.KeyName) {
	cursorPrefix := byte('[')
	if t.bufferMode {
		cursorPrefix = 'O'
	}

	switch key {
	case fyne.KeyUp:
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'A'})
	case fyne.KeyDown:
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'B'})
	case fyne.KeyLeft:
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'D'})
	case fyne.KeyRight:
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'C'})
	case fyne.KeyHome: // like the arrows, Home and End follow DECCKM
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'H'})
	case fyne.KeyEnd:
		_, _ = t.in.Write([]byte{asciiEscape, cursorPrefix, 'F'})
	}
}

type discardWriter struct{}

func (d discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (d discardWriter) Close() error {
	return nil
}

func (t *Terminal) modifiers() fyne.KeyModifier {
	var m fyne.KeyModifier
	if t.keyboardState.shiftPressed {
		m |= fyne.KeyModifierShift
	}
	if t.keyboardState.altPressed {
		m |= fyne.KeyModifierAlt
	}
	if t.keyboardState.ctrlPressed {
		m |= fyne.KeyModifierControl
	}
	return m
}
func (t *Terminal) modifiedCursor(key fyne.KeyName, mods fyne.KeyModifier) bool {
	if mods == 0 || mods&fyne.KeyModifierSuper != 0 {
		return false
	}
	final := map[fyne.KeyName]string{fyne.KeyUp: "A", fyne.KeyDown: "B", fyne.KeyRight: "C", fyne.KeyLeft: "D", fyne.KeyHome: "H", fyne.KeyEnd: "F"}[key]
	if final == "" {
		return false
	}
	m := 1
	if mods&fyne.KeyModifierShift != 0 {
		m++
	}
	if mods&fyne.KeyModifierAlt != 0 {
		m += 2
	}
	if mods&fyne.KeyModifierControl != 0 {
		m += 4
	}
	_, _ = t.in.Write([]byte(fmt.Sprintf("\x1b[1;%d%s", m, final)))
	return true
}
