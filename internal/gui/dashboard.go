package gui

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"fyne.io/fyne/v2/theme"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
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
	state                                                                     *RepoState
	content                                                                   *fyne.Container
	innerSlot                                                                 *fyne.Container     // holds the actual dashboard content for hot-swap
	scroll                                                                    *container.Scroll   // scrollable linked cards area
	cards                                                                     []fyne.CanvasObject // linked card widgets for scroll-to
	listScroll                                                                *container.Scroll
	mainCard                                                                  *tappableCard
	listRows                                                                  []fyne.CanvasObject
	listOffset                                                                fyne.Position
	inspectorScroll                                                           *container.Scroll
	inspectorPath                                                             string
	inspectorOffset                                                           fyne.Position
	inspectorViews                                                            [3]bool
	inspectorOpen                                                             bool
	boardScroll                                                               *container.Scroll
	stageScrolls                                                              [5]*container.Scroll
	stageViewports                                                            [5]*stageViewport
	stageCards                                                                [5][]fyne.CanvasObject
	boardColumns                                                              []fyne.CanvasObject
	gridOffset, boardOffset                                                   fyne.Position
	stageOffsets                                                              [5]fyne.Position
	mainExpanded                                                              bool
	builtView                                                                 ViewMode
	keyboardActive                                                            bool
	RepoName                                                                  string
	OnCreate, OnCreateFromIssue, OnFetchPR, OnRefresh                         func()
	OnViewChanged                                                             func(ViewMode)
	OnMainTerminal, OnMainEditor, OnMainNotes                                 func()
	OnOpenTerminal, OnOpenEditor, OnEditNotes, OnActivity                     func()
	OnPull, OnSendPR, OnDelete, OnStartSandbox, OnStopSandbox, OnSetupSandbox func()

	// OnCardSelected is called when a card is clicked. The index is the
	// worktree index (0=main, 1+=linked).
	OnCardSelected func(idx int)

	// OnNoteRequested fires when the user right-clicks a card and wants to
	// open the per-worktree note editor.
	OnNoteRequested func(wt git.Worktree)
}

// NewDashboard creates a dashboard from the given repo state.
func NewDashboard(state *RepoState) *Dashboard {
	d := &Dashboard{state: state, keyboardActive: true, inspectorOpen: state.ViewMode == ViewList}
	d.inspectorViews[ViewList] = true
	inner := d.build()
	d.builtView = state.ViewMode
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
//
// Returns false when the result is dropped because its snapshot generation
// predates the last applied snapshot (the refresh started before a mutation
// the user has since performed). Callers should skip any downstream work
// that derives from this snapshot when false is returned. Errors always
// pass through so users see them.
func (d *Dashboard) ApplyRefresh(result ops.RefreshResult) bool {
	previousSelection := d.state.SelectedCard
	wasVisible := d.selectedCardVisible()
	previousStage, previousRow := -1, -1
	if previousSelection > 0 && d.state.ViewMode == ViewKanban {
		previousStage = d.KanbanColumnOf(previousSelection)
		previousRow = d.KanbanRowOf(previousSelection, d.KanbanStages())
	}
	if !d.state.Apply(result) {
		return false
	}

	d.Rebuild()
	// Preserve manual scroll on ordinary updates, but follow the selected
	// worktree when a refresh moves it to a different row or lifecycle stage.
	moved := previousSelection != d.state.SelectedCard
	if d.state.SelectedCard > 0 && d.state.ViewMode == ViewKanban {
		moved = previousStage != d.KanbanColumnOf(d.state.SelectedCard) || previousRow != d.KanbanRowOf(d.state.SelectedCard, d.KanbanStages())
	}
	if moved && wasVisible {
		d.EnsureVisible()
	}
	return true
}

// selectedCardVisible detects intersection with the viewport before a refresh,
// so preserving a selected card does not interrupt intentional manual browsing.
func (d *Dashboard) selectedCardVisible() bool {
	index := d.state.SelectedCard
	if index <= 0 {
		return false
	}
	intersects := func(start, extent, offset, viewport float32) bool {
		return start < offset+viewport && start+extent > offset
	}
	if d.state.ViewMode == ViewKanban {
		stage := d.KanbanColumnOf(index)
		row := d.KanbanRowOf(index, d.KanbanStages())
		if d.boardScroll == nil || stage >= len(d.boardColumns) || row >= len(d.stageCards[stage]) || d.stageScrolls[stage] == nil {
			return false
		}
		column, card, scroll := d.boardColumns[stage], d.stageCards[stage][row], d.stageScrolls[stage]
		return intersects(column.Position().X, column.Size().Width, d.boardScroll.Offset.X, d.boardScroll.Size().Width) && intersects(card.Position().Y, card.Size().Height, scroll.Offset.Y, scroll.Size().Height)
	}
	if d.state.ViewMode == ViewList {
		if d.listScroll == nil || index > len(d.listRows) {
			return false
		}
		row := d.listRows[index-1]
		return intersects(row.Position().Y, row.Size().Height, d.listScroll.Offset.Y, d.listScroll.Size().Height)
	}
	if d.scroll == nil || index > len(d.cards) {
		return false
	}
	grid := d.scroll.Content.(*fyne.Container).Objects[1]
	card := d.cards[index-1]
	return intersects(grid.Position().Y+card.Position().Y, card.Size().Height, d.scroll.Offset.Y, d.scroll.Size().Height)
}

// EnsureVisible scrolls the linked cards area so the selected card is visible.
func (d *Dashboard) EnsureVisible() {
	if d.state.ViewMode == ViewList {
		index := d.state.SelectedCard
		if d.listScroll != nil && index > 0 && index <= len(d.listRows) {
			row := d.listRows[index-1]
			reveal(d.listScroll, row.Position().Y, row.Size().Height, false)
		}
		return
	}
	if d.state.SelectedCard <= 0 {
		return
	}
	if d.state.ViewMode == ViewKanban {
		stage := d.KanbanColumnOf(d.state.SelectedCard)
		if d.boardScroll != nil && stage < len(d.boardColumns) {
			reveal(d.boardScroll, d.boardColumns[stage].Position().X, d.boardColumns[stage].Size().Width, true)
			d.refreshStageViewports()
		}
		row := d.KanbanRowOf(d.state.SelectedCard, d.KanbanStages())
		if row < len(d.stageCards[stage]) && d.stageScrolls[stage] != nil {
			card := d.stageCards[stage][row]
			reveal(d.stageScrolls[stage], card.Position().Y, card.Size().Height, false)
		}
		return
	}
	idx := d.state.SelectedCard - 1
	if d.scroll == nil || idx >= len(d.cards) {
		return
	}
	card := d.cards[idx]
	// Cards live below the Worktrees heading, so include their grid position.
	grid := d.scroll.Content.(*fyne.Container).Objects[1]
	reveal(d.scroll, grid.Position().Y+card.Position().Y, card.Size().Height, false)
}
func reveal(scroll *container.Scroll, start, extent float32, horizontal bool) {
	offset, viewport := scroll.Offset.Y, scroll.Size().Height
	if horizontal {
		offset, viewport = scroll.Offset.X, scroll.Size().Width
	}
	target := offset
	if start < offset {
		target = start
	} else if start+extent > offset+viewport {
		target = start + extent - viewport
	}
	if horizontal {
		scroll.ScrollToOffset(fyne.NewPos(target, scroll.Offset.Y))
	} else {
		scroll.ScrollToOffset(fyne.NewPos(scroll.Offset.X, target))
	}
}

// Rebuild recreates the dashboard layout from current state.
// Must be called on the main thread.
func (d *Dashboard) Rebuild() {
	if d.inspectorScroll != nil {
		d.inspectorOffset = d.inspectorScroll.Offset
	}
	if d.state.ViewMode != d.builtView {
		d.inspectorViews[d.builtView] = d.inspectorOpen
		d.inspectorOpen = d.inspectorViews[d.state.ViewMode]
	}
	switch d.builtView {
	case ViewList:
		if d.listScroll != nil {
			d.listOffset = d.listScroll.Offset
		}
	case ViewGrid:
		if d.scroll != nil {
			d.gridOffset = d.scroll.Offset
		}
	default:
		if d.boardScroll != nil {
			d.boardOffset = d.boardScroll.Offset
		}
		for i, scroll := range d.stageScrolls {
			if scroll != nil {
				d.stageOffsets[i] = scroll.Offset
			}
		}
	}
	inner := d.build()
	d.innerSlot.Objects = []fyne.CanvasObject{inner}
	d.innerSlot.Refresh()
	// Constructing Border/Stack containers can temporarily lay out retained
	// scrolls at zero size. Restore offsets only after the new tree is laid out.
	if d.state.ViewMode == ViewKanban {
		if d.boardScroll != nil {
			d.boardScroll.ScrollToOffset(d.boardOffset)
		}
		for i, scroll := range d.stageScrolls {
			if scroll != nil {
				scroll.ScrollToOffset(d.stageOffsets[i])
			}
		}
	} else if d.state.ViewMode == ViewList && d.listScroll != nil {
		d.listScroll.Content.Refresh()
		d.listScroll.Refresh()
		d.listScroll.ScrollToOffset(d.listOffset)
	} else if d.scroll != nil {
		d.scroll.Content.Refresh()
		d.scroll.Refresh()
		d.scroll.ScrollToOffset(d.gridOffset)
	}
	if d.inspectorOpen && d.inspectorScroll != nil {
		d.inspectorScroll.Content.Refresh()
		d.inspectorScroll.Refresh()
		d.inspectorScroll.ScrollToOffset(d.inspectorOffset)
	}
	d.refreshStageViewports()
	d.builtView = d.state.ViewMode
}

func (d *Dashboard) build() fyne.CanvasObject {
	d.mainCard = nil
	topItems := []fyne.CanvasObject{}
	header := d.header()
	if d.state.StatusMessage != "" {
		c := colorGreen
		if d.state.StatusIsError {
			c = colorRed
		}
		msg := newMeasuredText(d.state.StatusMessage, c, false, false, false)
		topItems = append(topItems, msg)
	}
	main := d.state.MainWorktree()
	if main == nil {
		return container.New(&workspaceRootLayout{}, header, inset(container.NewVBox(append(topItems, uiText("No worktrees found.", colorGray, false))...), spaceLG, spaceLG))
	}
	mainPanel := inset(d.mainSummary(*main), spaceMD, spaceSM)
	top := container.NewVBox(topItems...)
	var body fyne.CanvasObject
	linked := d.state.LinkedWorktrees()
	if len(linked) == 0 {
		create := newActionControl("Create a worktree", theme.ContentAddIcon(), true, func() {
			if d.OnCreate != nil {
				d.OnCreate()
			}
		})
		body = container.NewCenter(container.NewVBox(secondaryText("Create a worktree to start a task."), create))
	} else if d.state.ViewMode == ViewList {
		body = d.buildListView()
	} else if d.state.ViewMode == ViewKanban {
		board := d.buildKanbanView()
		if d.boardScroll == nil {
			d.boardScroll = container.NewHScroll(board)
			d.boardScroll.OnScrolled = func(_ fyne.Position) { d.refreshStageViewports() }
		} else {
			d.boardScroll.Content = board
		}
		d.boardScroll.Offset = d.boardOffset
		body = newBoardViewport(d)
	} else {
		cards := make([]fyne.CanvasObject, 0, len(linked))
		for i, wt := range linked {
			idx := i + 1
			content := buildCardContent(wt, d.agentsFor(wt.Path), d.idesFor(wt.Path), d.terminalsFor(wt.Path), d.prFor(wt.Branch), d.state.CLIAvail, d.state.Provider, nil, maxLinkedPathChars, d.state.SelectedCard == idx)
			card := makeCard(content, d.state.SelectedCard == idx, false, func() { d.selectCard(idx) })
			wtCopy := wt
			card.SetOnSecondaryTap(func() {
				if d.OnNoteRequested != nil {
					d.OnNoteRequested(wtCopy)
				}
			})
			cards = append(cards, card)
		}
		heading := secondaryText(fmt.Sprintf("Worktrees · %d", len(linked)))
		grid := container.New(newFlexGridLayout(cardCellSize()), cards...)
		section := container.NewVBox(heading, grid)
		d.cards = cards
		if d.scroll == nil {
			d.scroll = container.NewVScroll(section)
		} else {
			d.scroll.Content = section
		}
		d.scroll.Offset = d.gridOffset
		body = d.scroll
	}
	browser := inset(container.NewBorder(top, nil, nil, nil, body), spaceMD, spaceSM)
	if d.inspectorOpen {
		return container.New(&workspaceRootLayout{footer: true}, header, mainPanel, container.New(&workspacePaneLayout{list: d.state.ViewMode == ViewList}, browser, d.buildInspector()), d.helpBar())
	}
	return container.New(&workspaceRootLayout{footer: true}, header, mainPanel, browser, d.helpBar())
}

func (d *Dashboard) selectCard(idx int) {
	d.state.SelectedCard = idx
	if d.OnCardSelected != nil {
		d.OnCardSelected(idx)
	}
	d.Rebuild()
	d.EnsureVisible()
}

func (d *Dashboard) header() fyne.CanvasObject {
	name := d.RepoName
	if name == "" {
		if main := d.state.MainWorktree(); main != nil {
			name = filepath.Base(main.Path)
		} else {
			name = "Worktrees"
		}
	}
	title := newMeasuredText(name, colorForeground, true, false, false)
	title.txt.TextSize = scaledSize(13)
	mode := "Host"
	if m := d.state.ActiveMode; m != nil && m.Type == "sandbox" {
		mode = "Sandbox"
		if m.Agent != "" {
			mode += " · " + m.Agent
		}
	}
	local, network := "Waiting", "Waiting"
	if !d.state.LastLocalRefresh.IsZero() {
		local = d.state.LastLocalRefresh.Format("15:04:05")
	}
	if !d.state.LastNetworkRefresh.IsZero() {
		network = d.state.LastNetworkRefresh.Format("15:04:05")
	}
	context := newMeasuredText(mode+" · Local "+local+" · Network "+network, colorDimGray, false, false, false)
	context.txt.TextSize = scaledSize(textSecondarySize)
	board := newActionControl("Board", nil, false, func() { d.setView(ViewKanban) })
	if d.state.ViewMode != ViewKanban {
		board.keyHint = "g"
	}
	board.selected = d.state.ViewMode == ViewKanban
	list := newActionControl("List", nil, false, func() { d.setView(ViewList) })
	if d.state.ViewMode == ViewKanban {
		list.keyHint = "v"
	}
	list.selected = d.state.ViewMode == ViewList
	grid := newActionControl("Grid", nil, false, func() { d.setView(ViewGrid) })
	switch d.state.ViewMode {
	case ViewKanban:
		grid.keyHint = "g"
	case ViewList:
		grid.keyHint = "v"
	default:
		board.keyHint = "g/v"
	}
	grid.selected = d.state.ViewMode == ViewGrid
	refresh := newActionControl("Refresh", theme.ViewRefreshIcon(), false, func() {
		if d.OnRefresh != nil {
			d.OnRefresh()
		}
	})
	create := newActionControl("New Worktree", theme.ContentAddIcon(), true, func() {
		if d.OnCreate != nil {
			d.OnCreate()
		}
	})
	create.disabled = d.state.MainWorktree() == nil
	inspector := newActionControl("Inspector", theme.InfoIcon(), false, func() { d.inspectorOpen = !d.inspectorOpen; d.Rebuild() })
	inspector.keyHint = "Ctrl+I"
	if runtime.GOOS == "darwin" {
		inspector.keyHint = "⌘I"
	}
	inspector.selected = d.inspectorOpen
	more := newActionControl("More", theme.MoreHorizontalIcon(), false, nil)
	more.onTap = func() { d.showCreateMenu(more) }
	return container.New(&workspaceToolbarLayout{}, container.NewVBox(title, context), board, list, grid, refresh, create, more, inspector)
}
func (d *Dashboard) setView(v ViewMode) {
	if d.OnViewChanged != nil {
		d.OnViewChanged(v)
	} else if d.state.ViewMode != v {
		d.state.ViewMode = v
		d.Rebuild()
	}
}

func (d *Dashboard) mainSummary(wt git.Worktree) fyne.CanvasObject {
	detailsLabel := "Details"
	detailsIcon := theme.NavigateNextIcon()
	if d.mainExpanded {
		detailsLabel = "Hide details"
		detailsIcon = theme.MoveDownIcon()
	}
	disclosure := newActionControl(detailsLabel, detailsIcon, false, func() { d.mainExpanded = !d.mainExpanded; d.Rebuild() })
	more := newActionControl("More", theme.MoreHorizontalIcon(), false, nil)
	more.onTap = func() { d.showCreateMenu(more) }
	state := "Clean"
	if wt.IsDirty {
		state = "Uncommitted changes"
	}
	activity := []string{state, syncLabel(wt.Sync)}
	if agents := d.agentsFor(wt.Path); len(agents) > 0 {
		activity = append(activity, string(agents[0].Kind)+" active")
	}
	if ides := d.idesFor(wt.Path); len(ides) > 0 {
		activity = append(activity, string(ides[0].Kind))
	}
	if terminals := d.terminalsFor(wt.Path); len(terminals) > 0 {
		activity = append(activity, fmt.Sprintf("%d terminal(s)", len(terminals)))
	}
	if sbx := d.sandboxInfo(); sbx != nil {
		status := "not found"
		switch sbx.Status {
		case sandbox.StatusRunning:
			status = "running"
		case sandbox.StatusStopped:
			status = "stopped"
		}
		activity = append(activity, "Sandbox "+status)
	}
	heading := newMeasuredText("Main checkout", colorForeground, true, false, false)
	heading.txt.TextSize = scaledSize(13)
	summaryBranch := newMeasuredText(wt.Branch, colorForeground, true, true, false)
	summaryBranch.txt.TextSize = scaledSize(13)
	summaryPath := newMeasuredText(wt.Path, colorDimGray, false, true, true)
	summaryPath.txt.TextSize = scaledSize(11)
	summaryStatus := newMeasuredText(strings.Join(activity, " · "), colorGray, false, false, false)
	summaryStatus.txt.TextSize = scaledSize(11)
	terminal := inspectorAction("Terminal", theme.ComputerIcon(), false, d.OnMainTerminal)
	editor := inspectorAction("Editor", theme.DocumentCreateIcon(), false, d.OnMainEditor)
	note := inspectorAction("Notes", theme.DocumentIcon(), false, d.OnMainNotes)
	if d.state.SelectedCard == 0 && d.keyboardActive {
		terminal.keyHint, editor.keyHint, note.keyHint = "Enter", "e", "m"
	}
	summary := container.New(&prominentMainLayout{}, heading, summaryBranch, summaryPath, summaryStatus, terminal, editor, note, disclosure, more)
	items := []fyne.CanvasObject{summary}
	if d.mainExpanded {
		details := buildCardContent(wt, d.agentsFor(wt.Path), d.idesFor(wt.Path), d.terminalsFor(wt.Path), d.prFor(wt.Branch), d.state.CLIAvail, d.state.Provider, d.sandboxInfo(), maxMainPathChars, d.state.SelectedCard == 0).(*fyne.Container)
		// Branch, path and checkout status already appear in the summary.
		metadata := details.Objects[2 : len(details.Objects)-1]
		if len(metadata) == 0 {
			metadata = []fyne.CanvasObject{secondaryText("No active agents, editors or terminals detected.")}
		}
		fullBranch := widget.NewLabelWithStyle(wt.Branch, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		fullBranch.Wrapping = fyne.TextWrapBreak
		fullPath := widget.NewLabelWithStyle(wt.Path, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		fullPath.Wrapping = fyne.TextWrapBreak
		fullValues := container.NewVBox(secondaryText("Branch"), fullBranch, secondaryText("Path"), fullPath)
		items = append(items, widget.NewSeparator(), fullValues, container.NewVBox(metadata...))
	}
	card := makeCard(container.NewVBox(items...), d.state.SelectedCard == 0, true, func() { d.selectCard(0) })
	d.mainCard = card
	card.SetOnSecondaryTap(func() {
		if d.OnNoteRequested != nil {
			d.OnNoteRequested(wt)
		}
	})
	return card
}

func (d *Dashboard) helpBar() fyne.CanvasObject {
	viewKeys := "g Board/Grid · v Board/List/Grid"
	text := "Projects · ↑ ↓ select mode · a Add repository · n Add sandbox mode · x Remove mode · Enter/Tab Worktrees\n" + viewKeys
	if d.keyboardActive {
		arrows := "↑ ↓ ← →"
		if d.state.ViewMode == ViewList {
			arrows = "↑ ↓"
		}
		text = "Worktrees · " + arrows + " navigate · Tab Projects · Esc Clear status · " + viewKeys + " · Ctrl/Cmd+I Inspector\n"
		actions := "Enter Terminal · e Editor · m Notes · l Activity · r Refresh · p Pull"
		if d.state.SelectedCard == 0 {
			actions += " · c New worktree"
			if d.state.Provider == provider.ProviderGitHub {
				actions += " · i From issue · f Fetch PR"
			}
			if mode := d.state.ActiveMode; mode != nil && mode.Type == "sandbox" {
				if d.state.SandboxStatus == sandbox.StatusNotFound {
					actions += " · n Create sandbox"
				} else {
					actions += " · d Remove sandbox"
					switch d.state.SandboxStatus {
					case sandbox.StatusStopped:
						actions += " · s Start sandbox"
					case sandbox.StatusRunning:
						actions += " · Shift+S Stop sandbox"
					}
				}
			} else {
				actions += " · n Create/enroll sandbox"
			}
		} else {
			actions += " · d Delete worktree"
			if d.state.Provider != provider.ProviderUnknown {
				actions += " · Shift+P Send PR"
			}
		}
		text += actions
	}
	help := newWrappedValue(text, false)
	help.muted = true
	return inset(help, spaceMD, spaceXS)
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

func (d *Dashboard) terminalsFor(wtPath string) []terminal.Info {
	if d.state.Terminals == nil {
		return nil
	}
	return d.state.Terminals[wtPath]
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
		Kits:          mode.Kits,
	}
}
