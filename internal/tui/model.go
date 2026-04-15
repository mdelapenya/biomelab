package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/tui/card"
	"github.com/mdelapenya/biomelab/internal/warp"
)

// DefaultNetworkRefreshInterval is the default interval for network operations
// (git fetch + PR lookups). Controlled by --refresh / BIOME_REFRESH.
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
	modeConfirmCreateSandbox
	modeConfirmRemoveSandbox
	modeEnrollSandboxFromCard // non-sandbox repo: prompt for agent to enroll
)

// Model is the top-level bubbletea model for biomelab.
type Model struct {
	repo         *git.Repository
	detector     *agent.Detector
	ideDetector  *ide.Detector
	procLister   process.Lister
	worktrees    []git.Worktree
	agents       agent.DetectionResult
	ides         ide.DetectionResult
	prs          provider.PRResult
	cliAvail  provider.CLIAvailability
	prProv    provider.PRProvider
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
	embedded          bool      // true when used inside App (no header in viewport calc)
	paused            bool      // true when repo is not the active tab; ticks stop self-scheduling
	fixedTopHeight  int    // lines occupied by the fixed top section (main card + input)
	fixedTopContent string // cached render of fixed top (set in syncViewport, read in viewContent)
	activeMode     *config.ModeEntry    // current mode (nil means regular)
	allWorktrees   []git.Worktree       // unfiltered worktree list from git
	sandboxStatus  sandbox.Status              // active mode's sandbox status
	sandboxVersion sandbox.VersionInfo         // sbx client/server versions
	modeStatuses   map[string]sandbox.Status   // all sandbox name → status (for tree dots)
}

type zone struct {
	idx            int
	x, y, w, h    int
}

// New creates a new TUI model. refreshInterval controls how often network
// operations (fetch + PR lookups) run. Pass 0 to use DefaultNetworkRefreshInterval.
// Local state (dirty status, agents, IDEs) always refreshes every LocalRefreshInterval.
func New(repo *git.Repository, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, refreshInterval time.Duration) Model {
	ti := textinput.New()
	ti.Placeholder = "branch-name"
	ti.CharLimit = 80
	ti.Width = 30

	if refreshInterval <= 0 {
		refreshInterval = DefaultNetworkRefreshInterval
	}

	// Detect the hosting provider from the origin URL.
	var prProv provider.PRProvider
	if repo != nil {
		prProv = provider.NewProvider(repo.OriginURL())
	} else {
		prProv = &provider.GitHubProvider{} // default for tests
	}

	return Model{
		repo:            repo,
		detector:        detector,
		ideDetector:     ideDetector,
		procLister:      procLister,
		keys:            defaultKeyMap(),
		textInput:       ti,
		mouseOn:         true,
		refreshInterval: refreshInterval,
		prProv:          prProv,
		agents:          make(agent.DetectionResult),
		ides:            make(ide.DetectionResult),
		prs:             make(provider.PRResult),
		modeStatuses:    make(map[string]sandbox.Status),
	}
}

// newEmbedded creates a Model that renders without its own header.
// Used by App, which renders the header above both columns.
func newEmbedded(repo *git.Repository, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, refreshInterval time.Duration) Model {
	m := New(repo, detector, ideDetector, procLister, refreshInterval)
	m.embedded = true
	return m
}

// Pause stops the model's refresh tick chains. Pending ticks will fire
// but won't re-schedule, so fetches stop after the current cycle.
func (m Model) Pause() Model {
	m.paused = true
	return m
}

// Resume restarts the model's refresh tick chains and triggers an
// immediate refresh. Call this when the repo becomes the active tab.
func (m Model) Resume() (Model, tea.Cmd) {
	m.paused = false
	rp := m.repoPath()
	return m, tea.Batch(
		doQuickRefresh(m.repo, rp),
		doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()),
		m.doLocalTick(),
		m.doTick(),
	)
}

// IsNormal returns true if the model is in normal (non-modal) mode.
func (m Model) IsNormal() bool {
	return m.mode == modeNormal
}

// RepoPath returns the root path of the repository this model manages.
// Returns an empty string if no repository is set (e.g., in tests).
func (m Model) RepoPath() string {
	if m.repo == nil {
		return ""
	}
	return m.repo.Root()
}

// isStale returns true if a message's repoPath does not match this model's repo.
// Used to discard async messages from a previously-active repo.
func (m Model) isStale(repoPath string) bool {
	return m.repo != nil && repoPath != "" && repoPath != m.repo.Root()
}

// repoPath returns the repo root or empty string if repo is nil.
func (m Model) repoPath() string {
	if m.repo != nil {
		return m.repo.Root()
	}
	return ""
}

func (m Model) repoName() string {
	if m.repo != nil {
		return m.repo.RepoName()
	}
	return ""
}

// isSandbox returns true if the current mode is sandbox.
func (m Model) isSandbox() bool {
	return m.activeMode != nil && m.activeMode.Type == "sandbox"
}

// sbxName returns the sandbox name from the active mode, or empty string.
func (m Model) sbxName() string {
	if m.activeMode != nil {
		return m.activeMode.SandboxName
	}
	return ""
}

// sbxAgent returns the agent name from the active mode, or empty string.
func (m Model) sbxAgent() string {
	if m.activeMode != nil {
		return m.activeMode.Agent
	}
	return ""
}

// ModeStatus returns the sandbox status for a given sandbox name.
// Returns StatusNotFound if the name is not in the map.
func (m Model) ModeStatus(sbxName string) sandbox.Status {
	if s, ok := m.modeStatuses[sbxName]; ok {
		return s
	}
	return sandbox.StatusNotFound
}

// SetActiveMode updates the active mode, re-filters worktrees, and fires
// sandbox check if needed. Returns the updated model and a cmd.
func (m Model) SetActiveMode(mode *config.ModeEntry) (Model, tea.Cmd) {
	m.activeMode = mode
	m.filterWorktrees()
	// Use cached status from modeStatuses if available, so the tree dot
	// doesn't flash red on every mode switch.
	if mode != nil && mode.Type == "sandbox" {
		m.sandboxStatus = m.ModeStatus(mode.SandboxName)
	} else {
		m.sandboxStatus = sandbox.StatusNotFound
	}
	m.sandboxVersion = sandbox.VersionInfo{}
	// Clear stale status messages from the previous mode.
	m.statusMsg = ""
	if m.isSandbox() {
		rp := m.repoPath()
		return m, doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName())
	}
	return m, nil
}

// filterWorktrees filters allWorktrees into worktrees based on activeMode.
// Main worktree is always included.
// Regular mode (or nil): include all worktrees NOT in a .sbx/ directory.
// Sandbox mode: include only paths containing ".sbx/<sandboxName>-worktrees/".
func (m *Model) filterWorktrees() {
	if len(m.allWorktrees) == 0 {
		m.worktrees = nil
		return
	}
	if !m.isSandbox() {
		// Regular mode: include everything that isn't in a sandbox directory.
		var filtered []git.Worktree
		for _, wt := range m.allWorktrees {
			if wt.IsMain || !strings.Contains(wt.Path, ".sbx/") {
				filtered = append(filtered, wt)
			}
		}
		m.worktrees = filtered
		return
	}
	// Sandbox mode: only include main + matching sandbox worktrees.
	needle := ".sbx/" + m.sbxName() + "-worktrees/"
	var filtered []git.Worktree
	for _, wt := range m.allWorktrees {
		if wt.IsMain || strings.Contains(wt.Path, needle) {
			filtered = append(filtered, wt)
		}
	}
	m.worktrees = filtered
}

func (m Model) Init() tea.Cmd {
	// Fast initial load: branch names only, no dirty/agents/network.
	// Local refresh (dirty + agents) and network refresh (fetch + PRs) follow immediately.
	// CLI pre-flight runs once at startup.
	rp := m.repoPath()
	return tea.Batch(
		doCheckCLI(m.prProv, rp),
		doQuickRefresh(m.repo, rp),
		doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()),
		doNetworkRefresh(m.repo, m.detector, m.ideDetector, m.procLister, m.prProv, m.cliAvail, rp, m.sbxName()),
		m.doLocalTick(),
		m.doTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(m.width, 1) // provisional; syncViewport adjusts height
			m.ready = true
		} else {
			m.viewport.Width = m.width
		}
		m.syncViewport()
		return m, nil

	case sandboxCreatedFromCardMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Sandbox create failed: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Sandbox created: " + msg.sandboxName)
		}
		return m, doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName())

	case sandboxStartedMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Start failed: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Sandbox started: " + msg.sandboxName)
		}
		return m, doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName())

	case sandboxStoppedCmdMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Stop failed: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Sandbox stopped: " + msg.sandboxName)
		}
		return m, doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName())

	case sandboxRemovedMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Sandbox remove failed: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Sandbox removed: " + msg.sandboxName)
		}
		return m, doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName())

	case cliCheckMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		m.cliAvail = msg.avail
		return m, nil

	case tickMsg:
		if m.isStale(msg.repoPath) || m.paused {
			return m, nil // don't re-schedule: breaks the tick chain
		}
		rp := m.repoPath()
		return m, tea.Batch(doNetworkRefresh(m.repo, m.detector, m.ideDetector, m.procLister, m.prProv, m.cliAvail, rp, m.sbxName()), m.doTick())

	case localTickMsg:
		if m.isStale(msg.repoPath) || m.paused {
			return m, nil // don't re-schedule: breaks the tick chain
		}
		rp := m.repoPath()
		return m, tea.Batch(
			doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()),
			m.doLocalTick(),
		)

	case refreshMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.allWorktrees = msg.worktrees
			m.filterWorktrees()
			m.agents = msg.agents
			m.ides = msg.ides
			if msg.hasPRs {
				m.prs = msg.prs
			}
			m.err = nil
		}
		if msg.fetchErr != nil {
			m.statusMsg = errorStyle.Render("Fetch: " + msg.fetchErr.Error())
		}
		// Only update sandbox status when the refresh actually checked it.
		// Quick refreshes don't check sandbox status (allSbxStatuses is nil).
		if msg.allSbxStatuses != nil {
			for k, v := range msg.allSbxStatuses {
				m.modeStatuses[k] = sandbox.Status(v)
			}
			if m.sbxName() != "" {
				if msg.sbxClientVer != "" {
					m.sandboxVersion = sandbox.VersionInfo{Client: msg.sbxClientVer, Server: msg.sbxServerVer}
				}
				prev := m.sandboxStatus
				m.sandboxStatus = sandbox.Status(msg.sandboxStatus)
				switch m.sandboxStatus {
				case sandbox.StatusNotFound:
					m.statusMsg = errorStyle.Render("Sandbox not found — press 'n' to create it")
				case sandbox.StatusStopped:
					m.statusMsg = sandboxStoppedStyle.Render("Sandbox stopped — run: sbx run " + m.sbxName())
				case sandbox.StatusRunning:
					if prev != sandbox.StatusRunning {
						m.statusMsg = ""
					}
				}
			}
		}
		if m.cursor >= len(m.worktrees) && len(m.worktrees) > 0 {
			m.cursor = len(m.worktrees) - 1
		}
		// Update timestamps and trigger ✓ flash for periodic refreshes.
		var flashCmd tea.Cmd
		rp := msg.repoPath
		switch msg.source {
		case refreshSourceLocal:
			m.lastLocalRefresh = time.Now()
			m.localFlash = true
			flashCmd = tea.Tick(time.Second, func(_ time.Time) tea.Msg { return localFlashDoneMsg{repoPath: rp} })
		case refreshSourceNetwork:
			m.lastNetworkRefresh = time.Now()
			m.netFlash = true
			flashCmd = tea.Tick(time.Second, func(_ time.Time) tea.Msg { return netFlashDoneMsg{repoPath: rp} })
		}
		m.syncViewport()
		return m, flashCmd

	case localFlashDoneMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		m.localFlash = false
		m.syncViewport()
		return m, nil

	case netFlashDoneMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		m.netFlash = false
		m.syncViewport()
		return m, nil

	case worktreeCreatedMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
			m.mode = modeNormal
			return m, tea.Batch(doQuickRefresh(m.repo, rp), doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()))
		}
		m.mode = modeNormal
		m.statusMsg = cleanStyle.Render("Worktree created")
		return m, tea.Batch(
			doQuickRefresh(m.repo, rp),
			doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()),
		)

	case prFetchedMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
			m.mode = modeNormal
			return m, tea.Batch(doQuickRefresh(m.repo, rp), doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()))
		}
		m.mode = modeNormal
		m.statusMsg = cleanStyle.Render("PR fetched")
		return m, tea.Batch(
			doQuickRefresh(m.repo, rp),
			doNetworkRefresh(m.repo, m.detector, m.ideDetector, m.procLister, m.prProv, m.cliAvail, rp, m.sbxName()),
		)

	case worktreeRemovedMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Worktree removed")
		}
		m.mode = modeNormal
		return m, tea.Batch(doQuickRefresh(m.repo, rp), doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()))

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
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		rp := m.repoPath()
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Pull: " + msg.err.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Pull complete")
		}
		// Pull changes local state only; sync status will update on next network tick.
		return m, tea.Batch(doQuickRefresh(m.repo, rp), doLocalRefresh(m.repo, m.detector, m.ideDetector, m.procLister, rp, m.sbxName()))

	case cardRefreshMsg:
		if m.isStale(msg.repoPath) {
			return m, nil
		}
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Refresh: " + msg.err.Error())
			return m, nil
		}
		if msg.fetchErr != nil {
			m.statusMsg = errorStyle.Render("Fetch: " + msg.fetchErr.Error())
		} else {
			m.statusMsg = cleanStyle.Render("Refreshed")
		}
		// Update worktree list (needed for dirty/sync status).
		m.allWorktrees = msg.worktrees
		m.filterWorktrees()
		// Merge per-card agent/IDE/PR data into existing maps.
		for k, v := range msg.agents {
			m.agents[k] = v
		}
		for k, v := range msg.ides {
			m.ides[k] = v
		}
		if msg.hasPRs {
			for k, v := range msg.prs {
				m.prs[k] = v
			}
		}
		if m.cursor >= len(m.worktrees) && len(m.worktrees) > 0 {
			m.cursor = len(m.worktrees) - 1
		}
		m.syncViewport()
		return m, nil

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

	if m.mode == modeCreate || m.mode == modeFetchPR || m.mode == modeEnrollSandboxFromCard {
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
			if m.isSandbox() {
				// Sandbox mode: create worktree inside existing sandbox.
				m.statusMsg = cleanStyle.Render("Creating sandbox worktree...")
				return m, doCreateSandboxWorktree(m.sbxName(), m.repoPath(), name)
			}
			return m, doCreateWorktree(m.repo, name, m.repoPath())
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
			if m.isSandbox() {
				return m, doFetchPRSandbox(m.repo, input, m.repoPath(), m.sbxName())
			}
			return m, doFetchPR(m.repo, input, m.repoPath())
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
				return m, doRemoveWorktree(m.repo, wt.Branch, m.repoPath())
			}
			m.mode = modeNormal
			m.deleteConfirmed = false
			m.statusMsg = ""
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.deleteConfirmed = true
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

	// Handle enroll sandbox from card (non-sandbox repo).
	if m.mode == modeEnrollSandboxFromCard {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			agentName := strings.TrimSpace(m.textInput.Value())
			if agentName == "" {
				m.mode = modeNormal
				return m, nil
			}
			m.mode = modeNormal
			m.statusMsg = cleanStyle.Render("Checking sbx readiness...")
			sbxName := sandbox.SanitizeName(m.repoName(), agentName)
			return m, func() tea.Msg {
				return enrollSandboxRequestMsg{
					repoPath: m.repoPath(),
					mode: config.ModeEntry{
						Type:        "sandbox",
						SandboxName: sbxName,
						Agent:       agentName,
					},
				}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	// Handle confirm create sandbox mode.
	if m.mode == modeConfirmCreateSandbox {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.mode = modeNormal
			m.statusMsg = cleanStyle.Render("Creating sandbox (this may take a few minutes)...")
			sbxArgs := sandbox.CreateArgs(m.sbxName(), m.sbxAgent(), m.repoPath())
			return m, doCreateSandboxFromCard(sbxArgs, m.sbxName(), m.repoPath())
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		default:
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		}
	}

	// Handle confirm remove sandbox mode.
	if m.mode == modeConfirmRemoveSandbox {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.mode = modeNormal
			m.statusMsg = cleanStyle.Render("Removing sandbox...")
			return m, doRemoveSandbox(m.sbxName(), m.repoPath())
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		default:
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
		m.ensureCursorVisible()

	case key.Matches(msg, m.keys.Right):
		if m.cursor >= 1 && m.cursor < len(m.worktrees)-1 {
			m.cursor++
		}
		m.statusMsg = ""
		m.ensureCursorVisible()

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
		m.ensureCursorVisible()

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
		m.ensureCursorVisible()

	case key.Matches(msg, m.keys.Pull):
		m.statusMsg = cleanStyle.Render("Pulling...")
		return m, doPull(m.repo, m.repoPath())

	case key.Matches(msg, m.keys.Refresh):
		if m.cursor >= len(m.worktrees) {
			return m, nil
		}
		wt := m.worktrees[m.cursor]
		m.statusMsg = cleanStyle.Render("Refreshing card state...")
		return m, doCardRefresh(m.repo, m.detector, m.ideDetector, m.procLister, m.prProv, m.cliAvail, m.repoPath(), wt.Path, wt.Branch)

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
		if m.cliAvail != provider.CLIAvailable {
			m.statusMsg = errorStyle.Render("CLI tool required for PR fetch")
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
			if m.isSandbox() {
				// Sandbox mode: sbx run --branch knows which sandbox to enter, no cd needed.
				args := sandbox.RunWithBranchArgs(m.sbxName(), wt.Branch)
				sbxCmd := sandbox.CommandString(args)
				return m, doOpenSandboxTab(m.repoName(), sbxCmd)
			}
			agents := m.agents[wt.Path]
			return m, doOpenWarpPanel(m.repoName(), wt, agents)
		}

	case key.Matches(msg, m.keys.Delete):
		if m.cursor >= 0 && m.cursor < len(m.worktrees) {
			wt := m.worktrees[m.cursor]
			if wt.IsMain {
				if m.isSandbox() && m.sandboxStatus != sandbox.StatusNotFound {
					// Sandbox mode: offer to remove the sandbox.
					m.mode = modeConfirmRemoveSandbox
					return m, nil
				}
				m.statusMsg = errorStyle.Render("Cannot delete the main worktree")
				return m, nil
			}
			m.mode = modeConfirmDelete
			m.deleteConfirmed = false
		}

	case key.Matches(msg, m.keys.CreateSandbox):
		if m.cursor == 0 {
			if m.isSandbox() {
				if m.sandboxStatus == sandbox.StatusNotFound {
					m.mode = modeConfirmCreateSandbox
					return m, nil
				}
				m.statusMsg = cleanStyle.Render("Sandbox already exists — use 'c' to create a worktree")
				return m, nil
			}
			// Non-sandbox repo: offer to enroll.
			m.mode = modeEnrollSandboxFromCard
			m.textInput.Reset()
			m.textInput.Placeholder = "agent (claude, codex, copilot, gemini, kiro, opencode, shell)"
			m.textInput.Focus()
			m.statusMsg = ""
			return m, m.textInput.Cursor.BlinkCmd()
		}

	case key.Matches(msg, m.keys.StartSandbox):
		if m.cursor == 0 && m.isSandbox() && m.sandboxStatus == sandbox.StatusStopped {
			m.statusMsg = cleanStyle.Render("Starting sandbox...")
			return m, doStartSandbox(m.sbxName(), m.repoPath())
		}

	case key.Matches(msg, m.keys.StopSandbox):
		if m.cursor == 0 && m.isSandbox() && m.sandboxStatus == sandbox.StatusRunning {
			m.statusMsg = cleanStyle.Render("Stopping sandbox...")
			return m, doStopSandbox(m.sbxName(), m.repoPath())
		}
	}

	return m, nil
}

// syncViewport updates the viewport content from the current model state.
// It renders both fixed-top and linked-cards sections, caching the fixed-top
// output and feeding only linked cards into the viewport.
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	m.cardZones = nil
	m.fixedTopContent = m.renderFixedTop()
	m.viewport.SetContent(m.renderLinkedCards())

	// Dynamically size viewport: total height minus fixed top minus footer.
	footerH := 3 // \n + helpStyle MarginTop(1) + help text
	vpH := m.height - m.fixedTopHeight - footerH
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.Height = vpH
}

// renderFixedTop renders the non-scrolling top section: error, main worktree
// card, and create/fetchPR text input. Sets m.fixedTopHeight as a side effect.
// Records the main card zone in m.cardZones with y relative to 0.
func (m *Model) renderFixedTop() string {
	var body strings.Builder
	currentY := 0

	// Refresh timestamps (shown inside the right panel when embedded).
	if m.embedded {
		ts := m.renderTimestamps()
		body.WriteString(ts + "\n")
		currentY += strings.Count(ts, "\n") + 1
	}

	if m.err != nil {
		line := errorStyle.Render("Error: "+m.err.Error()) + "\n"
		body.WriteString(line)
		currentY += strings.Count(line, "\n")
	}

	prov := m.providerType()

	if len(m.worktrees) > 0 {
		mainWt := m.worktrees[0]
		agents := m.agents[mainWt.Path]
		mainIDEs := m.ides[mainWt.Path]
		var sbxInfo *card.SandboxInfo
		if m.isSandbox() {
			sbxInfo = &card.SandboxInfo{
				Name:          m.sbxName(),
				Status:        card.SandboxStatus(m.sandboxStatus),
				Agent:         m.sbxAgent(),
				ClientVersion: m.sandboxVersion.Client,
				ServerVersion: m.sandboxVersion.Server,
			}
		}
		content := card.Render(mainWt, agents, mainIDEs, m.prs[mainWt.Branch], m.cliAvail, prov, sbxInfo)

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

		// Main card contextual help line.
		mainHelp := m.renderMainCardHelp()
		body.WriteString(mainHelp)
		body.WriteString("\n")
		currentY++

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
		if m.mode == modeEnrollSandboxFromCard {
			body.WriteString(inputPromptStyle.Render("  Sandbox agent: "))
			body.WriteString(m.textInput.View())
			body.WriteString("\n")
			currentY += 2
		}
	}

	m.fixedTopHeight = currentY
	return body.String()
}

// renderLinkedCards renders the scrollable linked worktree card grid.
// This content is placed inside the viewport. Zone y-coordinates are
// relative to 0 (viewport-local).
func (m *Model) renderLinkedCards() string {
	var body strings.Builder
	currentY := 0

	cols := m.columns()
	if cols < 1 {
		cols = 1
	}
	cardWidth := m.cardWidth(cols)
	prov := m.providerType()

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
			wtIDEs := m.ides[wt.Path]
			var linkedSbx *card.SandboxInfo
			if m.isSandbox() {
				linkedSbx = &card.SandboxInfo{Name: m.sbxName(), Status: card.SandboxStatus(m.sandboxStatus), Agent: m.sbxAgent()}
			}
			content := card.Render(wt, agents, wtIDEs, m.prs[wt.Branch], m.cliAvail, prov, linkedSbx)

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

	// When embedded, App already adjusts Y to be panel-content-relative.
	// When standalone, subtract the 2-line header.
	headerLines := 2
	if m.embedded {
		headerLines = 0
	}
	screenContentY := y - headerLines

	// Zone 1: Fixed top area (main card, text input).
	if screenContentY < m.fixedTopHeight {
		for _, z := range m.cardZones {
			if z.idx != 0 {
				continue
			}
			if x >= z.x && x < z.x+z.w &&
				screenContentY >= z.y && screenContentY < z.y+z.h {
				m.cursor = 0
				m.statusMsg = ""
				return m, nil
			}
		}
		return m, nil
	}

	// Zone 2: Scrollable linked cards (viewport).
	viewportScreenY := screenContentY - m.fixedTopHeight
	contentY := viewportScreenY
	if m.ready {
		contentY += m.viewport.YOffset
	}
	for _, z := range m.cardZones {
		if z.idx == 0 {
			continue
		}
		if x >= z.x && x < z.x+z.w &&
			contentY >= z.y && contentY < z.y+z.h {
			m.cursor = z.idx
			m.statusMsg = ""
			return m, nil
		}
	}

	return m, nil
}

// ensureCursorVisible scrolls the viewport so the selected linked card is visible.
func (m *Model) ensureCursorVisible() {
	if m.cursor == 0 || !m.ready {
		return
	}
	for _, z := range m.cardZones {
		if z.idx == m.cursor {
			if z.y < m.viewport.YOffset {
				m.viewport.SetYOffset(z.y)
			} else if z.y+z.h > m.viewport.YOffset+m.viewport.Height {
				m.viewport.SetYOffset(z.y + z.h - m.viewport.Height)
			}
			break
		}
	}
}

func (m Model) View() string {
	if len(m.worktrees) == 0 && m.err == nil {
		return titleStyle.Render("biomelab") + "\n\nLoading worktrees...\n"
	}

	title := titleStyle.Render("biomelab - Git Worktree Agent Manager")
	header := m.renderHeader(title)

	return header + "\n" + m.viewContent()
}

// ViewContent returns the body + help bar without the header.
// Used by App to render the Model inside the right panel while the
// header is displayed above both columns.
func (m Model) ViewContent() string {
	if len(m.worktrees) == 0 && m.err == nil {
		return "Loading worktrees...\n"
	}
	return m.viewContent()
}

func (m Model) viewContent() string {
	mouseLabel := "m mouse:off"
	if m.mouseOn {
		mouseLabel = "m mouse:on"
	}
	helpText := "←→↑↓ navigate • ↵ open tab • e editor • r refresh • d delete • p pull • " + mouseLabel + " • q quit"
	help := helpStyle.MaxWidth(m.width).Render(helpText)

	if m.ready {
		return m.fixedTopContent + m.viewport.View() + "\n" + help
	}

	return m.fixedTopContent + m.renderLinkedCards() + "\n" + help
}

// renderMainCardHelp returns a styled help line for main-card-specific actions.
// The content adapts to sandbox status; the style highlights when cursor is on the main card.
func (m Model) renderMainCardHelp() string {
	parts := []string{"c create", "f fetch PR"}

	if m.isSandbox() {
		switch m.sandboxStatus {
		case sandbox.StatusRunning:
			parts = append(parts, "S stop", "d rm sandbox")
		case sandbox.StatusStopped:
			parts = append(parts, "s start", "d rm sandbox")
		case sandbox.StatusNotFound:
			parts = append(parts, "n create sandbox")
		}
	} else {
		parts = append(parts, "n new sandbox")
	}

	text := strings.Join(parts, " • ")

	style := mainCardHelpFaintStyle
	if m.cursor == 0 {
		style = mainCardHelpActiveStyle
	}
	return style.Render(text)
}

// renderConfirmCreateSandboxPopup renders a confirmation popup for sandbox creation.
func (m Model) renderConfirmCreateSandboxPopup() string {
	cmd := strings.Join(sandbox.CreateArgs(m.sbxName(), m.sbxAgent(), m.repoPath()), " ")
	msg := fmt.Sprintf("Create sandbox %q?\n\nThis may take a few minutes.\n\n%s\n\n[y] confirm  [Esc] cancel", m.sbxName(), cmd)
	return popupStyle.Render(msg)
}

// renderConfirmRemoveSandboxPopup renders a confirmation popup for sandbox removal.
func (m Model) renderConfirmRemoveSandboxPopup() string {
	msg := fmt.Sprintf("Remove sandbox %q?\n\nThis will stop the sandbox, remove all\nits containers and worktrees.\n\nsbx rm --force %s\n\n[y] confirm  [Esc] cancel", m.sbxName(), m.sbxName())
	return popupStyle.Render(msg)
}

// renderConfirmPopup renders a centered confirmation popup for worktree deletion.
func (m Model) renderConfirmPopup() string {
	if m.cursor < 0 || m.cursor >= len(m.worktrees) {
		return ""
	}
	wt := m.worktrees[m.cursor]

	var msg string
	if m.deleteConfirmed {
		msg = fmt.Sprintf("Delete worktree %q?\n\nPress Enter to confirm, Esc to cancel.", wt.Branch)
	} else {
		msg = fmt.Sprintf("Delete worktree %q?\n\nThis removes the directory, branch,\nand prunes metadata.\n\n[y] confirm  [Esc] cancel", wt.Branch)
	}

	return popupStyle.Render(msg)
}

// overlayCenter renders a popup centered on a full-screen scrim that
// replaces the base content. True transparency is not possible in terminal
// emulators, so the scrim is a solid dark background that visually separates
// the popup from the underlying UI.
func overlayCenter(_, popup string, width, height int) string {
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceBackground(lipgloss.Color("233")),
	)
}


// ScrollState returns the viewport's scroll state for external scrollbar rendering.
// Returns totalLines, visibleLines, yOffset.
func (m Model) ScrollState() (int, int, int) {
	if !m.ready {
		return 0, 0, 0
	}
	return m.viewport.TotalLineCount(), m.viewport.Height, m.viewport.YOffset
}

// RenderHeader builds the header line with title and last-update timestamps.
// Public so App can render it above the two-column layout.
func (m Model) RenderHeader(title string) string {
	return m.renderHeader(title)
}

// renderTimestamps returns the last-update timestamp line.
func (m Model) renderTimestamps() string {
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
	return refreshTimestampStyle.Render(
		fmt.Sprintf("Last Update: local: %s (%s)   net: %s (%s)",
			localTime, LocalRefreshInterval, netTime, m.refreshInterval),
	)
}

// renderHeader builds the two-line header: title, then last-update timestamps below it.
func (m Model) renderHeader(title string) string {
	return title + "\n" + m.renderTimestamps()
}

// providerType returns the provider type from the PRProvider.
func (m Model) providerType() provider.Provider {
	if m.prProv != nil {
		return m.prProv.Provider()
	}
	return provider.ProviderGitHub
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
func doQuickRefresh(repo *git.Repository, repoPath string) tea.Cmd {
	return func() tea.Msg {
		wts, err := repo.ListWorktreesQuick()
		if err != nil {
			return refreshMsg{repoPath: repoPath, err: err}
		}
		return refreshMsg{repoPath: repoPath, source: refreshSourceQuick, worktrees: wts, agents: agent.DetectionResult{}, prs: provider.PRResult{}}
	}
}

// doLocalRefresh reads dirty status and detects agents and IDEs — no network I/O.
// Fires every LocalRefreshInterval and after any state-modifying operation.
func doLocalRefresh(repo *git.Repository, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, repoPath string, sbxName string) tea.Cmd {
	return func() tea.Msg {
		wts, err := repo.ListWorktrees()
		if err != nil {
			return refreshMsg{repoPath: repoPath, err: err}
		}
		paths := make([]string, len(wts))
		for i, wt := range wts {
			paths[i] = wt.Path
		}

		// Fetch processes once and share across both detectors.
		ctx := context.Background()
		procs, procErr := procLister.Processes(ctx)
		var agents agent.DetectionResult
		var ides ide.DetectionResult
		if procErr == nil {
			agents = detector.DetectFromProcesses(procs, paths)
			ides = ideDetector.DetectFromProcesses(procs, paths)
		}

		// Check all sandbox statuses and version with one sbx ls call.
		var sbxStatus sandbox.Status
		var sbxVer sandbox.VersionInfo
		var allStatuses map[string]int
		if sbxName != "" {
			statusMap := sandbox.CheckAllStatuses()
			if statusMap != nil {
				allStatuses = make(map[string]int, len(statusMap))
				for k, v := range statusMap {
					allStatuses[k] = int(v)
				}
				if s, ok := statusMap[sbxName]; ok {
					sbxStatus = s
				}
			}
			sbxVer = sandbox.Version()
		}

		return refreshMsg{repoPath: repoPath, source: refreshSourceLocal, worktrees: wts, agents: agents, ides: ides, sandboxStatus: int(sbxStatus), allSbxStatuses: allStatuses, sbxClientVer: sbxVer.Client, sbxServerVer: sbxVer.Server}
	}
}

// doNetworkRefresh fetches remote refs and looks up PR status.
// Fires every refreshInterval (controlled by --refresh / BIOME_REFRESH).
func doNetworkRefresh(repo *git.Repository, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, prProv provider.PRProvider, cliAvail provider.CLIAvailability, repoPath string, sbxName string) tea.Cmd {
	return func() tea.Msg {
		fetchErr := repo.Fetch()

		wts, err := repo.ListWorktrees()
		if err != nil {
			return refreshMsg{repoPath: repoPath, err: err}
		}

		paths := make([]string, len(wts))
		branches := make([]string, len(wts))
		for i, wt := range wts {
			paths[i] = wt.Path
			branches[i] = wt.Branch
		}

		// Fetch processes once and share across both detectors.
		ctx := context.Background()
		procs, procErr := procLister.Processes(ctx)
		var agents agent.DetectionResult
		var ides ide.DetectionResult
		if procErr == nil {
			agents = detector.DetectFromProcesses(procs, paths)
			ides = ideDetector.DetectFromProcesses(procs, paths)
		}

		var prs provider.PRResult
		if cliAvail == provider.CLIAvailable {
			prs = prProv.FetchPRs(repo.Root(), branches)
		} else {
			prs = make(provider.PRResult)
		}

		// Check sandbox status if configured.
		var sbxStatus sandbox.Status
		if sbxName != "" {
			sbxStatus = sandbox.CheckStatus(sbxName)
		}

		return refreshMsg{repoPath: repoPath, source: refreshSourceNetwork, worktrees: wts, agents: agents, ides: ides, prs: prs, hasPRs: true, fetchErr: fetchErr, sandboxStatus: int(sbxStatus)}
	}
}

// doCardRefresh refreshes a single worktree card: fetches remotes for sync
// status, detects agents/IDEs only for the target path, and looks up PRs
// only for the target branch.
func doCardRefresh(repo *git.Repository, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, prProv provider.PRProvider, cliAvail provider.CLIAvailability, repoPath, wtPath, branch string) tea.Cmd {
	return func() tea.Msg {
		fetchErr := repo.Fetch()

		wts, err := repo.ListWorktrees()
		if err != nil {
			return cardRefreshMsg{repoPath: repoPath, wtPath: wtPath, err: err}
		}

		// Detect agents/IDEs only for the target worktree.
		ctx := context.Background()
		procs, procErr := procLister.Processes(ctx)
		var agents agent.DetectionResult
		var ides ide.DetectionResult
		if procErr == nil {
			agents = detector.DetectFromProcesses(procs, []string{wtPath})
			ides = ideDetector.DetectFromProcesses(procs, []string{wtPath})
		}

		// Look up PR only for the target branch.
		var prs provider.PRResult
		if cliAvail == provider.CLIAvailable {
			prs = prProv.FetchPRs(repo.Root(), []string{branch})
		} else {
			prs = make(provider.PRResult)
		}

		return cardRefreshMsg{
			repoPath:  repoPath,
			wtPath:    wtPath,
			worktrees: wts,
			agents:    agents,
			ides:      ides,
			prs:       prs,
			hasPRs:    true,
			fetchErr:  fetchErr,
		}
	}
}

func doCheckCLI(prProv provider.PRProvider, repoPath string) tea.Cmd {
	return func() tea.Msg {
		return cliCheckMsg{repoPath: repoPath, avail: prProv.CheckCLI()}
	}
}

func (m Model) doTick() tea.Cmd {
	rp := m.repoPath()
	return tea.Tick(m.refreshInterval, func(_ time.Time) tea.Msg {
		return tickMsg{repoPath: rp}
	})
}

func (m Model) doLocalTick() tea.Cmd {
	rp := m.repoPath()
	return tea.Tick(LocalRefreshInterval, func(_ time.Time) tea.Msg {
		return localTickMsg{repoPath: rp}
	})
}

func doPull(repo *git.Repository, repoPath string) tea.Cmd {
	return func() tea.Msg {
		err := repo.Pull()
		return pullMsg{repoPath: repoPath, err: err}
	}
}

func doCreateWorktree(repo *git.Repository, branchName, repoPath string) tea.Cmd {
	return func() tea.Msg {
		err := repo.CreateWorktree(branchName)
		return worktreeCreatedMsg{repoPath: repoPath, branchName: branchName, err: err}
	}
}

func doFetchPR(repo *git.Repository, input, repoPath string) tea.Cmd {
	return func() tea.Msg {
		ref, err := github.ParsePRRef(input)
		if err != nil {
			return prFetchedMsg{repoPath: repoPath, err: err}
		}

		// Validate the PR exists via gh CLI.
		headBranch, err := github.ValidatePR(repo.Root(), ref)
		if err != nil {
			return prFetchedMsg{repoPath: repoPath, err: err}
		}

		// Use the PR's head branch name as the local branch.
		branchName := headBranch

		// Determine remote URL for fork PRs.
		remoteURL := ""
		if ref.Repo != "" {
			remoteURL = "https://github.com/" + ref.Repo + ".git"
		}

		wtPath, err := repo.FetchPR(ref.Number, branchName, remoteURL)
		return prFetchedMsg{repoPath: repoPath, branchName: branchName, wtPath: wtPath, err: err}
	}
}

func doRemoveWorktree(repo *git.Repository, name, repoPath string) tea.Cmd {
	return func() tea.Msg {
		err := repo.RemoveWorktree(name)
		return worktreeRemovedMsg{repoPath: repoPath, err: err}
	}
}

func doOpenEditor(dir string) tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("BIOME_EDITOR")
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

// doCreateSandboxWorktree creates a worktree inside an existing sandbox
// using sbx run -d --branch (detached). No terminal tab is opened —
// the user attaches later by pressing Enter on the card.
func doCreateSandboxWorktree(sandboxName, repoPath, branch string) tea.Cmd {
	return func() tea.Msg {
		// Create the worktree in detached mode.
		args := sandbox.RunDetachedWithBranchArgs(sandboxName, branch)
		out, err := sandbox.RunDetached(args)
		if err != nil {
			return worktreeCreatedMsg{
				repoPath:   repoPath,
				branchName: branch,
				err:        err,
				sbxOutput:  out,
			}
		}

		return worktreeCreatedMsg{
			repoPath:   repoPath,
			branchName: branch,
			sbxOutput:  out,
		}
	}
}

// doOpenSandboxTab opens a terminal tab running an sbx command (no cd prefix).
func doOpenSandboxTab(repoName, sbxCmd string) tea.Cmd {
	return func() tea.Msg {
		err := warp.OpenTab(repoName, "", sbxCmd)
		return warpOpenedMsg{err: err}
	}
}

// doFetchPRSandbox fetches a PR ref and creates the worktree inside the existing sandbox.
// No terminal tab is opened — the user attaches later by pressing Enter on the card.
func doFetchPRSandbox(repo *git.Repository, input, repoPath, sandboxName string) tea.Cmd {
	return func() tea.Msg {
		ref, err := github.ParsePRRef(input)
		if err != nil {
			return prFetchedMsg{repoPath: repoPath, err: err}
		}

		headBranch, err := github.ValidatePR(repo.Root(), ref)
		if err != nil {
			return prFetchedMsg{repoPath: repoPath, err: err}
		}

		branchName := headBranch
		remoteURL := ""
		if ref.Repo != "" {
			remoteURL = "https://github.com/" + ref.Repo + ".git"
		}

		// Fetch the PR ref to a local branch (no worktree creation on host).
		if err := repo.FetchPRRef(ref.Number, branchName, remoteURL); err != nil {
			return prFetchedMsg{repoPath: repoPath, err: err}
		}

		// Create the worktree inside the sandbox (detached).
		args := sandbox.RunDetachedWithBranchArgs(sandboxName, branchName)
		out, err := sandbox.RunDetached(args)
		if err != nil {
			return prFetchedMsg{repoPath: repoPath, err: fmt.Errorf("sbx worktree: %w: %s", err, out)}
		}
		_ = out

		return prFetchedMsg{repoPath: repoPath, branchName: branchName}
	}
}

// doCreateSandboxFromCard runs sbx create from the main card (after confirmation).
func doCreateSandboxFromCard(args []string, sandboxName, repoPath string) tea.Cmd {
	return func() tea.Msg {
		out, err := sandbox.Create(args)
		return sandboxCreatedFromCardMsg{repoPath: repoPath, sandboxName: sandboxName, output: out, err: err}
	}
}

// doRemoveSandbox runs sbx rm to remove a sandbox.
func doRemoveSandbox(sandboxName, repoPath string) tea.Cmd {
	return func() tea.Msg {
		out, err := sandbox.Remove(sandboxName)
		return sandboxRemovedMsg{repoPath: repoPath, sandboxName: sandboxName, output: out, err: err}
	}
}

// doStartSandbox runs sbx run -d to start a stopped sandbox.
func doStartSandbox(sandboxName, repoPath string) tea.Cmd {
	return func() tea.Msg {
		_, err := sandbox.Start(sandboxName)
		return sandboxStartedMsg{repoPath: repoPath, sandboxName: sandboxName, err: err}
	}
}

// doStopSandbox runs sbx stop to stop a sandbox without removing it.
func doStopSandbox(sandboxName, repoPath string) tea.Cmd {
	return func() tea.Msg {
		_, err := sandbox.Stop(sandboxName)
		return sandboxStoppedCmdMsg{repoPath: repoPath, sandboxName: sandboxName, err: err}
	}
}
