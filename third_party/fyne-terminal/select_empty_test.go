package terminal

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

type recordingClipboard struct {
	content string
	sets    int
}

func (c *recordingClipboard) Content() string { return c.content }
func (c *recordingClipboard) SetContent(s string) {
	c.content = s
	c.sets++
}

// Copy with nothing selected must not overwrite the clipboard.
func TestCopyWithoutSelectionKeepsClipboard(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("hello"))

	assert.Equal(t, "", term.SelectedText())
	clip := &recordingClipboard{content: "kept"}
	term.copySelectedText(clip)
	assert.Equal(t, "kept", clip.content)
	assert.Equal(t, 0, clip.sets)
}

// Destroying an old renderer must stop its own grid's blink ticker, not the
// grid of a renderer created after it.
func TestRendererDestroyStopsOnlyItsOwnGrid(t *testing.T) {
	term := New()
	old := term.CreateRenderer()
	oldGrid := term.content
	_ = term.CreateRenderer()
	newGrid := term.content

	term.handleOutput([]byte("\x1b[5mblink\x1b[0m"))
	newGrid.Refresh()
	assert.True(t, newGrid.Blinking())

	old.Destroy()
	newGrid.Refresh()
	assert.True(t, newGrid.Blinking(), "a newer renderer's grid must keep blinking")
	assert.False(t, oldGrid.Blinking())
	newGrid.StopBlink()
}
