package terminal

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

func termWithRows(t *testing.T) *Terminal {
	t.Helper()
	term := New()
	term.Resize(fyne.NewSize(500, 300))
	if term.config.Rows < 8 {
		t.Fatalf("need at least 8 rows, got %d", term.config.Rows)
	}
	// Rows 1..6 hold "r1".."r6".
	term.handleOutput([]byte("\x1b[1;1Hr1\x1b[2;1Hr2\x1b[3;1Hr3\x1b[4;1Hr4\x1b[5;1Hr5\x1b[6;1Hr6"))
	return term
}

// DL and IL only act at and below the cursor inside the scroll region, and
// a count larger than the room left deletes or inserts what fits.
func TestDeleteAndInsertLinesRespectScrollRegion(t *testing.T) {
	term := termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[3;1H\x1b[M")) // DL 1 at row 3, region 2..5
	rows := visibleRows(term)
	assert.Equal(t, []string{"r1", "r2", "r4", "r5", "", "r6"}, rows[:6])

	term = termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[3;1H\x1b[99M")) // DL more than fits
	assert.Equal(t, []string{"r1", "r2", "", "", "", "r6"}, visibleRows(term)[:6])

	term = termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[7;1H\x1b[M")) // cursor outside region
	assert.Equal(t, []string{"r1", "r2", "r3", "r4", "r5", "r6"}, visibleRows(term)[:6])

	term = termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[3;1H\x1b[2L")) // IL 2 at row 3
	assert.Equal(t, []string{"r1", "r2", "", "", "r3", "r6"}, visibleRows(term)[:6])

	term = termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[3;1H\x1b[99L")) // IL more than fits
	assert.Equal(t, []string{"r1", "r2", "", "", "", "r6"}, visibleRows(term)[:6])

	term = termWithRows(t)
	term.handleOutput([]byte("\x1b[2;5r\x1b[1;1H\x1b[L")) // cursor outside region
	assert.Equal(t, []string{"r1", "r2", "r3", "r4", "r5", "r6"}, visibleRows(term)[:6])
}

// Reverse video swaps the drawn colours without disturbing the logical ones,
// so SGR 27 restores them and colours set while reversed land on the right
// layer.
func TestReverseVideoIsADrawAttribute(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("\x1b[31;44m"))
	red, blue := term.currentFG, term.currentBG

	term.handleOutput([]byte("\x1b[7m"))
	fg, bg := term.displayColors()
	assert.Equal(t, blue, fg)
	assert.Equal(t, red, bg)
	term.handleOutput([]byte("\x1b[27m"))
	fg, bg = term.displayColors()
	assert.Equal(t, red, fg)
	assert.Equal(t, blue, bg)

	// A colour set while reversed is the logical foreground: after 27 it is
	// drawn as foreground on the default background, not as a background.
	term.handleOutput([]byte("\x1b[0m\x1b[7m\x1b[31m\x1b[27m"))
	fg, bg = term.displayColors()
	assert.Equal(t, red, fg)
	assert.Nil(t, bg)

	// Defaults stay defaults; while reversed they draw as theme colours.
	term.handleOutput([]byte("\x1b[0m\x1b[7m"))
	fg, bg = term.displayColors()
	assert.NotNil(t, fg)
	assert.NotNil(t, bg)
	term.handleOutput([]byte("\x1b[27m"))
	fg, bg = term.displayColors()
	assert.Nil(t, fg)
	assert.Nil(t, bg)

	// SGR 0 ends reverse video.
	term.handleOutput([]byte("\x1b[7m\x1b[0m"))
	assert.False(t, term.reversed)
}

// IL and DL leave the cursor at the left margin.
func TestInsertDeleteLinesHomeTheColumn(t *testing.T) {
	term := termWithRows(t)
	term.handleOutput([]byte("\x1b[3;5H\x1b[L"))
	assert.Equal(t, 0, term.cursorCol)
	term.handleOutput([]byte("\x1b[3;5H\x1b[M"))
	assert.Equal(t, 0, term.cursorCol)
}

// A custom scroll region that no longer fits after the screen shrinks is
// reset, so line operations cannot write rows past the screen.
func TestShrinkResetsOversizedScrollRegion(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 400))
	rows := int(term.config.Rows)
	term.handleOutput([]byte(fmt.Sprintf("\x1b[2;%dr", rows)))
	assert.Equal(t, rows-1, term.scrollBottom)
	term.Resize(fyne.NewSize(500, 150))
	assert.LessOrEqual(t, term.scrollBottom, int(term.config.Rows)-1)

	before := len(term.content.Rows)
	term.handleOutput([]byte("\x1b[2;1H\x1b[M\x1b[L"))
	assert.LessOrEqual(t, len(term.content.Rows), max(before, int(term.config.Rows)))
}
