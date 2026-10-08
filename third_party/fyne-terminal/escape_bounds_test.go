package terminal

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// Process output is untrusted: no escape sequence may panic the application.
func TestEscapeSequencesDoNotPanic(t *testing.T) {
	for name, input := range map[string]string{
		"multibyte CSI final":         "\x1b[世m",
		"multibyte CSI with params":   "\x1b[1;2世",
		"delete chars past row end":   "ab\x1b[20G\x1b[P",
		"insert chars past row end":   "ab\x1b[20G\x1b[@",
		"delete chars on empty row":   "\x1b[5;20H\x1b[3P",
		"insert chars on empty row":   "\x1b[5;20H\x1b[3@",
		"multibyte then plain output": "\x1b[é\x1b[2Cx",
	} {
		t.Run(name, func(t *testing.T) {
			term := New()
			term.Resize(fyne.NewSize(500, 150))
			assert.NotPanics(t, func() { term.handleOutput([]byte(input)) })
		})
	}
}

func TestDeleteAndInsertCharsWithinRow(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("abcdef\x1b[3G\x1b[2P"))
	assert.Equal(t, "abef", term.content.Text())

	term = New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("abcd\x1b[3G\x1b[2@"))
	assert.Equal(t, "ab  cd", term.content.Text())
}
