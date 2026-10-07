package terminal

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/require"
)

type terminalSizedTheme struct {
	fyne.Theme
	size float32
}

func (t terminalSizedTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameText {
		return t.size
	}
	return t.Theme.Size(name)
}

func TestEmbeddedCursorMatchesRenderedCells(t *testing.T) {
	for _, size := range []float32{10, 13, 14, 17, 24} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			term := New()
			window := test.NewWindow(container.NewThemeOverride(term, terminalSizedTheme{theme.DefaultTheme(), size}))
			defer window.Close()
			window.Resize(fyne.NewSize(1000, 300))
			term.Feed([]byte("prompt> abcdefghijklmnopqrstuvwxyz"))
			term.content.TextGrid.Refresh()
			term.Refresh()
			want := term.content.PositionForCursorLocation(term.rowOffset()+term.cursorRow, term.cursorCol)
			require.Equal(t, want, term.cursor.Position(), "cursor must use the rendered grid's cell advance")
		})
	}
}

// The software test driver normally executes fyne.Do inline. Native drivers
// queue it, which used to leave Feed's cursor refresh one output batch behind.
type queuedTerminalDriver struct {
	fyne.Driver
	pending []func()
}

func (d *queuedTerminalDriver) DoFromGoroutine(fn func(), wait bool) {
	if wait {
		fn()
		return
	}
	d.pending = append(d.pending, fn)
}

type queuedTerminalApp struct {
	fyne.App
	driver *queuedTerminalDriver
}

func (a *queuedTerminalApp) Driver() fyne.Driver { return a.driver }

func TestEmbeddedFeedUpdatesCursorBeforeReturning(t *testing.T) {
	previous := fyne.CurrentApp()
	driver := &queuedTerminalDriver{Driver: previous.Driver()}
	fyne.SetCurrentApp(&queuedTerminalApp{App: previous, driver: driver})
	defer fyne.SetCurrentApp(previous)
	term := New()
	term.Resize(fyne.NewSize(800, 240))
	for _, chunk := range []string{"prompt> ", "a", "b", "c"} {
		term.Feed([]byte(chunk))
		require.NotEmpty(t, term.content.Rows)
		require.Equal(t, len(term.content.Rows[0].Cells), term.cursorCol)
		require.Equal(t, term.content.PositionForCursorLocation(0, term.cursorCol), term.cursor.Position())
	}
	require.Equal(t, "prompt> abc", term.content.Text())
	require.Equal(t, 11, term.cursorCol)
}
