package terminal

import (
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

// SGR 27 undoes reverse video, restoring the colours in effect before SGR 7.
func TestReverseVideoOffRestoresColours(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("\x1b[31;44m"))
	fg, bg := term.currentFG, term.currentBG
	term.handleOutput([]byte("\x1b[7m"))
	assert.Equal(t, bg, term.currentFG)
	assert.Equal(t, fg, term.currentBG)
	term.handleOutput([]byte("\x1b[27m"))
	assert.Equal(t, fg, term.currentFG)
	assert.Equal(t, bg, term.currentBG)

	// Default colours come back as defaults, and 27 without 7 is a no-op.
	term.handleOutput([]byte("\x1b[0m\x1b[7m\x1b[27m"))
	assert.Nil(t, term.currentFG)
	assert.Nil(t, term.currentBG)
	term.handleOutput([]byte("\x1b[31m\x1b[27m"))
	assert.Equal(t, fg, term.currentFG)

	// SGR 0 ends reverse video, so a later 27 does not swap again.
	term.handleOutput([]byte("\x1b[7m\x1b[0m\x1b[32m\x1b[27m"))
	assert.Nil(t, term.currentBG)
}
