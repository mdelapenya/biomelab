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
