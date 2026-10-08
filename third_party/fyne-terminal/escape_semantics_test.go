package terminal

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

func newSizedTerm() *Terminal {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	return term
}

func visibleRows(term *Terminal) []string {
	var out []string
	for _, line := range strings.Split(term.content.Text(), "\n") {
		out = append(out, strings.TrimRight(strings.ReplaceAll(line, "\x00", " "), " "))
	}
	return out
}

// ED 1 erases from the start of the screen through the cursor, inclusive,
// including the row right above the cursor.
func TestEraseInDisplayToCursorIsInclusive(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("aaaa\r\nbbbb\r\ncccc\x1b[3;2H\x1b[1J"))
	rows := visibleRows(term)
	assert.Equal(t, "", rows[0])
	assert.Equal(t, "", rows[1], "the row directly above the cursor is erased")
	assert.Equal(t, "  cc", rows[2], "the cell under the cursor is erased")
}

// EL 1 erases from the start of the line through the cursor, inclusive.
func TestEraseInLineToCursorIsInclusive(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("abcd\x1b[2G\x1b[1K"))
	assert.Equal(t, "  cd", visibleRows(term)[0])

	term = newSizedTerm()
	term.handleOutput([]byte("ab\x1b[10G\x1b[1K")) // cursor past the content
	assert.Equal(t, "", visibleRows(term)[0])
}

// CUP with only a row parameter moves to that row, column 1.
func TestCursorPositionWithRowOnly(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("\x1b[3;7H\x1b[5H"))
	assert.Equal(t, 4, term.cursorRow)
	assert.Equal(t, 0, term.cursorCol)

	term.handleOutput([]byte("\x1b[H"))
	assert.Equal(t, 0, term.cursorRow)
	assert.Equal(t, 0, term.cursorCol)
}

// SU scrolls the content but leaves the cursor where it was.
func TestScrollUpKeepsCursor(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("\x1b[4;3H\x1b[2S"))
	assert.Equal(t, 3, term.cursorRow)
	assert.Equal(t, 2, term.cursorCol)
}

// Double-clicking the last character of a row selects its word.
func TestDoubleClickSelectsWordAtRowEnd(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("foo bar"))
	cell := term.guessCellSize()
	term.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(6.5*cell.Width, cell.Height/2)})
	assert.Equal(t, "bar", term.SelectedText())
}
