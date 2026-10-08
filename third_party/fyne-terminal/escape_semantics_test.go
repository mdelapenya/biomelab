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

// visibleRows returns the on-screen rows (scrollback excluded), with
// trailing spaces trimmed.
func visibleRows(term *Terminal) []string {
	var out []string
	for i := term.rowOffset(); i < term.rowOffset()+int(term.config.Rows); i++ {
		out = append(out, strings.TrimRight(term.content.RowText(i), " "))
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

// While the buffer is still shorter than the screen, SU must still move the
// visible content up so later output lands on the freed line.
func TestScrollUpWithShortBuffer(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("a\r\nb\r\nc\x1b[SX"))
	rows := visibleRows(term)
	assert.Equal(t, "b", rows[0])
	assert.Equal(t, "c", rows[1])
	assert.Equal(t, " X", rows[2])
}

// SU scrolls the scroll region even when the cursor is outside it, and never
// clears rows outside the region.
func TestScrollUpStaysInsideRegion(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 300))
	assert.GreaterOrEqual(t, int(term.config.Rows), 6)
	term.handleOutput([]byte("\x1b[1;1Hhead\x1b[3;1Hr3\x1b[4;1Hr4\x1b[3;4r\x1b[6;1H\x1b[S"))
	rows := visibleRows(term)
	assert.Equal(t, "head", rows[0])
	assert.Equal(t, "r4", rows[2], "region scrolled although the cursor is below it")
	assert.Equal(t, "", rows[3])

	term.handleOutput([]byte("\x1b[3;1Hr3\x1b[20S"))
	rows = visibleRows(term)
	assert.Equal(t, "head", rows[0], "a count larger than the region must not clear rows above it")
	assert.Equal(t, "", rows[2])
}

// Erased cells are spaces carrying the current background, not NUL cells.
func TestEraseToCursorUsesStyledSpaces(t *testing.T) {
	term := newSizedTerm()
	term.handleOutput([]byte("abcd\x1b[44m\x1b[2G\x1b[1K"))
	cells := term.content.Rows[0].Cells
	assert.Equal(t, ' ', cells[0].Rune)
	assert.Equal(t, ' ', cells[1].Rune)
	assert.NotNil(t, cells[0].Style)
}
