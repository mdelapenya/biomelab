package gui

import (
	"fmt"
	"image/color"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/provider"
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
	txt := monoText(label, c, false)
	txt.TextStyle.Underline = true
	pl := &prLink{txt: txt, u: u}
	pl.ExtendBaseWidget(pl)
	return pl
}

func (pl *prLink) Tapped(_ *fyne.PointEvent) {
	if pl.u != nil {
		fyne.CurrentApp().OpenURL(pl.u)
	}
}
func (pl *prLink) TappedSecondary(_ *fyne.PointEvent) {}
func (pl *prLink) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(pl.txt) }

// hintIcon wraps a coloured label and shows a tooltip popup on mouse hover.
type hintIcon struct {
	widget.BaseWidget
	txt  *canvas.Text
	hint string
	pop  *widget.PopUp
}

func newHintIcon(label string, c color.Color, hint string) *hintIcon {
	h := &hintIcon{
		txt:  monoText(label, c, false),
		hint: hint,
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hintIcon) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(h.txt) }

func (h *hintIcon) MouseIn(_ *desktop.MouseEvent) {
	cnv := fyne.CurrentApp().Driver().CanvasForObject(h)
	if cnv == nil {
		return
	}
	label := widget.NewLabel(h.hint)
	h.pop = widget.NewPopUp(label, cnv)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(h)
	h.pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+h.Size().Height))
}

func (h *hintIcon) MouseOut() {
	if h.pop != nil {
		h.pop.Hide()
		h.pop = nil
	}
}

func (h *hintIcon) MouseMoved(_ *desktop.MouseEvent) {}

// buildKanbanCardContent builds a compact card matching the website's kb-card
// layout:
//
//	Row 1:  ● (stage-coloured dot)  branch-name
//	Row 2:  ● agent-name            (omitted when no agent is running)
//	Row 3:  #42 ↗  review-icon  CI-icon   (omitted when no PR)
//	Row 4:  ~ dirty                 (omitted when clean)
//
// The PR number is a tappable link (↗ suffix signals this). Review and CI icons
// are prefixed with emojis (🔍 / 🤖) and show a tooltip on hover:
//
//	review  🔍✓ green   🔍! red   🔍~ yellow
//	CI      🤖✓ green   🤖✗ red   🤖○ yellow
func buildKanbanCardContent(
	wt git.Worktree,
	agents []agent.Info,
	pr *provider.PRInfo,
	selected bool,
) fyne.CanvasObject {
	var rows []fyne.CanvasObject

	stage := kanbanStageOf(pr)
	dotColor := kanbanColumnColor(stage)

	// ── Row 1: ● branch-name ────────────────────────────────────────────
	dot := monoText("●", dotColor, false)
	dot.TextSize = scaledSize(8)
	branchColor := colorBranch
	prefix := ""
	if selected {
		branchColor = colorSelected
		prefix = "▸ "
	}
	branchLabel := monoText(prefix+wt.Branch, branchColor, true)
	branchLabel.TextSize = scaledSize(12)
	rows = append(rows, container.NewHBox(dot, branchLabel))

	// ── Row 2: ● agent-name (omitted when no agent) ─────────────────────
	if len(agents) > 0 {
		agentLine := monoText("● "+string(agents[0].Kind), colorGreen, false)
		agentLine.TextSize = scaledSize(10)
		rows = append(rows, agentLine)
	}

	// ── Row 3: #42 ↗  review  CI  (omitted when no PR) ─────────────────
	if pr != nil {
		// PR number as a tappable link; ↗ makes clickability explicit.
		prNum := newPRLink(fmt.Sprintf("#%d", pr.Number), pr.URL, colorBlue)

		var statusParts []fyne.CanvasObject
		statusParts = append(statusParts, prNum)

		// Review icon — 🔍 prefix, tooltip on hover.
		switch pr.ReviewStatus {
		case "approved":
			statusParts = append(statusParts, newHintIcon("🔍✓", colorGreen, "Review: approved"))
		case "changes_requested":
			statusParts = append(statusParts, newHintIcon("🔍!", colorRed, "Review: changes requested"))
		case "commented":
			statusParts = append(statusParts, newHintIcon("🔍~", colorYellow, "Review: commented"))
		}

		// CI icon — 🤖 prefix, tooltip on hover.
		switch pr.CheckStatus {
		case "success":
			statusParts = append(statusParts, newHintIcon("🤖✓", colorGreen, "CI: success"))
		case "failure":
			statusParts = append(statusParts, newHintIcon("🤖✗", colorRed, "CI: failure"))
		case "pending":
			statusParts = append(statusParts, newHintIcon("🤖○", colorYellow, "CI: pending"))
		}

		rows = append(rows, container.NewHBox(statusParts...))
	}

	// ── Row 4: ~ dirty (omitted when clean) ─────────────────────────────
	if wt.IsDirty {
		dirty := monoText("~ dirty", colorYellow, false)
		dirty.TextSize = scaledSize(10)
		rows = append(rows, dirty)
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
func (w *kanbanCardWrapper) TappedSecondary(_ *fyne.PointEvent) {}
func (w *kanbanCardWrapper) CreateRenderer() fyne.WidgetRenderer {
	return &kanbanCardWrapperRenderer{w: w}
}

type kanbanCardWrapperRenderer struct{ w *kanbanCardWrapper }

func (r *kanbanCardWrapperRenderer) Layout(size fyne.Size) {
	r.w.inner.Move(fyne.NewPos(0, 0))
	r.w.inner.Resize(size)
}
func (r *kanbanCardWrapperRenderer) MinSize() fyne.Size {
	// Report width=1 so the VScroll gives the VBox the scroll (column) width,
	// not the widest text element. Height stays natural so rows don't collapse.
	return fyne.NewSize(1, r.w.inner.MinSize().Height)
}
func (r *kanbanCardWrapperRenderer) Refresh()                     { r.w.inner.Refresh() }
func (r *kanbanCardWrapperRenderer) Destroy()                     {}
func (r *kanbanCardWrapperRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.w.inner} }

// kanbanStageOf returns the kanban column index (0–3) for a worktree based on
// its PR/MR state and review status.
//
//	0 = Created   — no PR, or PR is closed
//	1 = PR Sent   — open PR with no review activity yet
//	2 = PR In Review — open PR that has at least one review
//	3 = PR Merged — PR has been merged
func kanbanStageOf(pr *provider.PRInfo) int {
	if pr == nil || pr.State == "closed" {
		return 0
	}
	switch pr.State {
	case "merged":
		return 3
	case "open":
		if pr.ReviewStatus != "" {
			return 2
		}
		return 1
	default:
		return 0
	}
}

var kanbanColumnTitles = [4]string{
	"Created",
	"PR Sent",
	"PR In Review",
	"PR Merged",
}

// kanbanColumnColor returns the header accent color for a kanban stage.
func kanbanColumnColor(stage int) color.Color {
	switch stage {
	case 0:
		return colorGray
	case 1:
		return colorBlue
	case 2:
		return colorYellow
	case 3:
		return colorPurple
	default:
		return colorGray
	}
}

// kanbanColumnBgColor returns a subtle body-tint for the column background.
func kanbanColumnBgColor(stage int) color.Color {
	switch stage {
	case 1:
		return color.NRGBA{R: 0x69, G: 0xa7, B: 0xff, A: 0x1a} // blue  ~10 %
	case 2:
		return color.NRGBA{R: 0xf3, G: 0xd6, B: 0x6b, A: 0x1a} // yellow ~10 %
	case 3:
		return color.NRGBA{R: 0xca, G: 0x79, B: 0xff, A: 0x1a} // purple ~10 %
	default:
		return color.NRGBA{R: 0x70, G: 0x83, B: 0x94, A: 0x12} // gray   ~ 7 %
	}
}

// kanbanColumnHeaderBgColor returns a stronger header tint for a kanban column.
func kanbanColumnHeaderBgColor(stage int) color.Color {
	switch stage {
	case 1:
		return color.NRGBA{R: 0x69, G: 0xa7, B: 0xff, A: 0x38} // blue  ~22 %
	case 2:
		return color.NRGBA{R: 0xf3, G: 0xd6, B: 0x6b, A: 0x38} // yellow ~22 %
	case 3:
		return color.NRGBA{R: 0xca, G: 0x79, B: 0xff, A: 0x38} // purple ~22 %
	default:
		return color.NRGBA{R: 0x70, G: 0x83, B: 0x94, A: 0x28} // gray  ~16 %
	}
}

// KanbanStages groups the 1-based linked-worktree indices (matching SelectedCard)
// into four slices, one per kanban column. Index 0 (main) is never included.
func (d *Dashboard) KanbanStages() [4][]int {
	var stages [4][]int
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
func (d *Dashboard) KanbanRowOf(cardIdx int, stages [4][]int) int {
	col := d.KanbanColumnOf(cardIdx)
	for row, idx := range stages[col] {
		if idx == cardIdx {
			return row
		}
	}
	return 0
}

// buildKanbanView constructs the four-column kanban board for linked worktrees.
// Each column is wrapped in a coloured border rectangle (matching the website
// kb-col design): subtle body tint + stronger header tint + coloured stroke.
// Cards scroll vertically inside each column via NewVScroll, which constrains
// card width to the column width so nothing overflows sideways.
func (d *Dashboard) buildKanbanView() fyne.CanvasObject {
	stages := d.KanbanStages()

	cols := make([]fyne.CanvasObject, 4)
	for si, stageIndices := range stages {
		accentColor := kanbanColumnColor(si)

		// ── Header (pinned, stronger tint) ──────────────────────────────
		headerText := fmt.Sprintf("%s  %d", kanbanColumnTitles[si], len(stageIndices))
		headerLabel := monoText(headerText, accentColor, true)
		headerLabel.TextSize = scaledSize(11)

		headerBg := canvas.NewRectangle(kanbanColumnHeaderBgColor(si))
		headerBg.CornerRadius = 4
		header := container.NewStack(headerBg, container.NewPadded(headerLabel))
		headerSection := container.NewVBox(header, widget.NewSeparator())

		// ── Cards (vertically scrollable, width-constrained) ─────────────
		var cardArea fyne.CanvasObject
		if len(stageIndices) == 0 {
			empty := monoText("—", colorDimGray, false)
			cardArea = container.NewPadded(empty)
		} else {
			var cardItems []fyne.CanvasObject
			for _, wtIdx := range stageIndices {
				wt := d.state.Worktrees[wtIdx]
				isSelected := d.state.SelectedCard == wtIdx
				content := buildKanbanCardContent(
					wt,
					d.agentsFor(wt.Path),
					d.prFor(wt.Branch),
					isSelected,
				)
				cardWtIdx := wtIdx // capture for closure
				card := makeCard(content, isSelected, false, func() {
					d.state.SelectedCard = cardWtIdx
					if d.OnCardSelected != nil {
						d.OnCardSelected(cardWtIdx)
					}
					d.Rebuild()
				})
				// Wrap each card so its MinSize.Width = 1, forcing the VScroll to
				// size it to the column width rather than the natural text width.
				cardItems = append(cardItems, newKanbanCardWrapper(card))
			}
			cardArea = container.NewVScroll(container.NewVBox(cardItems...))
		}

		// ── Column border + background rectangle ─────────────────────────
		// Mirrors the website's .kb-col: coloured stroke + subtle body tint.
		colBg := canvas.NewRectangle(kanbanColumnBgColor(si))
		colBg.CornerRadius = 6
		colBg.StrokeColor = accentColor
		colBg.StrokeWidth = 1.5

		// NewPadded keeps the content (header + cards) away from the border stroke.
		colContent := container.NewBorder(headerSection, nil, nil, nil, cardArea)
		cols[si] = container.NewStack(colBg, container.NewPadded(colContent))
	}

	return container.NewGridWithColumns(4, cols...)
}
