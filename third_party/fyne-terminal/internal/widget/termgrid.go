package widget

import (
	"context"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"fyne.io/fyne/v2"
)

const blinkingInterval = 500 * time.Millisecond

// TermGrid is a monospaced grid of characters.
// This is designed to be used by our terminal emulator.
type TermGrid struct {
	widget.TextGrid

	tickerCancel context.CancelFunc
	// stopped is set once the owning renderer is destroyed, so a refresh
	// already queued by the ticker cannot start a new blink goroutine.
	stopped bool
	batch   bool
}

// CreateRenderer is a private method to Fyne which links this widget to its renderer
func (t *TermGrid) CreateRenderer() fyne.WidgetRenderer {
	t.ExtendBaseWidget(t)
	t.stopped = false

	return t.TextGrid.CreateRenderer()
}

// Blinking reports whether the blink goroutine is running.
func (t *TermGrid) Blinking() bool { return t.tickerCancel != nil }

// StopBlink ends the blink goroutine and keeps it from restarting until the
// grid is rendered again. Call it when the owning renderer is destroyed.
func (t *TermGrid) StopBlink() {
	t.stopped = true
	if t.tickerCancel != nil {
		t.tickerCancel()
		t.tickerCancel = nil
	}
}

// NewTermGrid creates a new empty TextGrid widget.
func NewTermGrid() *TermGrid {
	grid := &TermGrid{}
	grid.ExtendBaseWidget(grid)

	grid.Scroll = container.ScrollNone
	return grid
}

// Refresh will be called when this grid should update.
// We update our blinking status and then call the TextGrid we extended to refresh too.
func (t *TermGrid) Refresh() {
	if t.batch {
		return
	}
	t.refreshBlink(false)
}

func (t *TermGrid) refreshBlink(blink bool) {
	// reset shouldBlink which can be set by setCellRune if a cell with BlinkEnabled is found
	shouldBlink := false

	for _, row := range t.Rows {
		for _, r := range row.Cells {
			if s, ok := r.Style.(*TermTextGridStyle); ok && s != nil && s.BlinkEnabled {
				shouldBlink = true

				s.blink(blink)
			}
		}
	}
	t.TextGrid.Refresh()

	switch {
	case shouldBlink && t.tickerCancel == nil && !t.stopped:
		t.runBlink()
	case !shouldBlink && t.tickerCancel != nil:
		t.tickerCancel()
		t.tickerCancel = nil
	}
}

func (t *TermGrid) runBlink() {
	if t.tickerCancel != nil {
		t.tickerCancel()
		t.tickerCancel = nil
	}
	var tickerContext context.Context
	tickerContext, t.tickerCancel = context.WithCancel(context.Background())
	ticker := time.NewTicker(blinkingInterval)
	blinking := false
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-tickerContext.Done():
				return
			case <-ticker.C:
				blinking = !blinking
				fyne.Do(func() {
					t.refreshBlink(blinking)
				})
			}
		}
	}()
}

// BeginUpdate defers rendering while the UI thread applies a chunk of output.
func (t *TermGrid) BeginUpdate() { t.batch = true }

// EndUpdate leaves rendering to the terminal's final refresh.
func (t *TermGrid) EndUpdate() { t.batch = false }

// SetCell avoids TextGrid's per-character renderer work during output parsing.
func (t *TermGrid) SetCell(row, col int, cell widget.TextGridCell) {
	if !t.batch {
		t.TextGrid.SetCell(row, col, cell)
		return
	}
	if row < 0 || col < 0 {
		return
	}
	for len(t.Rows) <= row {
		t.Rows = append(t.Rows, widget.TextGridRow{})
	}
	for len(t.Rows[row].Cells) <= col {
		t.Rows[row].Cells = append(t.Rows[row].Cells, widget.TextGridCell{})
	}
	t.Rows[row].Cells[col] = cell
}
