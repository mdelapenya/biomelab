package gui

import (
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"fyne.io/fyne/v2/theme"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// flexGridLayout lays out cards on a grid where the column count is determined
// by how many min-sized cells fit in the parent width, but the actual cell
// width is stretched so the row fills the full width (minus padding between
// cells). Height stays at min. Column count is capped at len(objects) so a
// small number of cards still spreads across the row.
type flexGridLayout struct {
	minCellSize fyne.Size
	colCount    int
	rowCount    int
}

func newFlexGridLayout(minCellSize fyne.Size) *flexGridLayout {
	return &flexGridLayout{minCellSize: minCellSize, colCount: 1, rowCount: 1}
}

func (g *flexGridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	padding := theme.Padding()
	g.colCount = 1
	g.rowCount = 0

	if size.Width > g.minCellSize.Width {
		g.colCount = int(math.Floor(float64(size.Width+padding) / float64(g.minCellSize.Width+padding)))
	}
	if g.colCount < 1 {
		g.colCount = 1
	}
	if visible := countVisible(objects); g.colCount > visible && visible > 0 {
		g.colCount = visible
	}

	cellH := g.minCellSize.Height
	cellW := (size.Width - padding*float32(g.colCount-1)) / float32(g.colCount)
	if cellW < g.minCellSize.Width {
		cellW = g.minCellSize.Width
	}

	i, x, y := 0, float32(0), float32(0)
	for _, child := range objects {
		if !child.Visible() {
			continue
		}
		if i%g.colCount == 0 {
			g.rowCount++
		}
		child.Move(fyne.NewPos(x, y))
		child.Resize(fyne.NewSize(cellW, cellH))
		if (i+1)%g.colCount == 0 {
			x = 0
			y += cellH + padding
		} else {
			x += cellW + padding
		}
		i++
	}
}

func (g *flexGridLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	rows := g.rowCount
	if rows < 1 {
		rows = 1
	}
	return fyne.NewSize(g.minCellSize.Width,
		(g.minCellSize.Height*float32(rows))+(float32(rows-1)*theme.Padding()))
}

func countVisible(objects []fyne.CanvasObject) int {
	n := 0
	for _, o := range objects {
		if o.Visible() {
			n++
		}
	}
	return n
}

// baseCardSize is the card size at the default font size (14).
// Actual size scales proportionally with the theme text size.
var baseCardSize = fyne.NewSize(360, 200)
const baseTextSize float32 = 14

// cardCellSize computes the card cell size scaled to the current font.
func cardCellSize() fyne.Size {
	current := theme.TextSize()
	scale := current / baseTextSize
	return fyne.NewSize(baseCardSize.Width*scale, baseCardSize.Height*scale)
}

// Dashboard is the right-panel worktree dashboard for a single repo.
type Dashboard struct {
	state     *RepoState
	content   *fyne.Container
	innerSlot *fyne.Container      // holds the actual dashboard content for hot-swap
	scroll    *container.Scroll    // scrollable linked cards area
	cards     []fyne.CanvasObject  // linked card widgets for scroll-to

	// OnCardSelected is called when a card is clicked. The index is the
	// worktree index (0=main, 1+=linked).
	OnCardSelected func(idx int)
}

// NewDashboard creates a dashboard from the given repo state.
func NewDashboard(state *RepoState) *Dashboard {
	d := &Dashboard{state: state}
	inner := d.build()
	d.innerSlot = container.NewStack(inner)
	d.content = container.NewStack(d.innerSlot)
	return d
}

// Content returns the renderable dashboard layout.
func (d *Dashboard) Content() fyne.CanvasObject {
	return d.content
}

// ApplyRefresh updates the dashboard state from a refresh result and rebuilds.
// Must be called on the main thread (via fyne.Do).
func (d *Dashboard) ApplyRefresh(result ops.RefreshResult) {
	if result.Err != nil {
		d.state.StatusMessage = result.Err.Error()
		d.state.StatusIsError = true
		return
	}
	if result.Worktrees != nil {
		d.state.SetWorktrees(result.Worktrees)
	}
	if result.Agents != nil {
		d.state.Agents = result.Agents
	}
	if result.IDEs != nil {
		d.state.IDEs = result.IDEs
	}
	if result.HasPRs {
		d.state.PRs = result.PRs
		d.state.LastNetworkRefresh = time.Now()
		d.state.NetFlash = true
		// Clear flash after 1 second.
		time.AfterFunc(time.Second, func() {
			fyne.Do(func() {
				d.state.NetFlash = false
				d.Rebuild()
			})
		})
	} else {
		d.state.LastLocalRefresh = time.Now()
		d.state.LocalFlash = true
		time.AfterFunc(time.Second, func() {
			fyne.Do(func() {
				d.state.LocalFlash = false
				d.Rebuild()
			})
		})
	}
	// Update sandbox status only when the refresh actually checked it.
	if result.HasSbxStatus {
		d.state.SandboxStatus = result.SandboxStatus
	}
	if result.SbxClientVer != "" {
		d.state.SbxClientVersion = result.SbxClientVer
	}
	if result.SbxServerVer != "" {
		d.state.SbxServerVersion = result.SbxServerVer
	}
	d.state.StatusMessage = ""
	d.state.StatusIsError = false

	d.Rebuild()
}

// EnsureVisible scrolls the linked cards area so the selected card is visible.
func (d *Dashboard) EnsureVisible() {
	if d.scroll == nil || d.state.SelectedCard <= 0 {
		return
	}
	idx := d.state.SelectedCard - 1 // 0-based in linked cards
	if idx < 0 || idx >= len(d.cards) {
		return
	}

	// Compute the row of the selected card and scroll to show it.
	cellSize := cardCellSize()
	scrollW := d.scroll.Size().Width
	cols := int(scrollW / cellSize.Width)
	if cols < 1 {
		cols = 1
	}
	row := idx / cols
	// The section label adds some height above the grid.
	labelHeight := float32(scaledSize(11) + 12) // label + padding
	cardY := labelHeight + float32(row)*cellSize.Height
	cardBottom := cardY + cellSize.Height

	// Only scroll if the card is outside the visible area.
	visibleTop := d.scroll.Offset.Y
	visibleBottom := visibleTop + d.scroll.Size().Height

	if cardY < visibleTop {
		d.scroll.ScrollToOffset(fyne.NewPos(0, cardY))
	} else if cardBottom > visibleBottom {
		d.scroll.ScrollToOffset(fyne.NewPos(0, cardBottom-d.scroll.Size().Height))
	}
}

// Rebuild recreates the dashboard layout from current state.
// Must be called on the main thread.
func (d *Dashboard) Rebuild() {
	inner := d.build()
	d.innerSlot.Objects = []fyne.CanvasObject{inner}
	d.innerSlot.Refresh()
}

func (d *Dashboard) build() fyne.CanvasObject {
	// Refresh timestamps with flash indicators.
	localTs := "--:--:--"
	netTs := "--:--:--"
	localFlash := ""
	netFlash := ""
	if !d.state.LastLocalRefresh.IsZero() {
		localTs = d.state.LastLocalRefresh.Format("15:04:05")
	}
	if d.state.LocalFlash {
		localFlash = " ✓"
	}
	if !d.state.LastNetworkRefresh.IsZero() {
		netTs = d.state.LastNetworkRefresh.Format("15:04:05")
	}
	if d.state.NetFlash {
		netFlash = " ✓"
	}
	tsText := fmt.Sprintf("local: %s%s    net: %s%s", localTs, localFlash, netTs, netFlash)
	timestamps := monoText(tsText, colorDimGray, false)
	timestamps.TextSize = scaledSize(9)

	// Status message (errors or transient info).
	var statusLine fyne.CanvasObject
	if d.state.StatusMessage != "" {
		c := colorGreen
		if d.state.StatusIsError {
			c = colorRed
		}
		statusLine = monoText(d.state.StatusMessage, c, false)
	}

	// Main card.
	mainWt := d.state.MainWorktree()
	if mainWt == nil {
		return container.NewVBox(timestamps, monoText("No worktrees found.", colorGray, false))
	}

	mainSelected := d.state.SelectedCard == 0
	mainContent := buildCardContent(
		*mainWt,
		d.agentsFor(mainWt.Path),
		d.idesFor(mainWt.Path),
		d.prFor(mainWt.Branch),
		d.state.CLIAvail,
		d.state.Provider,
		d.sandboxInfo(),
		maxMainPathChars,
		mainSelected,
	)
	mainCard := makeCard(mainContent, mainSelected, true, func() {
		d.state.SelectedCard = 0
		if d.OnCardSelected != nil {
			d.OnCardSelected(0)
		}
		d.Rebuild()
	})

	// Contextual help below main card (dynamic, matches TUI).
	helpStr := "[c] create  [f] fetch PR  [p] pull"
	sbxInfo := d.sandboxInfo()
	if sbxInfo != nil {
		switch sbxInfo.Status {
		case sandbox.StatusRunning:
			helpStr += "  [S] stop  [d] del sandbox"
		case sandbox.StatusStopped:
			helpStr += "  [s] start  [d] del sandbox"
		case sandbox.StatusNotFound:
			helpStr += "  [n] create sandbox"
		}
	}
	helpColor := colorDimGray
	if mainSelected {
		helpColor = colorGray
	}
	helpText := monoText(helpStr, helpColor, false)
	helpText.TextSize = scaledSize(9)

	// Top section: timestamps + status + main card + help.
	topItems := []fyne.CanvasObject{timestamps}
	if statusLine != nil {
		topItems = append(topItems, statusLine)
	}
	topItems = append(topItems, mainCard, helpText)
	topSection := container.NewVBox(topItems...)

	// Linked cards.
	linked := d.state.LinkedWorktrees()
	if len(linked) == 0 {
		return container.NewBorder(topSection, d.helpBar(), nil, nil, nil)
	}

	var cards []fyne.CanvasObject
	for i, wt := range linked {
		wtIdx := i + 1 // offset by 1 because main card is index 0
		isSelected := d.state.SelectedCard == wtIdx
		content := buildCardContent(
			wt,
			d.agentsFor(wt.Path),
			d.idesFor(wt.Path),
			d.prFor(wt.Branch),
			d.state.CLIAvail,
			d.state.Provider,
			nil, // sandbox info only shown on main card
			maxLinkedPathChars,
			isSelected,
		)
		cardIdx := wtIdx // capture for closure
		cards = append(cards, makeCard(content, isSelected, false, func() {
			d.state.SelectedCard = cardIdx
			if d.OnCardSelected != nil {
				d.OnCardSelected(cardIdx)
			}
			d.Rebuild()
		}))
	}

	// Section header.
	sectionLabel := monoText("Worktrees", colorGray, true)
	sectionLabel.TextSize = scaledSize(11)

	grid := container.New(newFlexGridLayout(cardCellSize()), cards...)
	linkedSection := container.NewVBox(sectionLabel, grid)
	d.cards = cards
	d.scroll = container.NewScroll(linkedSection)

	return container.NewBorder(topSection, d.helpBar(), nil, nil, d.scroll)
}

func (d *Dashboard) helpBar() fyne.CanvasObject {
	bg := canvas.NewRectangle(colorPanelBg)
	bg.CornerRadius = 0

	help := monoText("↑↓ nav  [Tab] panel  [⏎] open  [e] editor  [r] refresh  [d] delete  [p] pull  [P] PR", colorDimGray, false)
	help.TextSize = scaledSize(9)

	return container.NewStack(bg, container.NewPadded(help))
}

func (d *Dashboard) agentsFor(wtPath string) []agent.Info {
	if d.state.Agents == nil {
		return nil
	}
	return d.state.Agents[wtPath]
}

func (d *Dashboard) idesFor(wtPath string) []ide.Info {
	if d.state.IDEs == nil {
		return nil
	}
	return d.state.IDEs[wtPath]
}

func (d *Dashboard) prFor(branch string) *provider.PRInfo {
	if d.state.PRs == nil {
		return nil
	}
	return d.state.PRs[branch]
}

func (d *Dashboard) sandboxInfo() *SandboxCardInfo {
	mode := d.state.ActiveMode
	if mode == nil || mode.Type != "sandbox" {
		return nil
	}
	return &SandboxCardInfo{
		Name:          mode.SandboxName,
		Status:        d.state.SandboxStatus,
		Agent:         mode.Agent,
		ClientVersion: d.state.SbxClientVersion,
		ServerVersion: d.state.SbxServerVersion,
	}
}
