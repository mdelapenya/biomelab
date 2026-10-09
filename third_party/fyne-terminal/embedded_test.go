package terminal

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedModifiedKeysThroughShortcut(t *testing.T) {
	for _, tc := range []struct {
		mod  fyne.KeyModifier
		want string
	}{
		{fyne.KeyModifierControl, "\x1b[1;5D"}, {fyne.KeyModifierAlt, "\x1b[1;3D"},
		{fyne.KeyModifierShift | fyne.KeyModifierAlt, "\x1b[1;4D"},
	} {
		var b bytes.Buffer
		term := New()
		term.AttachWriter(NopCloser(&b))
		term.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft, Modifier: tc.mod})
		require.Equal(t, tc.want, b.String())
	}
}
func TestEmbeddedPreservesTabEscapeAndWorkspaceChord(t *testing.T) {
	var b bytes.Buffer
	term := New()
	term.AttachWriter(NopCloser(&b))
	term.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	term.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	left := false
	term.OnReturnToWorkspace = func() { left = true }
	term.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeySpace, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift})
	require.True(t, left)
	require.Equal(t, "\t\x1b", b.String())
}
func TestEmbeddedHistorySurvivesAlternateScreen(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(500, 150))
	for i := 0; i < 200; i++ {
		term.Feed([]byte(fmt.Sprintf("line-%d\r\n", i)))
	}
	before := term.content.Text()
	require.Contains(t, before, "line-1\n")
	term.Feed([]byte("\x1b[?1049h"))
	for i := 0; i < 200; i++ {
		term.Feed([]byte("alternate\r\n"))
	}
	require.LessOrEqual(t, len(term.content.Rows), int(term.config.Rows))
	term.Feed([]byte("\x1b[?1049l"))
	require.Equal(t, before, term.content.Text())
	require.NotContains(t, term.content.Text(), "alternate")
}
func TestEmbeddedBoundedHistoryAndUnicodeChunks(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(500, 150))
	term.scrollbackMax = 20
	for i := 0; i < 100; i++ {
		term.Feed([]byte("line\r\n"))
	}
	require.LessOrEqual(t, len(term.content.Rows), 20+int(term.config.Rows))
	data := []byte("héllo 世界")
	for _, b := range data {
		term.Feed([]byte{b})
	}
	require.True(t, strings.Contains(term.Text(), "héllo 世界"), term.Text())
}

func TestEmbeddedResizeAndTransientZeroSize(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(500, 300))
	term.Feed([]byte("before resize\r\n"))
	rows, cols := term.Dimensions()
	term.Resize(fyne.NewSize(0, 0))
	r, c := term.Dimensions()
	require.Equal(t, rows, r)
	require.Equal(t, cols, c)
	term.Resize(fyne.NewSize(400, 150))
	term.Feed([]byte("after resize"))
	require.Contains(t, term.Text(), "after resize")
	require.Less(t, term.cursorRow, int(term.config.Rows))
	term.Resize(fyne.NewSize(500, 300))
	require.Contains(t, term.Text(), "after resize")
}

func BenchmarkEmbeddedResize(b *testing.B) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(800, 400))
	term.Feed([]byte(strings.Repeat("build output\r\n", 1000)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		term.Resize(fyne.NewSize(float32(780+i%40), 400))
	}
}

func TestEmbeddedFollowsOutputBurst(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(500, 150))
	var burst bytes.Buffer
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&burst, "line-%d\r\n", i)
	}
	// ConPTY and PTYs deliver large bursts in one read; the view must still
	// land on the newest row instead of staying at the top of the history.
	term.Feed(burst.Bytes())
	sc := term.scrollContainer
	require.NotNil(t, sc)
	bottom := sc.Content.MinSize().Height - sc.Size().Height
	require.Greater(t, bottom, float32(0))
	require.InDelta(t, bottom, sc.Offset.Y, 1, "viewport should follow the burst to the last row")
	term.Feed([]byte("tail\r\n"))
	bottom = sc.Content.MinSize().Height - sc.Size().Height
	require.InDelta(t, bottom, sc.Offset.Y, 1, "viewport should keep following later output")
	// A reader who scrolled up must not be yanked back down.
	sc.ScrollToTop()
	term.Feed([]byte("more\r\n"))
	require.Equal(t, float32(0), sc.Offset.Y)
}

func TestEmbeddedClearHistoryClampsViewport(t *testing.T) {
	for _, scrolledUp := range []bool{false, true} {
		term := New()
		term.Refresh()
		term.Resize(fyne.NewSize(800, 500))
		term.Feed([]byte(strings.Repeat("history\r\n", 200)))
		sc := term.scrollContainer
		if scrolledUp {
			sc.Offset.Y /= 2
			sc.Refresh()
		}
		term.Feed([]byte("\x1b[2J\x1b[3J\x1b[Hprompt> "))
		require.Contains(t, term.Text(), "prompt>")
		require.LessOrEqual(t, sc.Offset.Y, max(float32(0), sc.Content.MinSize().Height-sc.Size().Height))
		require.InDelta(t, max(sc.Content.MinSize().Height, sc.Size().Height), sc.Content.Size().Height, 1)
	}
}

func BenchmarkEmbeddedOutput2000Lines(b *testing.B) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(800, 500))
	data := []byte(strings.Repeat(strings.Repeat("x", 68)+"\r\n", 2000))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for offset := 0; offset < len(data); offset += 32 * 1024 {
			term.Feed(data[offset:min(offset+32*1024, len(data))])
		}
	}
}

func TestEmbeddedRendersOnlyVisibleRows(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(800, 300))
	term.Feed([]byte(strings.Repeat("history line\r\n", 2000)))
	require.Greater(t, len(term.content.Rows), 900)
	// Drawing every history row cost two canvas objects per cell and an
	// O(rows²) layout, freezing the app for tens of seconds on resize.
	require.LessOrEqual(t, term.content.RenderedRows(), int(term.config.Rows)+2)
	sc := term.scrollContainer
	sc.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, 400)})
	require.LessOrEqual(t, term.content.RenderedRows(), int(term.config.Rows)+2)
	require.Contains(t, strings.Join(visibleRowTexts(term), "\n"), "history line")
}

func visibleRowTexts(term *Terminal) []string {
	cell := term.guessCellSize()
	sc := term.scrollContainer
	start := int(sc.Offset.Y / cell.Height)
	var rows []string
	for i := start; i < min(start+int(term.config.Rows), len(term.content.Rows)); i++ {
		rows = append(rows, term.content.RowText(i))
	}
	return rows
}

func TestEmbeddedResizeKeepsFollowingOutput(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(600, 400))
	term.Feed([]byte(strings.Repeat("line\r\n", 300)))
	sc := term.scrollContainer
	atBottom := func() {
		t.Helper()
		require.InDelta(t, sc.Content.MinSize().Height-sc.Size().Height, sc.Offset.Y, 1)
	}
	atBottom()
	// Expand, restore, and hide the drawer (zero height) before showing it again.
	for _, size := range []fyne.Size{{Width: 600, Height: 700}, {Width: 600, Height: 200}, {}, {Width: 600, Height: 400}} {
		term.Resize(size)
		if size.Height > 0 {
			atBottom()
		}
	}
	// A reader in history keeps their place across a resize.
	sc.ScrollToTop()
	sc.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -50)})
	offset := sc.Offset.Y
	require.Greater(t, offset, float32(0))
	term.Resize(fyne.NewSize(600, 300))
	require.InDelta(t, offset, sc.Offset.Y, 1)
}

func TestEmbeddedShrinkMatchesConPTYRepaint(t *testing.T) {
	term := New()
	term.Refresh()
	cell := term.guessCellSize()
	resize := func(rows int) {
		term.Resize(fyne.NewSize(cell.Width*100, cell.Height*float32(rows)))
		require.Equal(t, uint(rows), term.config.Rows)
	}
	resize(10)
	for i := 1; i <= 30; i++ {
		term.Feed([]byte(fmt.Sprintf("L%d\r\n", i)))
	}
	term.Feed([]byte("PS> "))
	// Expanding pads blank rows below the prompt; ConPTY keeps its top row.
	resize(20)
	// Restoring must drop those rows again rather than push L22..L30 and the
	// prompt into history, because ConPTY repaints the same viewport from its
	// top row, which would then duplicate them.
	resize(10)
	require.Equal(t, 9, term.cursorRow)
	var repaint strings.Builder
	repaint.WriteString("\x1b[?25l\x1b[H")
	for i := 22; i <= 30; i++ {
		fmt.Fprintf(&repaint, "L%d\x1b[K\r\n", i)
	}
	repaint.WriteString("PS> \x1b[K\x1b[?25h")
	term.Feed([]byte(repaint.String()))
	require.Equal(t, 1, strings.Count(term.Text(), "L25\n"), term.Text())
	require.Equal(t, 1, strings.Count(term.Text(), "PS> "), term.Text())
}

func TestEmbeddedNegativeSizeKeepsDimensions(t *testing.T) {
	term := New()
	term.Refresh()
	term.Resize(fyne.NewSize(500, 300))
	rows, cols := term.Dimensions()
	term.Resize(fyne.NewSize(500, -40))
	r, c := term.Dimensions()
	require.Equal(t, rows, r)
	require.Equal(t, cols, c)
	term.Resize(fyne.NewSize(500, 150))
	require.Less(t, term.config.Rows, rows)
}
