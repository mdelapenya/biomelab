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
