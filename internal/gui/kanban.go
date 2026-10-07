package gui

import (
	"fmt"
	"image/color"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/notes"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// prLink is a tappable monospace label that opens the PR/MR URL in the default
// browser when clicked.
type prLink struct {
	widget.BaseWidget
	txt *canvas.Text
	u   *url.URL
}

func newPRLink(label, rawURL string, c color.Color) *prLink {
	u, _ := url.Parse(rawURL)
	txt := uiText(label, c, false)
	txt.TextStyle.Underline = true
	pl := &prLink{txt: txt, u: u}
	pl.ExtendBaseWidget(pl)
	return pl
}

func (pl *prLink) Tapped(_ *fyne.PointEvent) {
	if pl.u != nil {
		_ = fyne.CurrentApp().OpenURL(pl.u)
	}
}
func (pl *prLink) TappedSecondary(_ *fyne.PointEvent)  {}
func (pl *prLink) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(pl.txt) }

// statusText renders independent review and CI state without a hover surface.
type statusText struct {
	widget.BaseWidget
	txt *canvas.Text
}

func newStatusText(label string, c color.Color) *statusText {
	s := &statusText{txt: uiText(label, c, false)}
	s.ExtendBaseWidget(s)
	return s
}

func (s *statusText) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(s.txt) }

// buildKanbanCardContent leads with the provider title when one exists;
// branches remain technical and titleless cards retain their natural height.
func buildKanbanCardContent(wt git.Worktree, agents []agent.Info, terminals []terminal.Info, pr *provider.PRInfo, selected bool) fyne.CanvasObject {
	branch := newMeasuredText(wt.Branch, colorDimGray, false, true, false)
	branch.txt.TextSize = scaledSize(11)
	rows := []fyne.CanvasObject{}
	if pr != nil && pr.Title != "" {
		title := newMeasuredText(pr.Title, colorForeground, true, false, false)
		title.txt.TextSize = scaledSize(12)
		rows = append(rows, title)
	}
	rows = append(rows, branch)
	if pr != nil {
		state := pr.State
		if pr.Draft {
			state = "draft"
		}
		link := newPRLink(fmt.Sprintf("#%d · %s", pr.Number, state), pr.URL, colorBlue)
		link.txt.TextSize = scaledSize(11)
		rows = append(rows, link)
	}
	if len(agents) > 0 {
		rows = append(rows, secondaryText(string(agents[0].Kind)+" active"))
	}
	if len(terminals) > 0 {
		rows = append(rows, secondaryText(fmt.Sprintf("%d terminal(s)", len(terminals))))
	}
	if notes.Exists(wt.Path) {
		rows = append(rows, secondaryText("Notes available"))
	}
	if wt.IsDirty {
		dirty := secondaryText("Uncommitted changes")
		dirty.Color = colorYellow
		rows = append(rows, dirty)
	}
	if pr != nil {
		rows = append(rows, prStatusLabels(pr)...)
	}
	if selected {
		for _, o := range rows {
			switch t := o.(type) {
			case *measuredText:
				t.txt.Color = colorOnAction
			case *prLink:
				t.txt.Color = colorOnAction
			case *statusText:
				t.txt.Color = colorOnAction
			case *canvas.Text:
				t.Color = colorOnAction
			}
		}
	}
	return container.NewVBox(rows...)
}

// kanbanCardWrapper wraps a tappableCard and reports MinSize.Width = 1 so that
// container.NewVScroll always sizes the card to the column width rather than to
// the natural text-content width (which can exceed the narrow kanban column).
type kanbanCardWrapper struct {
	widget.BaseWidget
	inner *tappableCard
}

func newKanbanCardWrapper(inner *tappableCard) *kanbanCardWrapper {
	w := &kanbanCardWrapper{inner: inner}
	w.ExtendBaseWidget(w)
	return w
}

func (w *kanbanCardWrapper) Tapped(e *fyne.PointEvent)          { w.inner.Tapped(e) }
func (w *kanbanCardWrapper) TappedSecondary(e *fyne.PointEvent) { w.inner.TappedSecondary(e) }
func (w *kanbanCardWrapper) CreateRenderer() fyne.WidgetRenderer {
	return &kanbanCardWrapperRenderer{w: w}
}

type kanbanCardWrapperRenderer struct{ w *kanbanCardWrapper }

func (r *kanbanCardWrapperRenderer) Layout(size fyne.Size) {
	pad := scaledSize(1)
	r.w.inner.Move(fyne.NewPos(pad, pad))
	r.w.inner.Resize(fyne.NewSize(max(float32(0), size.Width-2*pad), max(float32(0), size.Height-2*pad)))
}
func (r *kanbanCardWrapperRenderer) MinSize() fyne.Size {
	// Report width=1 so the VScroll gives the VBox the scroll (column) width,
	// not the widest text element. Height stays natural so rows don't collapse.
	return fyne.NewSize(1, r.w.inner.MinSize().Height+scaledSize(2))
}
func (r *kanbanCardWrapperRenderer) Refresh() { r.w.inner.Refresh() }
func (r *kanbanCardWrapperRenderer) Destroy() {}
func (r *kanbanCardWrapperRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.w.inner}
}

// kanbanStageOf returns the kanban column index (0–4) for a worktree based on
// its PR/MR state and review status.
//
//	0 = Closed Unmerged — PR is closed but not merged
//	1 = Created   — no PR
//	2 = PR Sent   — open PR with no review activity yet
//	3 = PR In Review — open PR that has at least one review
//	4 = PR Merged — PR has been merged
func kanbanStageOf(pr *provider.PRInfo) int {
	if pr == nil {
		return 1
	}
	switch pr.State {
	case "closed":
		return 0
	case "merged":
		return 4
	case "open":
		if pr.ReviewStatus != "" {
			return 3
		}
		return 2
	default:
		return 1
	}
}

var kanbanColumnTitles = [5]string{
	"Closed Unmerged",
	"Created",
	"PR Sent",
	"PR In Review",
	"PR Merged",
}

// kanbanColumnColor returns the header accent color for a kanban stage.
func kanbanColumnColor(stage int) color.Color {
	switch stage {
	case 0:
		return colorRed
	case 1:
		return colorGray
	case 2:
		return colorBlue
	case 3:
		return colorYellow
	case 4:
		return colorPurple
	default:
		return colorGray
	}
}

// makeStagePill returns a small coloured pill showing the PR lifecycle stage label.
// Background is the stage accent at ~16% opacity; text is the full accent color.
func makeStagePill(stage int) fyne.CanvasObject {
	accent := kanbanColumnColor(stage)
	nrgba := accent.(color.NRGBA)
	bg := canvas.NewRectangle(color.NRGBA{R: nrgba.R, G: nrgba.G, B: nrgba.B, A: 0x28})
	bg.CornerRadius = scaledSize(radiusControl)
	label := uiText(kanbanColumnTitles[stage], accent, false)
	label.TextSize = scaledSize(11)
	return container.NewStack(bg, container.NewPadded(label))
}

// KanbanStages groups the 1-based linked-worktree indices (matching SelectedCard)
// into five slices, one per kanban column. Index 0 (main) is never included.
func (d *Dashboard) KanbanStages() [5][]int {
	var stages [5][]int
	linked := d.state.LinkedWorktrees()
	for i, wt := range linked {
		idx := i + 1 // 1-based: index 0 is the main worktree
		pr := d.prFor(wt.Branch)
		stage := kanbanStageOf(pr)
		stages[stage] = append(stages[stage], idx)
	}
	return stages
}

// KanbanColumnOf returns the stage (0–3) for the given 1-based worktree index.
func (d *Dashboard) KanbanColumnOf(cardIdx int) int {
	if cardIdx <= 0 || cardIdx >= len(d.state.Worktrees) {
		return 0
	}
	wt := d.state.Worktrees[cardIdx]
	pr := d.prFor(wt.Branch)
	return kanbanStageOf(pr)
}

// KanbanRowOf returns the 0-based row within the column for the given 1-based
// worktree index. Returns 0 if the card is not found in the column.
func (d *Dashboard) KanbanRowOf(cardIdx int, stages [5][]int) int {
	col := d.KanbanColumnOf(cardIdx)
	for row, idx := range stages[col] {
		if idx == cardIdx {
			return row
		}
	}
	return 0
}

// buildKanbanView preserves all five provider-derived columns. Column scrolls
// are retained, and the caller wraps the neutral board in a horizontal scroll.
func (d *Dashboard) buildKanbanView() fyne.CanvasObject {
	stages := d.KanbanStages()
	cols := make([]fyne.CanvasObject, len(stages))
	for stage, indices := range stages {
		dot := canvas.NewRectangle(kanbanColumnColor(stage))
		dot.CornerRadius = scaledSize(4)
		dot.SetMinSize(fyne.NewSize(scaledSize(7), scaledSize(7)))
		heading := uiText(kanbanColumnTitles[stage], colorForeground, true)
		heading.TextSize = scaledSize(11)
		header := container.NewBorder(nil, nil, container.NewCenter(dot), countChip(len(indices), colorSecondaryBg), heading)
		var items []fyne.CanvasObject
		d.stageCards[stage] = nil
		for _, idx := range indices {
			wt := d.state.Worktrees[idx]
			content := buildKanbanCardContent(wt, d.agentsFor(wt.Path), d.terminalsFor(wt.Path), d.prFor(wt.Branch), d.state.SelectedCard == idx)
			cardIdx := idx
			wtCopy := wt
			card := makeKanbanCard(content, d.state.SelectedCard == idx, func() { d.selectCard(cardIdx) })
			card.SetOnSecondaryTap(func() {
				if d.OnNoteRequested != nil {
					d.OnNoteRequested(wtCopy)
				}
			})
			wrapped := newKanbanCardWrapper(card)
			items = append(items, wrapped)
			d.stageCards[stage] = append(d.stageCards[stage], wrapped)
		}
		if len(items) == 0 {
			items = append(items, inset(secondaryText("No worktrees"), 0, spaceSM))
		}
		area := container.New(&stageBodyLayout{}, container.NewVBox(items...))
		if d.stageScrolls[stage] == nil {
			d.stageScrolls[stage] = container.NewVScroll(area)
		} else {
			d.stageScrolls[stage].Content = area
		}
		d.stageScrolls[stage].Offset = d.stageOffsets[stage]
		bg := canvas.NewRectangle(colorBackground)
		d.stageViewports[stage] = newStageViewport(d, stage, d.stageScrolls[stage], area)
		col := container.NewBorder(inset(header, 0, spaceXS), nil, nil, nil, d.stageViewports[stage])
		cols[stage] = container.NewStack(bg, inset(col, spaceSM, spaceSM))
	}
	d.boardColumns = cols
	return container.New(&boardLayout{}, cols...)
}
