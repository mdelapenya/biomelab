package gui

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"strings"
)

func (d *Dashboard) buildListView() fyne.CanvasObject {
	d.listRows = nil
	for i, wt := range d.state.LinkedWorktrees() {
		idx := i + 1
		foreground, muted := colorForeground, colorGray
		bg := canvas.NewRectangle(colorBackground)
		bg.CornerRadius = scaledSize(radiusControl)
		if idx == d.state.SelectedCard {
			bg.FillColor = colorActionBg
			foreground = colorOnAction
			muted = colorOnAction
		}
		title := wt.Branch
		mono := true
		if wt.IsMain {
			title = "Main checkout"
			mono = false
		}
		if pr := d.prFor(wt.Branch); pr != nil && pr.Title != "" {
			title = pr.Title
			mono = false
		}
		heading := newMeasuredText(title, foreground, true, mono, false)
		heading.txt.TextSize = scaledSize(12)
		subtitle := newMeasuredText(wt.Branch, muted, false, true, false)
		subtitle.txt.TextSize = scaledSize(11)
		status := syncLabel(wt.Sync)
		if !wt.IsMain {
			status = kanbanColumnTitles[kanbanStageOf(d.prFor(wt.Branch))]
		}
		parts := []string{status}
		if wt.IsDirty {
			parts = append(parts, "Modified")
		}
		if n := len(d.agentsFor(wt.Path)); n > 0 {
			parts = append(parts, fmt.Sprintf("%d agent(s)", n))
		}
		activity := newMeasuredText(strings.Join(parts, " · "), muted, false, false, false)
		activity.txt.TextSize = scaledSize(11)
		index, worktree := idx, wt
		items := []fyne.CanvasObject{heading}
		if title != wt.Branch {
			items = append(items, subtitle)
		}
		items = append(items, activity)
		row := newTappableCard(container.NewStack(bg, inset(container.NewVBox(items...), spaceSM, spaceSM)), func() { d.selectCard(index) })
		row.SetOnSecondaryTap(func() {
			if d.OnNoteRequested != nil {
				d.OnNoteRequested(worktree)
			}
		})
		d.listRows = append(d.listRows, row)
	}
	content := container.NewVBox(d.listRows...)
	if d.listScroll == nil {
		d.listScroll = container.NewVScroll(content)
	} else {
		d.listScroll.Content = content
	}
	d.listScroll.Offset = d.listOffset
	return d.listScroll
}
