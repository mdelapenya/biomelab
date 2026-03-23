package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/github"
	"github.com/mdelapenya/gwaim/internal/tui/card"
	"github.com/mdelapenya/gwaim/internal/warp"
)

// DefaultNetworkRefreshInterval is the default interval for network operations
// (git fetch + PR lookups). Controlled by --refresh / GWAIM_REFRESH.
const DefaultNetworkRefreshInterval = 30 * time.Second

// LocalRefreshInterval is the fixed interval for local state refreshes
// (dirty status, agent detection). Not user-configurable.
const LocalRefreshInterval = 5 * time.Second

// DefaultRefreshInterval is kept for backwards compatibility.
// Deprecated: use DefaultNetworkRefreshInterval.
const DefaultRefreshInterval = DefaultNetworkRefreshInterval

type mode int

const (
	modeNormal mode = iota
	modeCreate
	modeFetchPR
	modeConfirmDelete
)

// Model is the top-level bubbletea model for gwaim.
type Model struct {
	repo      *git.Repository
	detector  *agent.Detector
	worktrees []git.Worktree
	agents    agent.DetectionResult
	prs       github.PRResult
	ghAvail   github.GHAvailability
	cursor    int
	width     int
	height    int
	keys      keyMap
	mode      mode
	viewport  viewport.Model
	textInput textinput.Model
	err       error
	statusMsg string
	ready     bool
	// cardZones tracks bounding rects for click detection.
	// Each entry maps worktree index -> {x, y, width, height} in body coordinates.
	cardZones      []zone
	mouseOn         bool // default true, toggled with 'm'
	deleteConfirmed bool // true after user types 'y' in confirm-delete mode
	refreshInterval time.Duration
	localFlash        bool      // true while showing ✓ after a local refresh
	netFlash          bool      // true while showing ✓ after a network refresh
	lastLocalRefresh  time.Time // time of last completed local refresh
	lastNetworkRefresh time.Time // time of last completed network refresh
}

type zone struct {
	idx            int
	x, y, w, h    int
}

// New creates a new TUI model. refreshInterval controls how often network
// operations (fetch + PR lookups) run. Pass 0 to use DefaultNetworkRefreshInterval.
// Local state (dirty status, agents) always refreshes every LocalRefreshInterval.
func New(repo *git.Repository, detector *agent.Detector, refreshInterval time.Duration) Model {
	ti := textinput.New()
	ti.Placeholder = "branch-name"
	ti.CharLimit = 80
	ti.Width = 30

	if refreshInterval <= 0 {
		refreshInterval = DefaultNetworkRefreshInterval
	}

	return Model{
		repo:            repo,
		detector:        detector,
		keys:            defaultKeyMap(),
		textInput:       ti,
		mouseOn:         true,
		refreshInterval: refreshInterval,
	}
}

func (m Model) Init() tea.Cmd {
	// Fast initial load: branch names only, no dirty/agents/network.
	// Local refresh (dirty + agents) and network refresh (fetch + PRs) follow immediately.
	// gh pre-flight runs once at startup.
	return tea.Batch(
		doCheckGH(),
		doQuickRefresh(m.repo),
		doLocalRefresh(m.repo, m.detector),
		doNetworkRefresh(m.repo, m.detector, m.ghAvail),
		m.doLocalTick(),
		m.doTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3 // title + last-update line + margin
		footerHeight := 2 // help bar + margin
		vpHeight := m.height - headerHeight - footerHeight
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}
		m.syncViewport()
		return m, nil

	case ghCheckMsg:
		m.ghAvail = msg.avail
		return m, nil

	case tickMsg:
		return m, tea.Batch(doNetworkRefresh(m.repo, m.detector, m.ghAvail), m.doTick())

	case localTickMsg:
		return m, tea.Batch(doLocalRefresh(m.repo, m.detector), m.doLocalTick())

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.worktrees = msg.worktrees
			m.agents = msg.agents
			if msg.hasPRs {
				m.prs = msg.prs
			}
			m.err = nil
		}
		if msg.fetchErr != nil {
			m.statusMsg = errorStyle.Render("Fetch: " + msg.fetchErr.Error())
		}
		if m.cursor >= len(m.worktrees) && len(m.worktrees) > 0 {
			m.cursor = len(m.worktrees) - 1
		}
		// Update timestamps and trigger ✓ flash for periodic refreshes.
		var flashCmd tea.Cmd
		switch msg.source {
		case refreshSourceLocal:
			m.lastLocalRefresh = time.Now()
			m.localFlash = true
			flashCmd = tea.Tick(time.Second, func(_ time.Time) tea.Msg { return localFlashDoneMsg{} })
		case refreshSourceNetwork:
			m.lastNetworkRefresh = time.Now()
			m.netFlash = true
			flashCmd = tea.Tick(time.Second, func(_ time.Time) tea.Msg { return netFlashDoneMsg{} })
		}
		m.syncViewport()
		return m, flashCmd

	case localFlashDoneMsg:
		m.localFlash = false
		m.syncViewport()
		return m, nil

	case netFlashDoneMsg:
		m.netFlash = false
		m.syncViewport()
		return m, nil

	case worktreeCreatedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
			m.mode = modeNormal
			return m, tea.Batch(doQuickRefresh(m.repo), doLocalRefresh(m.repo, m.detector))
		}
		m.statusMsg = cleanStyle.Render("Worktree created — opening panel...")
		m.mode = modeNormal
		// Open a Warp panel in the new worktree with the agent command.
		newWtPath := filepath.Join(m.repo.Root(), ".gwaim-worktrees", msg.branchName)
		return m, tea.Batch(
			doQuickRefresh(m.repo),
			doLocalRefresh(m.repo, m.detector),
			doOpenWarpPanel(m.repo.RepoName(), git.Worktree{Path: newWtPath, Branch: msg.branchName}, nil),
		)

	case prFetchedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
			m.mode = modeNormal
			return m, tea.Batch(doQuickRefresh(m.repo), doLocalRefresh(m.repo, m.detector))
		}
		m.statusMsg = cleanStyle.Render("PR fetched — opening panel...")
		m.mode = modeNormal
		// Trigger a network refresh so the new worktree's PR badge appears immediately.
		return m, tea.Batch(
			doQuickRefresh(m.repo),
			doNetworkRefresh(m.repo, m.detector, m.ghAvail),
			doOpenWarpPanel(m.repo.RepoName(), git.Worktree{Path: msg.wtPath, Branch: msg.branchName}, nil),
		)

	case worktreeRemovedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Worktree removed")
		}
		m.mode = modeNormal
		return m, tea.Batch(doQuickRefresh(m.repo), doLocalRefresh(m.repo, m.detector))

	case warpOpenedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Warp: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Warp panel opened")
		}
		return m, nil

	case editorOpenedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Editor: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Editor opened")
		}
		return m, nil

	case pullMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Pull: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Pull complete")
		}
		// Pull changes local state only; sync status will update on next network tick.
		return m, tea.Batch(doQuickRefresh(m.repo), doLocalRefresh(m.repo, m.detector))

	case worktreeRepairedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Repair: " + msg.err.Error())
		} else if msg.output != "" {
			m.statusMsg = cleanStyle.Render("Repair: " + msg.output)
		} else {
			m.statusMsg = cleanStyle.Render("Nothing to repair")
		}
		return m, tea.Batch(doQuickRefresh(m.repo), doLocalRefresh(m.repo, m.detector))

	case tea.KeyMsg:
		updated, cmd := m.handleKey(msg)
		m = updated.(Model)
		m.syncViewport()
		return m, cmd

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			updated, cmd := m.handleClick(msg)
			m = updated.(Model)
			m.syncViewport()
			return m, cmd
		}
	}

	// Forward to viewport for scroll handling.
	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if cmd != nil {
			return m, cmd
		}
	}

	if m.mode == modeCreate || m.mode == modeFetchPR {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle create mode input.
	if m.mode == modeCreate {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			name := strings.TrimSpace(m.textInput.Value())
			if name == "" {
				m.mode = modeNormal
				return m, nil
			}
			m.mode = modeNormal
			return m, doCreateWorktree(m.repo, name)
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	// Handle fetch PR mode input.
	if m.mode == modeFetchPR {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			input := strings.TrimSpace(m.textInput.Value())
			if input == "" {
				m.mode = modeNormal
				return m, nil
			}
			m.mode = modeNormal
			m.statusMsg = cleanStyle.Render("Fetching PR...")
			return m, doFetchPR(m.repo, input)
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	// Handle confirm delete mode.
	if m.mode == modeConfirmDelete {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if m.deleteConfirmed && m.cursor >= 0 && m.cursor < len(m.worktrees) {
				wt := m.worktrees[m.cursor]
				m.mode = modeNormal
				m.deleteConfirmed = false
				return m, doRemoveWorktree(m.repo, wt.Branch)
			}
			m.mode = modeNormal
			m.deleteConfirmed = false
			m.statusMsg = ""
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.deleteConfirmed = true
			if m.cursor >= 0 && m.cursor < len(m.worktrees) {
				wt := m.worktrees[m.cursor]
				m.statusMsg = confirmStyle.Render(
					fmt.Sprintf("Delete worktree %q? Press Enter to confirm, Esc to cancel.", wt.Branch),
				)
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			m.deleteConfirmed = false
			m.statusMsg = ""
			return m, nil
		default:
			m.deleteConfirmed = false
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		}
	}

	// Normal mode.
	// Navigation: cursor 0 = main worktree (own row), cursor 1+ = linked grid.
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Left):
		if m.cursor > 1 {
			m.cursor--
		}
		m.statusMsg = ""

	case key.Matches(msg, m.keys.Right):
		if m.cursor >= 1 && m.cursor < len(m.worktrees)-1 {
			m.cursor++
		}
		m.statusMsg = ""

	case key.Matches(msg, m.keys.Up):
		if m.cursor == 0 {
			// Already at main, no-op.
		} else {
			cols := m.columns()
			linkedIdx := m.cursor - 1 // 0-based index within linked grid
			if linkedIdx-cols >= 0 {
				// Move up within linked grid.
				m.cursor -= cols
			} else {
				// First row of linked grid → go to main.
				m.cursor = 0
			}
		}
		m.statusMsg = ""

	case key.Matches(msg, m.keys.Down):
		if m.cursor == 0 {
			// From main → first linked card.
			if len(m.worktrees) > 1 {
				m.cursor = 1
			}
		} else {
			cols := m.columns()
			if m.cursor+cols < len(m.worktrees) {
				m.cursor += cols
			}
		}
		m.statusMsg = ""

	case key.Matches(msg, m.keys.Pull):
		m.statusMsg = cleanStyle.Render("Pulling...")
		return m, doPull(m.repo)

	case key.Matches(msg, m.keys.Repair):
		if m.cursor != 0 {
			return m, nil // repair only from main worktree
		}
		m.statusMsg = cleanStyle.Render("Repairing worktrees...")
		return m, doRepairWorktrees(m.repo)

	case key.Matches(msg, m.keys.Mouse):
		m.mouseOn = !m.mouseOn
		if m.mouseOn {
			m.statusMsg = cleanStyle.Render("Mouse enabled (click to select)")
			return m, tea.EnableMouseCellMotion
		}
		m.statusMsg = cleanStyle.Render("Mouse disabled (text selection available)")
		return m, tea.DisableMouse

	case key.Matches(msg, m.keys.Create):
		if m.cursor != 0 {
			return m, nil // create only from main worktree
		}
		m.mode = modeCreate
		m.textInput.Reset()
		m.textInput.Placeholder = "branch-name"
		m.textInput.Focus()
		m.statusMsg = ""
		return m, m.textInput.Cursor.BlinkCmd()

	case key.Matches(msg, m.keys.FetchPR):
		if m.cursor != 0 {
			return m, nil // fetch PR only from main worktree
		}
		if m.ghAvail != github.GHAvailable {
			m.statusMsg = errorStyle.Render("gh CLI required for PR fetch")
			return m, nil
		}
		m.mode = modeFetchPR
		m.textInput.Reset()
		m.textInput.Placeholder = "PR number or owner/repo#number"
		m.textInput.Focus()
		m.statusMsg = ""
		return m, m.textInput.Cursor.BlinkCmd()

	case key.Matches(msg, m.keys.Editor):
		if m.cursor >= 0 && m.cursor < len(m.worktrees) {
			wt := m.worktrees[m.cursor]
			return m, doOpenEditor(wt.Path)
		}

	case key.Matches(msg, m.keys.Enter):
		if m.cursor >= 0 && m.cursor < len(m.worktrees) {
			wt := m.worktrees[m.cursor]
			agents := m.agents[wt.Path]
			return m, doOpenWarpPanel(m.repo.RepoName(), wt, agents)
		}

	case key.Matches(msg, m.keys.Delete):
		if m.cursor >= 0 && m.cursor < len(m.worktrees) {
			wt := m.worktrees[m.cursor]
			if wt.IsMain {
				m.statusMsg = errorStyle.Render("Cannot delete the main worktree")
				return m, nil
			}
			m.mode = modeConfirmDelete
			m.deleteConfirmed = false
			m.statusMsg = confirmStyle.Render(
				fmt.Sprintf("Delete worktree %q? This removes the directory, branch, and prunes metadata. (y/N)", wt.Branch),
			)
		}
	}

	return m, nil
}

// syncViewport updates the viewport content from the current model state.
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderBody())
}

// renderBody produces the scrollable body content and updates cardZones for click detection.
func (m *Model) renderBody() string {
	var body strings.Builder
	m.cardZones = nil
	currentY := 0

	if m.err != nil {
		line := errorStyle.Render("Error: "+m.err.Error()) + "\n"
		body.WriteString(line)
		currentY += strings.Count(line, "\n")
	}

	cols := m.columns()
	if cols < 1 {
		cols = 1
	}
	cardWidth := m.cardWidth(cols)

	if len(m.worktrees) > 0 {
		mainWt := m.worktrees[0]
		agents := m.agents[mainWt.Path]
		content := card.Render(mainWt, agents, m.prs[mainWt.Branch], m.ghAvail)

		mainWidth := m.width - 4
		if mainWidth < 40 {
			mainWidth = 40
		}

		style := mainCardStyle.Width(mainWidth)
		if m.cursor == 0 {
			style = selectedMainCardStyle.Width(mainWidth)
		}
		rendered := style.Render(content)
		h := strings.Count(rendered, "\n") + 1

		m.cardZones = append(m.cardZones, zone{idx: 0, x: 0, y: currentY, w: mainWidth + 4, h: h})

		body.WriteString(rendered)
		body.WriteString("\n")
		currentY += h + 1

		if m.mode == modeCreate {
			body.WriteString(inputPromptStyle.Render("  New branch name: "))
			body.WriteString(m.textInput.View())
			body.WriteString("\n")
			currentY += 2
		}
		if m.mode == modeFetchPR {
			body.WriteString(inputPromptStyle.Render("  Fetch PR: "))
			body.WriteString(m.textInput.View())
			body.WriteString("\n")
			currentY += 2
		}
	}

	var linked []git.Worktree
	if len(m.worktrees) > 1 {
		linked = m.worktrees[1:]
	}
	if len(linked) > 0 {
		body.WriteString("\n")
		body.WriteString(sectionStyle.Render(fmt.Sprintf("Worktrees (%d)", len(linked))))
		body.WriteString("\n")
		currentY += 3 // blank + section header + newline

		var row []string
		var rowIndices []int
		for i, wt := range linked {
			agents := m.agents[wt.Path]
			content := card.Render(wt, agents, m.prs[wt.Branch], m.ghAvail)

			globalIdx := i + 1
			style := cardStyle.Width(cardWidth)
			if globalIdx == m.cursor {
				style = selectedCardStyle.Width(cardWidth)
			}

			rendered := style.Render(content)
			row = append(row, rendered)
			rowIndices = append(rowIndices, globalIdx)

			if len(row) == cols || i == len(linked)-1 {
				joined := lipgloss.JoinHorizontal(lipgloss.Top, row...)
				rowH := strings.Count(joined, "\n") + 1

				// Record zones for each card in this row.
				xOffset := 0
				for j, idx := range rowIndices {
					cw := lipgloss.Width(row[j])
					m.cardZones = append(m.cardZones, zone{
						idx: idx, x: xOffset, y: currentY, w: cw, h: rowH,
					})
					xOffset += cw
				}

				body.WriteString(joined)
				body.WriteString("\n")
				currentY += rowH + 1
				row = nil
				rowIndices = nil
			}
		}
	}

	if m.mode != modeCreate && m.statusMsg != "" {
		body.WriteString("\n" + m.statusMsg + "\n")
	}

	return body.String()
}

func (m Model) handleClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || len(m.worktrees) == 0 {
		return m, nil
	}

	x, y := msg.X, msg.Y

	// Convert screen coordinates to body content coordinates.
	headerLines := 2
	scrollOffset := 0
	if m.ready {
		scrollOffset = m.viewport.YOffset
	}
	contentY := y - headerLines + scrollOffset
	contentX := x

	// Hit test against recorded zones.
	for _, z := range m.cardZones {
		if contentX >= z.x && contentX < z.x+z.w &&
			contentY >= z.y && contentY < z.y+z.h {
			m.cursor = z.idx
			m.statusMsg = ""
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	if len(m.worktrees) == 0 && m.err == nil {
		return titleStyle.Render("gwaim") + "\n\nLoading worktrees...\n"
	}

	title := titleStyle.Render("gwaim - Git Worktree Agent Manager")
	header := m.renderHeader(title)

	mouseLabel := "m mouse:off"
	if m.mouseOn {
		mouseLabel = "m mouse:on"
	}
	help := helpStyle.Render("←→↑↓ navigate • ↵ open tab • e editor • p pull • r repair • c create • f fetch PR • d delete • " + mouseLabel + " • q quit")

	if m.ready {
		return header + "\n" + m.viewport.View() + "\n" + help
	}

	return header + "\n" + m.renderBody() + "\n" + help
}

// renderHeader builds the two-line header: title, then last-update timestamps below it.
func (m Model) renderHeader(title string) string {
	localTime := "—"
	if !m.lastLocalRefresh.IsZero() {
		localTime = m.lastLocalRefresh.Format("15:04:05")
	}
	if m.localFlash {
		localTime = "✓"
	}
	netTime := "—"
	if !m.lastNetworkRefresh.IsZero() {
		netTime = m.lastNetworkRefresh.Format("15:04:05")
	}
	if m.netFlash {
		netTime = "✓"
	}
	status := refreshTimestampStyle.Render(
		fmt.Sprintf("Last Update: local: %s (%s)   net: %s (%s)",
			localTime, LocalRefreshInterval, netTime, m.refreshInterval),
	)
	return title + "\n" + status
}

func (m Model) columns() int {
	if m.width == 0 {
		return 2
	}
	cw := 44 // card width + border + padding
	cols := m.width / cw
	if cols < 1 {
		cols = 1
	}
	return cols
}

func (m Model) cardWidth(cols int) int {
	if m.width == 0 {
		return 40
	}
	// Account for borders (2 per card) and gaps.
	available := m.width - (cols * 4)
	w := available / cols
	if w < 30 {
		w = 30
	}
	return w
}

// Commands

// doQuickRefresh lists worktrees with branch info only — no dirty/sync checks,
// no fetch, no agent detection, no PR lookup. Used for instant first render.
func doQuickRefresh(repo *git.Repository) tea.Cmd {
	return func() tea.Msg {
		wts, err := repo.ListWorktreesQuick()
		if err != nil {
			return refreshMsg{err: err}
		}
		return refreshMsg{source: refreshSourceQuick, worktrees: wts, agents: agent.DetectionResult{}, prs: github.PRResult{}}
	}
}

// doLocalRefresh reads dirty status and detects agents — no network I/O.
// Fires every LocalRefreshInterval and after any state-modifying operation.
func doLocalRefresh(repo *git.Repository, detector *agent.Detector) tea.Cmd {
	return func() tea.Msg {
		wts, err := repo.ListWorktrees()
		if err != nil {
			return refreshMsg{err: err}
		}
		paths := make([]string, len(wts))
		for i, wt := range wts {
			paths[i] = wt.Path
		}
		agents := detector.Detect(paths)
		return refreshMsg{source: refreshSourceLocal, worktrees: wts, agents: agents}
	}
}

// doNetworkRefresh fetches remote refs and looks up PR status.
// Fires every refreshInterval (controlled by --refresh / GWAIM_REFRESH).
func doNetworkRefresh(repo *git.Repository, detector *agent.Detector, ghAvail github.GHAvailability) tea.Cmd {
	return func() tea.Msg {
		fetchErr := repo.Fetch()

		wts, err := repo.ListWorktrees()
		if err != nil {
			return refreshMsg{err: err}
		}

		paths := make([]string, len(wts))
		branches := make([]string, len(wts))
		for i, wt := range wts {
			paths[i] = wt.Path
			branches[i] = wt.Branch
		}

		agents := detector.Detect(paths)
		var prs github.PRResult
		if ghAvail == github.GHAvailable {
			prs = github.FetchPRs(repo.Root(), branches)
		} else {
			prs = make(github.PRResult)
		}
		return refreshMsg{source: refreshSourceNetwork, worktrees: wts, agents: agents, prs: prs, hasPRs: true, fetchErr: fetchErr}
	}
}

func doCheckGH() tea.Cmd {
	return func() tea.Msg {
		return ghCheckMsg{avail: github.CheckGH()}
	}
}

func (m Model) doTick() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) doLocalTick() tea.Cmd {
	return tea.Tick(LocalRefreshInterval, func(_ time.Time) tea.Msg {
		return localTickMsg{}
	})
}

func doPull(repo *git.Repository) tea.Cmd {
	return func() tea.Msg {
		err := repo.Pull()
		return pullMsg{err: err}
	}
}

func doCreateWorktree(repo *git.Repository, branchName string) tea.Cmd {
	return func() tea.Msg {
		err := repo.CreateWorktree(branchName)
		return worktreeCreatedMsg{branchName: branchName, err: err}
	}
}

func doFetchPR(repo *git.Repository, input string) tea.Cmd {
	return func() tea.Msg {
		ref, err := github.ParsePRRef(input)
		if err != nil {
			return prFetchedMsg{err: err}
		}

		// Validate the PR exists via gh CLI.
		headBranch, err := github.ValidatePR(repo.Root(), ref)
		if err != nil {
			return prFetchedMsg{err: err}
		}

		// Use the PR's head branch name as the local branch.
		branchName := headBranch

		// Determine remote URL for fork PRs.
		remoteURL := ""
		if ref.Repo != "" {
			remoteURL = "https://github.com/" + ref.Repo + ".git"
		}

		wtPath, err := repo.FetchPR(ref.Number, branchName, remoteURL)
		return prFetchedMsg{branchName: branchName, wtPath: wtPath, err: err}
	}
}

func doRepairWorktrees(repo *git.Repository) tea.Cmd {
	return func() tea.Msg {
		output, err := repo.Repair()
		return worktreeRepairedMsg{output: output, err: err}
	}
}

func doRemoveWorktree(repo *git.Repository, name string) tea.Cmd {
	return func() tea.Msg {
		err := repo.RemoveWorktree(name)
		return worktreeRemovedMsg{err: err}
	}
}

func doOpenEditor(dir string) tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("GWAIM_EDITOR")
		if editor == "" {
			editor = "code"
		}
		cmd := exec.Command(editor, dir)
		err := cmd.Start()
		return editorOpenedMsg{err: err}
	}
}

func doOpenWarpPanel(repoName string, wt git.Worktree, agents []agent.Info) tea.Cmd {
	return func() tea.Msg {
		agentCmd := ""
		if len(agents) > 0 {
			agentCmd = string(agents[0].Kind)
		}
		err := warp.OpenTab(repoName, wt.Path, agentCmd)
		return warpOpenedMsg{err: err}
	}
}
