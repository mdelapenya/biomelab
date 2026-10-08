package widget

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	_ "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
)

func blinkingGrid() *TermGrid {
	grid := NewTermGrid()
	style := NewTermTextGridStyle(color.White, color.Black, 0xff, true, fyne.TextStyle{})
	grid.Rows = []widget.TextGridRow{{Cells: []widget.TextGridCell{{Rune: 'x', Style: style}}}}
	return grid
}

func TestStopBlinkEndsTickerAndPreventsRestart(t *testing.T) {
	grid := blinkingGrid()
	grid.Refresh()
	assert.NotNil(t, grid.tickerCancel, "a blinking cell starts the ticker")

	grid.StopBlink()
	assert.Nil(t, grid.tickerCancel)

	// A refresh already queued by the old ticker must not start a new one.
	grid.Refresh()
	assert.Nil(t, grid.tickerCancel)

	// Rendering the grid again re-enables blinking.
	grid.CreateRenderer()
	grid.Refresh()
	assert.NotNil(t, grid.tickerCancel)
	grid.StopBlink()
}
