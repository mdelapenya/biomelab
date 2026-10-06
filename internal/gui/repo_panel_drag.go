package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// dragHandle is the small burger-icon widget that sits to the left of each
// repo name. Only the handle is draggable — the rest of the header stays
// passive — so users don't accidentally drag the row by clicking anywhere on
// it. Drag events are delegated back to the owning panel so reorder logic
// lives in one place.
type dragHandle struct {
	widget.BaseWidget
	groupIdx int
	panel    *RepoPanel
	icon     *widget.Icon
}

func newDragHandle(groupIdx int, panel *RepoPanel) *dragHandle {
	h := &dragHandle{
		groupIdx: groupIdx,
		panel:    panel,
		icon:     widget.NewIcon(theme.MenuIcon()),
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *dragHandle) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.icon)
}

func (h *dragHandle) Dragged(e *fyne.DragEvent) {
	h.panel.onHeaderDragged(h, e)
}

func (h *dragHandle) DragEnd() {
	h.panel.onHeaderDragEnd(h)
}

// Cursor signals vertical-drag intent on hover.
func (h *dragHandle) Cursor() desktop.Cursor {
	return desktop.VResizeCursor
}

// onHeaderDragged is the per-frame drag callback. The first call begins the
// drag (snapshotting header positions); subsequent calls accumulate the
// vertical delta and recompute the drop target. We only trigger a list
// rebuild when the target slot actually changes — this keeps the indicator
// visible without thrashing the canvas on every pixel of movement.
func (rp *RepoPanel) onHeaderDragged(h *dragHandle, e *fyne.DragEvent) {
	if rp.dragSrc < 0 {
		rp.dragSrc = h.groupIdx
		rp.dragDst = h.groupIdx
		rp.dragCumulativeDY = 0
		rp.snapshotHeaderPositions()
	}
	rp.dragCumulativeDY += e.Dragged.DY
	newDst := rp.computeDropTarget()
	if newDst != rp.dragDst {
		rp.dragDst = newDst
		rp.rebuildList()
	}
}

func (rp *RepoPanel) onHeaderDragEnd(_ *dragHandle) {
	if rp.dragSrc < 0 {
		return
	}
	src := rp.dragSrc
	dst := rp.dragDst

	rp.dragSrc = -1
	rp.dragDst = -1
	rp.dragCumulativeDY = 0
	rp.headerYSnap = nil
	rp.headerHSnap = nil

	if src != dst && dst >= 0 && rp.OnReorder != nil {
		// computeDropTarget returns len(groups) when the user has dragged
		// past the bottom-most group. Normalize to last-index for the
		// reorder callback.
		if dst >= len(rp.groups) {
			dst = len(rp.groups) - 1
		}
		rp.OnReorder(src, dst)
		return
	}
	// No reorder — repaint to clear the drop indicator.
	rp.rebuildList()
}

func (rp *RepoPanel) snapshotHeaderPositions() {
	n := len(rp.headerRows)
	rp.headerYSnap = make([]float32, n)
	rp.headerHSnap = make([]float32, n)
	for i, row := range rp.headerRows {
		rp.headerYSnap[i] = row.Position().Y
		rp.headerHSnap[i] = row.Size().Height
	}
}

// computeDropTarget returns the group index where the dragged row would land
// if the user released right now. Range is [0, len(groups)]; len(groups) means
// "after the last group".
func (rp *RepoPanel) computeDropTarget() int {
	src := rp.dragSrc
	if src < 0 || src >= len(rp.headerYSnap) {
		return src
	}
	// The midpoint of the source row at its current dragged position.
	srcMidY := rp.headerYSnap[src] + rp.headerHSnap[src]/2 + rp.dragCumulativeDY

	// Dragging up: target = first slot whose midpoint sits below the
	// dragged midpoint.
	for i := range src {
		midY := rp.headerYSnap[i] + rp.headerHSnap[i]/2
		if srcMidY < midY {
			return i
		}
	}
	// Dragging down: target = last slot whose midpoint sits above the
	// dragged midpoint.
	for i := len(rp.headerYSnap) - 1; i > src; i-- {
		midY := rp.headerYSnap[i] + rp.headerHSnap[i]/2
		if srcMidY > midY {
			return i
		}
	}
	return src
}
