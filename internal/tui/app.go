package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/config"
	"github.com/mdelapenya/gwaim/internal/git"
)

type focusPanel int

const (
	focusLeft  focusPanel = iota // repo list
	focusRight                   // worktree dashboard
)

type appMode int

const (
	appModeNormal        appMode = iota
	appModeAddRepo                      // text input for repo path
	appModeConfirmRemove                // confirmation prompt for repo removal
)

// repoTab holds the state for a single registered repository.
type repoTab struct {
	path  string // repo root path (unique key)
	name  string // display name
	model Model  // per-repo worktree dashboard
}

// panelScroll holds scroll state for a bordered panel's scrollbar.
type panelScroll struct {
	total, visible, offset int
}

// App is the top-level bubbletea model that manages multiple repositories.
type App struct {
	repos            []*repoTab
	active           int        // selected repo index
	focus            focusPanel // which panel has focus
	detector         *agent.Detector
	configPath       string
	width            int
	height           int
	mode             appMode
	textInput        textinput.Model
	statusMsg        string
	refreshInterval  time.Duration
	repoScrollOffset int // first visible repo card index
}

// addRepoMsg is returned after validating and opening a new repo.
type addRepoMsg struct {
	path string
	name string
	repo *git.Repository
	err  error
}

// NewApp creates a new App model.
func NewApp(configPath string, detector *agent.Detector, refreshInterval time.Duration) App {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repository"
	ti.CharLimit = 256
	ti.Width = 50

	if refreshInterval <= 0 {
		refreshInterval = DefaultNetworkRefreshInterval
	}

	return App{
		detector:        detector,
		configPath:      configPath,
		textInput:       ti,
		refreshInterval: refreshInterval,
	}
}

func (a App) Init() tea.Cmd {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		a.statusMsg = errorStyle.Render("Config: " + err.Error())
		return nil
	}

	var cmds []tea.Cmd
	for i, entry := range cfg.Repos {
		repo, err := git.OpenRepository(entry.Path)
		if err != nil {
			continue // skip repos that can't be opened
		}
		m := newEmbedded(repo, a.detector, a.refreshInterval)
		if i != 0 {
			// Non-active repos start paused — no tick chains.
			m.paused = true
		}
		a.repos = append(a.repos, &repoTab{
			path:  entry.Path,
			name:  entry.Name,
			model: m,
		})
		if i == 0 {
			// Only the active repo starts its full refresh cycle.
			cmds = append(cmds, m.Init())
		}
	}

	// Return the modified app as a cmd that sets up repos.
	// Since Init() can't mutate the receiver, we use a message.
	return func() tea.Msg {
		return appInitMsg{repos: a.repos, cmds: cmds}
	}
}

// appInitMsg carries the repos loaded during Init.
type appInitMsg struct {
	repos []*repoTab
	cmds  []tea.Cmd
}

// childResizeMsg is a WindowSizeMsg targeted at the active child model only.
// Unlike tea.WindowSizeMsg, it is NOT intercepted by the App's dimension tracking.
type childResizeMsg struct {
	width  int
	height int
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case appInitMsg:
		a.repos = msg.repos
		if len(a.repos) > 0 {
			a.focus = focusRight
		}
		// Non-active repos are already created paused in Init().
		// If we already received a WindowSizeMsg (which bubbletea sends before
		// Init cmd results), resize all children now so they're not stuck on "Loading...".
		var resizeCmds []tea.Cmd
		if a.width > 0 && len(a.repos) > 0 {
			w, h := a.childDimensions()
			sizeMsg := tea.WindowSizeMsg{Width: w, Height: h}
			for i := range a.repos {
				updated, cmd := a.repos[i].model.Update(sizeMsg)
				a.repos[i].model = updated.(Model)
				if cmd != nil {
					resizeCmds = append(resizeCmds, cmd)
				}
			}
		}
		return a, tea.Batch(append(msg.cmds, resizeCmds...)...)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Forward adjusted size to active child.
		if len(a.repos) > 0 && a.active < len(a.repos) {
			w, h := a.childDimensions()
			childMsg := tea.WindowSizeMsg{Width: w, Height: h}
			updated, cmd := a.repos[a.active].model.Update(childMsg)
			a.repos[a.active].model = updated.(Model)
			return a, cmd
		}
		return a, nil

	case childResizeMsg:
		if len(a.repos) > 0 && a.active < len(a.repos) {
			sizeMsg := tea.WindowSizeMsg{Width: msg.width, Height: msg.height}
			updated, cmd := a.repos[a.active].model.Update(sizeMsg)
			a.repos[a.active].model = updated.(Model)
			return a, cmd
		}
		return a, nil

	case addRepoMsg:
		return a.handleAddRepoMsg(msg)

	case tea.MouseMsg:
		return a.handleMouse(msg)

	case tea.KeyMsg:
		return a.handleKeyMsg(msg)
	}

	// Route repo-specific messages to matching child by repoPath.
	rp := extractRepoPath(msg)
	if rp != "" {
		for i, rt := range a.repos {
			if rt.path == rp {
				updated, cmd := rt.model.Update(msg)
				a.repos[i].model = updated.(Model)
				return a, cmd
			}
		}
		// No matching repo, discard stale message.
		return a, nil
	}

	// Forward unrecognized messages to active child.
	if len(a.repos) > 0 && a.active < len(a.repos) {
		updated, cmd := a.repos[a.active].model.Update(msg)
		a.repos[a.active].model = updated.(Model)
		return a, cmd
	}

	return a, nil
}

func (a App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle add repo mode.
	if a.mode == appModeAddRepo {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			input := strings.TrimSpace(a.textInput.Value())
			if input == "" {
				a.mode = appModeNormal
				a.statusMsg = ""
				return a, nil
			}
			a.mode = appModeNormal
			a.statusMsg = cleanStyle.Render("Adding repository...")
			return a, doAddRepo(input, a.configPath)
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			a.mode = appModeNormal
			a.statusMsg = ""
			return a, nil
		default:
			var cmd tea.Cmd
			a.textInput, cmd = a.textInput.Update(msg)
			return a, cmd
		}
	}

	// Handle confirm remove mode.
	if a.mode == appModeConfirmRemove {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			if a.active >= 0 && a.active < len(a.repos) {
				rt := a.repos[a.active]
				// Remove from config. Errors are non-fatal — the in-memory state is canonical.
				cfg, err := config.Load(a.configPath)
				if err == nil {
					cfg.Remove(rt.path)
					_ = config.Save(a.configPath, cfg)
				}

				// Remove from repos slice.
				a.repos = append(a.repos[:a.active], a.repos[a.active+1:]...)
				if a.active >= len(a.repos) && len(a.repos) > 0 {
					a.active = len(a.repos) - 1
				}
				if len(a.repos) == 0 {
					a.active = 0
					a.focus = focusLeft
				}
				a.statusMsg = cleanStyle.Render("Repository removed")

				// The new active repo may have been paused — resume it.
				if len(a.repos) > 0 && a.repos[a.active].model.paused {
					resumed, cmd := a.repos[a.active].model.Resume()
					a.repos[a.active].model = resumed
					a.mode = appModeNormal
					return a, cmd
				}
			}
			a.mode = appModeNormal
			return a, nil
		default:
			a.mode = appModeNormal
			a.statusMsg = ""
			return a, nil
		}
	}

	// Block all navigation when a modal popup is active (App or child level).
	childModal := len(a.repos) > 0 && a.active < len(a.repos) && !a.repos[a.active].model.IsNormal()
	if childModal {
		// Forward directly to the child so it can handle its own modal keys.
		updated, cmd := a.repos[a.active].model.Update(msg)
		a.repos[a.active].model = updated.(Model)
		return a, cmd
	}

	// Global quit.
	if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
		// Only quit if child is in normal mode (so 'q' in text input doesn't quit).
		if len(a.repos) == 0 || a.focus == focusLeft ||
			(a.focus == focusRight && a.active < len(a.repos) && a.repos[a.active].model.IsNormal()) {
			return a, tea.Quit
		}
	}

	// Tab / Shift+Tab switches focus.
	if key.Matches(msg, key.NewBinding(key.WithKeys("tab", "shift+tab"))) {
		if len(a.repos) > 0 {
			if a.focus == focusLeft {
				a.focus = focusRight
			} else {
				a.focus = focusLeft
			}
		}
		return a, nil
	}

	// Left panel navigation.
	if a.focus == focusLeft {
		return a.handleLeftPanelKey(msg)
	}

	// Right panel: forward to active child.
	if len(a.repos) > 0 && a.active < len(a.repos) {
		updated, cmd := a.repos[a.active].model.Update(msg)
		a.repos[a.active].model = updated.(Model)
		return a, cmd
	}

	return a, nil
}

func (a App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.mode != appModeNormal {
		return a, nil
	}
	// Block mouse when a child model has a modal popup active.
	if len(a.repos) > 0 && a.active < len(a.repos) && !a.repos[a.active].model.IsNormal() {
		return a, nil
	}
	// Handle scroll wheel in left panel.
	if (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) &&
		msg.X < a.leftPanelWidth() {
		if msg.Button == tea.MouseButtonWheelUp {
			a.repoScrollOffset--
		} else {
			a.repoScrollOffset++
		}
		a = a.clampRepoScroll()
		return a, nil
	}

	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		// Forward non-click mouse events (scroll, motion) to active child.
		if len(a.repos) > 0 && a.active < len(a.repos) {
			updated, cmd := a.repos[a.active].model.Update(msg)
			a.repos[a.active].model = updated.(Model)
			return a, cmd
		}
		return a, nil
	}

	if len(a.repos) == 0 {
		return a, nil
	}

	hh := a.headerHeight()
	leftWidth := a.leftPanelWidth()

	if msg.X < leftWidth {
		// Click in left panel — focus it and select the repo at this row.
		a.focus = focusLeft
		// Repo cards start at: header height + 1 (panel border) + 1 ("Repos" label).
		// Each card is repoCardHeight lines tall. Add scroll offset.
		contentY := msg.Y - hh - 2
		if contentY >= 0 {
			cardIdx := contentY/repoCardHeight + a.repoScrollOffset
			if cardIdx >= 0 && cardIdx < len(a.repos) && cardIdx != a.active {
				return a.switchRepo(cardIdx)
			}
		}
		return a, nil
	}

	// Click in right panel — focus it and forward to child.
	a.focus = focusRight
	if a.active < len(a.repos) {
		adjustedMsg := msg
		adjustedMsg.X = msg.X - leftWidth
		adjustedMsg.Y = msg.Y - hh - 1 // -1 for panel top border
		updated, cmd := a.repos[a.active].model.Update(adjustedMsg)
		a.repos[a.active].model = updated.(Model)
		return a, cmd
	}
	return a, nil
}

func (a App) handleLeftPanelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if a.active > 0 {
			return a.switchRepo(a.active - 1)
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if a.active < len(a.repos)-1 {
			return a.switchRepo(a.active + 1)
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		a.mode = appModeAddRepo
		a.textInput.Reset()
		a.textInput.Placeholder = "/path/to/repository"
		a.textInput.Focus()
		a.statusMsg = ""
		return a, a.textInput.Cursor.BlinkCmd()
	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		if len(a.repos) > 0 && a.active < len(a.repos) {
			a.mode = appModeConfirmRemove
		}
		return a, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		// Enter switches focus to right panel.
		if len(a.repos) > 0 {
			a.focus = focusRight
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleAddRepoMsg(msg addRepoMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
		return a, nil
	}

	// Pause the previously active repo.
	if a.active < len(a.repos) {
		a.repos[a.active].model = a.repos[a.active].model.Pause()
	}

	// Create the child model (starts unpaused — it's the new active).
	m := newEmbedded(msg.repo, a.detector, a.refreshInterval)
	rt := &repoTab{
		path:  msg.path,
		name:  msg.name,
		model: m,
	}
	a.repos = append(a.repos, rt)
	a.active = len(a.repos) - 1
	a.focus = focusRight
	a.statusMsg = cleanStyle.Render("Repository added: " + msg.name)

	// Resize the new child and start its refresh cycle.
	return a, tea.Batch(rt.model.Init(), a.resizeActiveChild())
}

// appTitleStyle is the title style for the App header — no MarginBottom,
// unlike the standalone titleStyle which has MarginBottom(1).
var appTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("39"))

func (a App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	// Empty state.
	if len(a.repos) == 0 {
		return a.renderEmptyState()
	}

	// --- Top header: title only (timestamps are inside the right panel) ---
	header := appTitleStyle.Render("gwaim - Git Worktree Agent Manager")

	// --- Two-column layout with manual borders ---
	leftWidth := a.leftPanelWidth()
	rightWidth := a.dashWidth()
	leftInner := leftWidth - 4 // border(1) + padding(1) each side
	if leftInner < 10 {
		leftInner = 10
	}
	rightInner := rightWidth - 4
	if rightInner < 20 {
		rightInner = 20
	}

	headerHeight := lipgloss.Height(header)
	contentH := a.height - headerHeight - 2 // -2 for top+bottom border rows
	if contentH < 1 {
		contentH = 1
	}

	leftContent := a.renderRepoList(leftWidth, contentH)
	rightContent := a.renderDashboard(rightWidth)

	// Scroll state for both panels.
	leftScroll := a.repoScrollState()
	var rightScroll panelScroll
	if a.active >= 0 && a.active < len(a.repos) {
		t, v, o := a.repos[a.active].model.ScrollState()
		rightScroll = panelScroll{total: t, visible: v, offset: o}
	}

	columns := a.buildPanels(leftContent, rightContent, leftInner, rightInner, contentH, leftScroll, rightScroll)

	// Assemble final output.
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(columns)

	if a.mode == appModeAddRepo {
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  Repository path: ") + a.textInput.View())
	} else if a.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(a.statusMsg)
	}

	result := b.String()

	// Overlay modal popups over the full screen.
	if a.mode == appModeConfirmRemove {
		popup := a.renderConfirmRemovePopup()
		result = overlayCenter(result, popup, a.width, a.height)
	} else if a.active >= 0 && a.active < len(a.repos) && a.repos[a.active].model.mode == modeConfirmDelete {
		popup := a.repos[a.active].model.renderConfirmPopup()
		result = overlayCenter(result, popup, a.width, a.height)
	}

	return result
}

// renderConfirmRemovePopup renders a centered confirmation popup for repo removal.
func (a App) renderConfirmRemovePopup() string {
	if a.active < 0 || a.active >= len(a.repos) {
		return ""
	}
	name := a.repos[a.active].name
	msg := fmt.Sprintf("Remove %q from dashboard?\n\n[y] confirm  [any key] cancel", name)
	return popupStyle.Render(msg)
}

func (a App) renderEmptyState() string {
	title := titleStyle.Render("gwaim - Git Worktree Agent Manager")
	body := "\n\nNo repositories registered.\nPress 'a' to add a repository.\n"

	if a.mode == appModeAddRepo {
		body += "\n" + inputPromptStyle.Render("  Repository path: ") + a.textInput.View() + "\n"
	}
	if a.statusMsg != "" {
		body += "\n" + a.statusMsg + "\n"
	}

	help := helpStyle.Render("a add repo • q quit")
	return title + body + "\n" + help
}

// repoCardHeight is the number of lines each repo card occupies
// (top border + content + bottom border).
const repoCardHeight = 3

func (a App) renderRepoList(width, panelHeight int) string {
	innerWidth := width - 4 // panel border + padding
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Card content width: innerWidth minus card border(2) and card padding(2).
	cardContentWidth := innerWidth - 4
	// Name max: card content minus marker(2).
	maxNameWidth := cardContentWidth - 2
	if maxNameWidth < 5 {
		maxNameWidth = 5
	}

	// Calculate visible range based on scroll offset.
	visibleCards := (panelHeight - 2) / repoCardHeight // -2 for header + help
	if visibleCards < 1 {
		visibleCards = 1
	}
	start := a.repoScrollOffset
	end := start + visibleCards
	if end > len(a.repos) {
		end = len(a.repos)
	}

	// Top section: header + visible repo cards.
	var parts []string
	parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("Repos"))

	for i := start; i < end; i++ {
		rt := a.repos[i]
		marker := "  "
		if i == a.active {
			marker = "▸ "
		}

		name := rt.name
		if len(name) > maxNameWidth {
			name = name[:maxNameWidth-1] + "…"
		}

		nameStyle := unselectedRepoStyle
		cs := repoCardStyle.Width(cardContentWidth)
		if i == a.active {
			nameStyle = selectedRepoStyle
			cs = selectedRepoCardStyle.Width(cardContentWidth)
		}

		parts = append(parts, cs.Render(marker+nameStyle.Render(name)))
	}

	topContent := strings.Join(parts, "\n")

	// Bottom section: help hint.
	help := repoPanelHelpStyle.Render("[a]dd [x]rm")

	// Combine top and bottom with fill space between them.
	content := lipgloss.JoinVertical(lipgloss.Left,
		topContent,
		lipgloss.NewStyle().Height(panelHeight-lipgloss.Height(topContent)-1).Render(""),
		help,
	)

	// Hard-clamp to panelHeight lines so it never exceeds the panel.
	lines := strings.Split(content, "\n")
	if len(lines) > panelHeight {
		lines = lines[:panelHeight]
	}
	return strings.Join(lines, "\n")
}

func (a App) renderDashboard(width int) string {
	if a.active >= 0 && a.active < len(a.repos) {
		return a.repos[a.active].model.ViewContent()
	}
	return ""
}

// switchRepo pauses the current repo's ticks, changes active, and resumes the new one.
// Returns the App and a tea.Cmd that starts the new repo's tick chains + resize.
func (a App) switchRepo(newIdx int) (App, tea.Cmd) {
	if newIdx == a.active || newIdx < 0 || newIdx >= len(a.repos) {
		return a, nil
	}
	// Pause old.
	if a.active < len(a.repos) {
		a.repos[a.active].model = a.repos[a.active].model.Pause()
	}
	a.active = newIdx
	a = a.ensureActiveRepoVisible()
	// Resume new.
	resumed, cmd := a.repos[a.active].model.Resume()
	a.repos[a.active].model = resumed
	return a, tea.Batch(cmd, a.resizeActiveChild())
}

// visibleRepoCards returns how many repo cards fit in the left panel.
func (a App) visibleRepoCards() int {
	contentH := a.height - a.headerHeight() - 2 // -2 for panel border
	if contentH < 1 {
		contentH = 1
	}
	// Subtract 2 for "Repos" header (1) + help footer (1).
	v := (contentH - 2) / repoCardHeight
	if v < 1 {
		v = 1
	}
	return v
}

// ensureActiveRepoVisible adjusts repoScrollOffset so the active repo is visible.
func (a App) ensureActiveRepoVisible() App {
	visible := a.visibleRepoCards()
	if a.active < a.repoScrollOffset {
		a.repoScrollOffset = a.active
	}
	if a.active >= a.repoScrollOffset+visible {
		a.repoScrollOffset = a.active - visible + 1
	}
	a = a.clampRepoScroll()
	return a
}

// clampRepoScroll ensures repoScrollOffset is within valid bounds.
func (a App) clampRepoScroll() App {
	visible := a.visibleRepoCards()
	maxOffset := len(a.repos) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if a.repoScrollOffset > maxOffset {
		a.repoScrollOffset = maxOffset
	}
	if a.repoScrollOffset < 0 {
		a.repoScrollOffset = 0
	}
	return a
}

// repoScrollState returns the scroll state for the left panel scrollbar.
func (a App) repoScrollState() panelScroll {
	return panelScroll{
		total:   len(a.repos),
		visible: a.visibleRepoCards(),
		offset:  a.repoScrollOffset,
	}
}

// headerHeight returns the number of visible rows the header occupies.
func (a App) headerHeight() int {
	return lipgloss.Height(appTitleStyle.Render("gwaim - Git Worktree Agent Manager"))
}

// leftPanelWidth returns the width of the repo list panel (15%, min 20).
func (a App) leftPanelWidth() int {
	w := a.width * 15 / 100
	if w < 20 {
		w = 20
	}
	return w
}

// dashWidth returns the width of the dashboard panel.
func (a App) dashWidth() int {
	return a.width - a.leftPanelWidth()
}

// childDimensions returns the width and height for a child model's content area.
// Consistent across WindowSizeMsg forwarding, resizeActiveChild, and appInitMsg.
func (a App) childDimensions() (int, int) {
	w := a.dashWidth() - 4 // border(2) + padding(2)
	if w < 20 {
		w = 20
	}
	h := a.height - a.headerHeight() - 2 // -2 for panel border
	if h < 1 {
		h = 1
	}
	return w, h
}

// resizeActiveChild sends a childResizeMsg to resize the active child model.
// Uses childResizeMsg (not tea.WindowSizeMsg) to avoid the App overwriting
// its own stored terminal dimensions with the child's smaller dimensions.
func (a App) resizeActiveChild() tea.Cmd {
	if len(a.repos) == 0 || a.active >= len(a.repos) {
		return nil
	}
	w, h := a.childDimensions()
	return func() tea.Msg {
		return childResizeMsg{width: w, height: h}
	}
}

// scrollbarGeometry computes thumb height and top position for a scrollbar.
// Returns (thumbHeight, thumbTop, scrollable).
func scrollbarGeometry(s panelScroll, contentH int) (int, int, bool) {
	if s.total <= s.visible {
		return 0, 0, false
	}
	thumbHeight := max(1, contentH*s.visible/s.total)
	scrollableItems := s.total - s.visible
	thumbTop := 0
	if scrollableItems > 0 {
		thumbTop = s.offset * (contentH - thumbHeight) / scrollableItems
	}
	if thumbTop+thumbHeight > contentH {
		thumbTop = contentH - thumbHeight
	}
	return thumbHeight, thumbTop, true
}

// buildPanels renders two side-by-side bordered panels with manual border
// characters. This avoids lipgloss border height bugs by controlling every
// row explicitly. Returns exactly contentH + 2 lines (content + top/bottom).
// leftScroll/rightScroll drive each panel's scrollbar.
func (a App) buildPanels(leftContent, rightContent string, leftInner, rightInner, contentH int, leftScroll, rightScroll panelScroll) string {
	// Split and clamp/pad both contents to exactly contentH lines.
	leftLines := splitClampPad(leftContent, contentH)
	rightLines := splitClampPad(rightContent, contentH)

	// Border color: cyan when focused, default otherwise.
	lbStyle := lipgloss.NewStyle()
	rbStyle := lipgloss.NewStyle()
	if a.focus == focusLeft {
		lbStyle = lbStyle.Foreground(lipgloss.Color("39"))
	}
	if a.focus == focusRight {
		rbStyle = rbStyle.Foreground(lipgloss.Color("39"))
	}

	// Scrollbar geometry for both panels.
	lThumbH, lThumbTop, lScrollable := scrollbarGeometry(leftScroll, contentH)
	rThumbH, rThumbTop, rScrollable := scrollbarGeometry(rightScroll, contentH)

	// Width-forcing style for content cells (pad short, truncate long).
	leftCellStyle := lipgloss.NewStyle().Width(leftInner).MaxWidth(leftInner)
	rightCellStyle := lipgloss.NewStyle().Width(rightInner).MaxWidth(rightInner)

	rows := make([]string, 0, contentH+2)

	// Top border.
	rows = append(rows,
		lbStyle.Render("╭"+strings.Repeat("─", leftInner+2)+"╮")+
			rbStyle.Render("╭"+strings.Repeat("─", rightInner+2)+"╮"))

	// Content rows.
	for i := range contentH {
		lc := leftCellStyle.Render(leftLines[i])
		rc := rightCellStyle.Render(rightLines[i])

		// Left panel right border: show scrollbar thumb or track.
		leftBorder := lbStyle.Render("│")
		if lScrollable {
			if i >= lThumbTop && i < lThumbTop+lThumbH {
				leftBorder = scrollThumbStyle.Render("┃")
			} else {
				leftBorder = scrollTrackStyle.Render("│")
			}
		}

		// Right panel right border: show scrollbar thumb or track.
		rightBorder := rbStyle.Render("│")
		if rScrollable {
			if i >= rThumbTop && i < rThumbTop+rThumbH {
				rightBorder = scrollThumbStyle.Render("┃")
			} else {
				rightBorder = scrollTrackStyle.Render("│")
			}
		}

		rows = append(rows,
			lbStyle.Render("│")+" "+lc+" "+leftBorder+
				rbStyle.Render("│")+" "+rc+" "+rightBorder)
	}

	// Bottom border.
	rows = append(rows,
		lbStyle.Render("╰"+strings.Repeat("─", leftInner+2)+"╯")+
			rbStyle.Render("╰"+strings.Repeat("─", rightInner+2)+"╯"))

	return strings.Join(rows, "\n")
}

// splitClampPad splits a string into lines, then ensures exactly n lines
// by truncating excess or padding with empty strings.
func splitClampPad(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// extractRepoPath pulls the repoPath from repo-specific message types.
func extractRepoPath(msg tea.Msg) string {
	switch m := msg.(type) {
	case refreshMsg:
		return m.repoPath
	case tickMsg:
		return m.repoPath
	case localTickMsg:
		return m.repoPath
	case worktreeCreatedMsg:
		return m.repoPath
	case worktreeRemovedMsg:
		return m.repoPath
	case prFetchedMsg:
		return m.repoPath
	case pullMsg:
		return m.repoPath
	case worktreeRepairedMsg:
		return m.repoPath
	case localFlashDoneMsg:
		return m.repoPath
	case netFlashDoneMsg:
		return m.repoPath
	case cliCheckMsg:
		return m.repoPath
	}
	return ""
}

// doAddRepo validates a path and opens it as a git repository.
func doAddRepo(input, configPath string) tea.Cmd {
	return func() tea.Msg {
		root, err := git.RepoRoot(input)
		if err != nil {
			return addRepoMsg{err: fmt.Errorf("not a git repository: %s", input)}
		}

		repo, err := git.OpenRepository(root)
		if err != nil {
			return addRepoMsg{err: fmt.Errorf("cannot open repository: %w", err)}
		}

		name := repo.RepoName()

		// Persist to config.
		cfg, _ := config.Load(configPath)
		cfg.Add(root, name)
		_ = config.Save(configPath, cfg)

		return addRepoMsg{path: root, name: name, repo: repo}
	}
}
