package widget

import (
	"context"
	"math"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

	// Viewport reports the visible vertical span (offset and height) of the
	// grid inside its scroller. When set, only those rows are rendered.
	Viewport func() (top, height float32)
	view     *widget.TextGrid
}

// CreateRenderer is a private method to Fyne which links this widget to its renderer
func (t *TermGrid) CreateRenderer() fyne.WidgetRenderer {
	t.ExtendBaseWidget(t)
	t.stopped = false

	// Initialise TextGrid's internal content so its methods stay safe to call.
	// It is never displayed: with ScrollNone, TextGrid builds a row widget (two
	// canvas objects per column) for every buffered row, and each row scans
	// every other row on layout. With full scrollback that is hundreds of
	// thousands of objects and an O(rows²) resize, so only the rows inside
	// Viewport are drawn, by a small TextGrid holding that slice.
	t.TextGrid.CreateRenderer()
	t.view = widget.NewTextGrid()
	return &termGridRenderer{grid: t}
}

// PositionForCursorLocation returns the grid-relative position of a cell,
// measured by the grid that is actually drawn.
func (t *TermGrid) PositionForCursorLocation(row, col int) fyne.Position {
	if t.view == nil {
		return t.TextGrid.PositionForCursorLocation(row, col)
	}
	return t.view.PositionForCursorLocation(row, col)
}

// RenderedRows reports how many buffered rows are currently drawn.
func (t *TermGrid) RenderedRows() int {
	if t.view == nil {
		return len(t.Rows)
	}
	return len(t.view.Rows)
}

type termGridRenderer struct {
	grid *TermGrid
}

func (r *termGridRenderer) cellSize() fyne.Size {
	th := r.grid.Theme()
	size := fyne.MeasureText("M", th.Size(theme.SizeNameText), fyne.TextStyle{Monospace: true})
	return fyne.NewSize(float32(math.Round(float64(size.Width))), float32(math.Round(float64(size.Height))))
}

// window returns the buffered rows [start, end) that intersect the viewport.
func (r *termGridRenderer) window(cell fyne.Size) (start, end int) {
	end = len(r.grid.Rows)
	if r.grid.Viewport == nil || cell.Height <= 0 {
		return 0, end
	}
	top, height := r.grid.Viewport()
	start = min(max(int(math.Floor(float64(top/cell.Height))), 0), end)
	end = min(max(int(math.Ceil(float64((top+height)/cell.Height)))+1, start), end)
	return start, end
}

func (r *termGridRenderer) Layout(size fyne.Size) {
	cell := r.cellSize()
	start, end := r.window(cell)
	view := r.grid.view
	view.Rows = r.grid.Rows[start:end]
	view.TabWidth = r.grid.TabWidth
	view.ShowWhitespace = r.grid.ShowWhitespace
	view.Move(fyne.NewPos(0, float32(start)*cell.Height))
	view.Resize(fyne.NewSize(size.Width, float32(end-start)*cell.Height))
}

func (r *termGridRenderer) MinSize() fyne.Size {
	longest := 0
	for _, row := range r.grid.Rows {
		longest = max(longest, len(row.Cells))
	}
	cell := r.cellSize()
	return fyne.NewSize(cell.Width*float32(longest), cell.Height*float32(len(r.grid.Rows)))
}

func (r *termGridRenderer) Refresh() {
	r.Layout(r.grid.Size())
	r.grid.view.Refresh()
}

func (r *termGridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.grid.view}
}

func (r *termGridRenderer) Destroy() {}

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

// SetCell stores a cell; outside an update batch it also redraws the grid.
func (t *TermGrid) SetCell(row, col int, cell widget.TextGridCell) {
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
	if !t.batch {
		t.Refresh()
	}
}
