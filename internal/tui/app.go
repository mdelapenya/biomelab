package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

type focusPanel int

const (
	focusLeft  focusPanel = iota // repo list
	focusRight                   // worktree dashboard
)

type appMode int

const (
	appModeNormal          appMode = iota
	appModeAddRepo                        // text input for repo path
	appModeConfirmRemove                  // confirmation prompt for repo removal
	appModeSelectRepoMode                 // choose regular vs sandbox after path validation
	appModeEnrollAgent                    // text input for sandbox agent name (add-repo flow)
	appModeAddSandboxMode                 // text input for agent to add sandbox mode to existing repo
)

// repoGroup holds the state for a single registered repository with its modes.
type repoGroup struct {
	path       string             // repo root path (unique key)
	name       string             // display name
	modes      []config.ModeEntry // all modes for this repo
	activeMode int                // index into modes
	model      Model              // per-repo worktree dashboard
}

// panelScroll holds scroll state for a bordered panel's scrollbar.
type panelScroll struct {
	total, visible, offset int
}

// App is the top-level bubbletea model that manages multiple repositories.
type App struct {
	repos            []*repoGroup
	active           int        // selected repo index
	focus            focusPanel // which panel has focus
	detector         *agent.Detector
	ideDetector      *ide.Detector
	procLister       process.Lister
	configPath       string
	width            int
	height           int
	mode             appMode
	textInput        textinput.Model
	statusMsg        string
	refreshInterval  time.Duration
	repoScrollOffset    int // first visible repo card index
	pendingRepo         *repoValidatedMsg // holds validated repo during mode/agent selection
	pendingAgent        string            // agent name during sandbox enrollment preflight
	pendingEnrollPath   string            // repo path for from-card sandbox enrollment
	pendingEnrollAgent  string            // agent for from-card sandbox enrollment
	sbxStatuses         map[string]sandbox.Status // system-wide sandbox statuses for tree dots
}

// addRepoMsg is returned after validating and opening a new repo.
type addRepoMsg struct {
	path string
	name string
	repo *git.Repository
	err  error
	mode config.ModeEntry
}

// NewApp creates a new App model.
func NewApp(configPath string, detector *agent.Detector, ideDetector *ide.Detector, procLister process.Lister, refreshInterval time.Duration) App {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repository"
	ti.CharLimit = 256
	ti.Width = 50

	if refreshInterval <= 0 {
		refreshInterval = DefaultNetworkRefreshInterval
	}

	return App{
		detector:        detector,
		ideDetector:     ideDetector,
		procLister:      procLister,
		configPath:      configPath,
		textInput:       ti,
		refreshInterval: refreshInterval,
		sbxStatuses:     make(map[string]sandbox.Status),
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
		m := newEmbedded(repo, a.detector, a.ideDetector, a.procLister, a.refreshInterval)
		modes := entry.Modes
		if len(modes) == 0 {
			modes = []config.ModeEntry{{Type: "regular"}}
		}
		m.activeMode = &modes[0]
		if i != 0 {
			// Non-active repos start paused — no tick chains.
			m.paused = true
		}
		a.repos = append(a.repos, &repoGroup{
			path:  entry.Path,
			name:  entry.Name,
			modes: modes,
			model: m,
		})
		if i == 0 {
			// Only the active repo starts its full refresh cycle.
			cmds = append(cmds, m.Init())
		}
	}

	// Seed sandbox statuses so tree dots are correct from the start.
	if statusMap := sandbox.CheckAllStatuses(); statusMap != nil {
		for k, v := range statusMap {
			a.sbxStatuses[k] = v
		}
	}

	// Return the modified app as a cmd that sets up repos.
	// Since Init() can't mutate the receiver, we use a message.
	return func() tea.Msg {
		return appInitMsg{repos: a.repos, sbxStatuses: a.sbxStatuses, cmds: cmds}
	}
}

// appInitMsg carries the repos loaded during Init.
type appInitMsg struct {
	repos       []*repoGroup
	cmds        []tea.Cmd
	sbxStatuses map[string]sandbox.Status
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
		if msg.sbxStatuses != nil {
			a.sbxStatuses = msg.sbxStatuses
		}
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

	case repoValidatedMsg:
		return a.handleRepoValidatedMsg(msg)

	case sandboxCreatedMsg:
		if msg.err != nil {
			a.statusMsg = errorStyle.Render("Sandbox: " + msg.err.Error())
		} else {
			a.statusMsg = cleanStyle.Render("Sandbox created: " + msg.repoName)
		}
		return a, nil

	case enrollSandboxRequestMsg:
		// Non-sandbox repo wants to enroll — run preflight.
		a.pendingEnrollPath = msg.repoPath
		a.pendingEnrollAgent = msg.mode.Agent
		return a, doSandboxPreflight()

	case sandboxPreflightMsg:
		if msg.err != nil {
			a.statusMsg = errorStyle.Render(msg.err.Error())
			// Clear both enrollment flows.
			if a.pendingEnrollPath != "" {
				a.pendingEnrollPath = ""
				a.pendingEnrollAgent = ""
				// Update child status.
				for i, rt := range a.repos {
					if rt.path == a.pendingEnrollPath {
						a.repos[i].model.statusMsg = errorStyle.Render(msg.err.Error())
						break
					}
				}
			} else {
				a.mode = appModeNormal
				a.pendingRepo = nil
				a.pendingAgent = ""
			}
			return a, nil
		}

		// sbx is ready. Check which enrollment flow we're in.
		if a.pendingEnrollPath != "" {
			// From-card enrollment: update existing repo to sandbox mode.
			return a.finalizeCardEnrollment()
		}

		// From add-repo enrollment.
		pending := a.pendingRepo
		agentName := a.pendingAgent
		a.pendingRepo = nil
		a.pendingAgent = ""
		a.mode = appModeNormal
		a.statusMsg = cleanStyle.Render("Adding sandbox repository...")
		sbxName := sandbox.SanitizeName(pending.name, agentName)
		mode := config.ModeEntry{Type: "sandbox", SandboxName: sbxName, Agent: agentName}
		return a, doFinalizeAddRepo(pending, mode, a.configPath)

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

				// Sync sandbox statuses from child to App level (system-wide data).
				for k, v := range a.repos[i].model.modeStatuses {
					a.sbxStatuses[k] = v
				}

				// After sandbox removal succeeds, remove the mode from the group.
				if rm, ok := msg.(sandboxRemovedMsg); ok && rm.err == nil {
					a.removeSandboxModeFromGroup(i, rm.sandboxName)
				}

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
			a.statusMsg = cleanStyle.Render("Validating repository...")
			return a, doValidateRepo(input)
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

	// Handle repo mode selection (regular vs sandbox).
	if a.mode == appModeSelectRepoMode {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			// Regular mode — finalize immediately.
			pending := a.pendingRepo
			a.pendingRepo = nil
			a.mode = appModeNormal
			a.statusMsg = cleanStyle.Render("Adding repository...")
			return a, doFinalizeAddRepo(pending, config.ModeEntry{Type: "regular"}, a.configPath)
		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			// Sandbox mode — prompt for agent name.
			a.mode = appModeEnrollAgent
			a.textInput.Reset()
			a.textInput.Placeholder = "agent (claude, codex, copilot, gemini, kiro, opencode, shell)"
			a.textInput.Focus()
			a.statusMsg = ""
			return a, a.textInput.Cursor.BlinkCmd()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			a.mode = appModeNormal
			a.pendingRepo = nil
			a.statusMsg = ""
			return a, nil
		}
		return a, nil
	}

	// Handle sandbox agent name input.
	if a.mode == appModeEnrollAgent {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			agentName := strings.TrimSpace(a.textInput.Value())
			if agentName == "" {
				a.mode = appModeNormal
				a.pendingRepo = nil
				a.statusMsg = ""
				return a, nil
			}
			// Store the agent while we run the preflight check.
			a.pendingAgent = agentName
			a.statusMsg = cleanStyle.Render("Checking sbx readiness...")
			return a, doSandboxPreflight()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			a.mode = appModeNormal
			a.pendingRepo = nil
			a.statusMsg = ""
			return a, nil
		default:
			var cmd tea.Cmd
			a.textInput, cmd = a.textInput.Update(msg)
			return a, cmd
		}
	}

	// Handle add sandbox mode to existing repo.
	if a.mode == appModeAddSandboxMode {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			agentName := strings.TrimSpace(a.textInput.Value())
			if agentName == "" {
				a.mode = appModeNormal
				a.statusMsg = ""
				return a, nil
			}
			// Store for preflight flow.
			if a.active < len(a.repos) {
				rg := a.repos[a.active]
				a.pendingEnrollPath = rg.path
				a.pendingEnrollAgent = agentName
				a.mode = appModeNormal
				a.statusMsg = cleanStyle.Render("Checking sbx readiness...")
				return a, doSandboxPreflight()
			}
			a.mode = appModeNormal
			return a, nil
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
				rg := a.repos[a.active]
				activeMode := rg.modes[rg.activeMode]

				if len(rg.modes) > 1 {
					// Multiple modes: remove just the active mode.
					cfg, err := config.Load(a.configPath)
					if err == nil {
						cfg.RemoveMode(rg.path, activeMode)
						_ = config.Save(a.configPath, cfg)
					}
					// Update in-memory modes.
					rg.modes = append(rg.modes[:rg.activeMode], rg.modes[rg.activeMode+1:]...)
					if rg.activeMode >= len(rg.modes) {
						rg.activeMode = len(rg.modes) - 1
					}
					updated, cmd := rg.model.SetActiveMode(&rg.modes[rg.activeMode])
					rg.model = updated
					a.statusMsg = cleanStyle.Render("Mode removed")
					a.mode = appModeNormal
					return a, cmd
				}

				if activeMode.Type == "sandbox" {
					// Last mode is sandbox: convert to regular instead of removing.
					regularMode := config.ModeEntry{Type: "regular"}
					cfg, err := config.Load(a.configPath)
					if err == nil {
						cfg.RemoveMode(rg.path, activeMode)
						cfg.Add(rg.path, rg.name, regularMode)
						_ = config.Save(a.configPath, cfg)
					}
					rg.modes = []config.ModeEntry{regularMode}
					rg.activeMode = 0
					updated, cmd := rg.model.SetActiveMode(&rg.modes[0])
					rg.model = updated
					a.statusMsg = cleanStyle.Render("Sandbox removed — repo converted to host mode")
					a.mode = appModeNormal
					return a, cmd
				}

				// Last mode is regular: remove the entire repo.
				cfg, err := config.Load(a.configPath)
				if err == nil {
					cfg.Remove(rg.path)
					_ = config.Save(a.configPath, cfg)
				}
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
		// Click in left panel — focus it and find which mode was clicked.
		a.focus = focusLeft
		// Tree content starts at: header height + 1 (panel border) + 1 ("Repos" label).
		contentY := msg.Y - hh - 2
		if contentY >= 0 {
			// Iterate through groups to find which line was clicked.
			lineIdx := 0
			for gi, rg := range a.repos {
				// Clicking the group header selects its first mode.
				if lineIdx == contentY {
					if gi != a.active || rg.activeMode != 0 {
						return a.switchMode(gi, 0)
					}
					return a, nil
				}
				lineIdx++ // group header line
				for mi := range rg.modes {
					if lineIdx == contentY {
						if gi != a.active || mi != rg.activeMode {
							return a.switchMode(gi, mi)
						}
						return a, nil
					}
					lineIdx++
				}
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
		return a.moveModeUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		return a.moveModeDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		a.mode = appModeAddRepo
		a.textInput.Reset()
		a.textInput.Placeholder = "/path/to/repository"
		a.textInput.Focus()
		a.statusMsg = ""
		return a, a.textInput.Cursor.BlinkCmd()
	case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
		if len(a.repos) > 0 && a.active < len(a.repos) {
			a.mode = appModeAddSandboxMode
			a.textInput.Reset()
			a.textInput.Placeholder = "agent (claude, codex, copilot, gemini, kiro, opencode, shell)"
			a.textInput.Focus()
			a.statusMsg = ""
			return a, a.textInput.Cursor.BlinkCmd()
		}
		return a, nil
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

// removeSandboxModeFromGroup removes a sandbox mode by name from a group.
// If it was the last sandbox, converts to regular. Updates config.
func (a App) removeSandboxModeFromGroup(groupIdx int, sbxName string) {
	rg := a.repos[groupIdx]

	// Find and remove the mode.
	var remaining []config.ModeEntry
	for _, m := range rg.modes {
		if m.Type == "sandbox" && m.SandboxName == sbxName {
			continue
		}
		remaining = append(remaining, m)
	}

	if len(remaining) == len(rg.modes) {
		return // mode not found, nothing to do
	}

	// If no modes left, convert to regular.
	if len(remaining) == 0 {
		remaining = []config.ModeEntry{{Type: "regular"}}
	}

	// Update config.
	cfg, err := config.Load(a.configPath)
	if err == nil {
		cfg.RemoveMode(rg.path, config.ModeEntry{Type: "sandbox", SandboxName: sbxName})
		if len(remaining) == 1 && remaining[0].Type == "regular" {
			cfg.Add(rg.path, rg.name, remaining[0])
		}
		_ = config.Save(a.configPath, cfg)
	}

	// Update in-memory group.
	rg.modes = remaining
	if rg.activeMode >= len(rg.modes) {
		rg.activeMode = len(rg.modes) - 1
	}
	updated, _ := rg.model.SetActiveMode(&rg.modes[rg.activeMode])
	rg.model = updated
}

// finalizeCardEnrollment converts an existing regular repo to sandbox mode.
func (a App) finalizeCardEnrollment() (tea.Model, tea.Cmd) {
	repoPath := a.pendingEnrollPath
	agentName := a.pendingEnrollAgent
	a.pendingEnrollPath = ""
	a.pendingEnrollAgent = ""

	// Find the repo and update it.
	for i, rt := range a.repos {
		if rt.path == repoPath {
			sbxName := sandbox.SanitizeName(rt.name, agentName)
			newMode := config.ModeEntry{Type: "sandbox", SandboxName: sbxName, Agent: agentName}

			// Update config: add sandbox mode (replaces regular if present).
			cfg, _ := config.Load(a.configPath)
			cfg.Add(repoPath, rt.name, newMode)
			_ = config.Save(a.configPath, cfg)

			// Reload modes from config (cfg.Add handles dedup/replace logic).
			idx := cfg.IndexOf(repoPath)
			if idx >= 0 {
				a.repos[i].modes = cfg.Repos[idx].Modes
			}
			// Select the newly added mode.
			newModeIdx := len(a.repos[i].modes) - 1
			a.repos[i].activeMode = newModeIdx
			a.repos[i].model.activeMode = &a.repos[i].modes[newModeIdx]
			a.repos[i].model.statusMsg = cleanStyle.Render("Creating sandbox (this may take a few minutes)...")

			// Create the sandbox.
			sbxArgs := sandbox.CreateArgs(sbxName, agentName, repoPath)
			return a, doCreateSandbox(sbxArgs, rt.name)
		}
	}
	return a, nil
}

func (a App) handleRepoValidatedMsg(msg repoValidatedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
		a.mode = appModeNormal
		return a, nil
	}
	a.pendingRepo = &msg
	a.mode = appModeSelectRepoMode
	a.statusMsg = ""
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
	m := newEmbedded(msg.repo, a.detector, a.ideDetector, a.procLister, a.refreshInterval)
	modes := []config.ModeEntry{msg.mode}
	m.activeMode = &modes[0]
	rg := &repoGroup{
		path:  msg.path,
		name:  msg.name,
		modes: modes,
		model: m,
	}
	a.repos = append(a.repos, rg)
	a.active = len(a.repos) - 1
	a.focus = focusRight
	a.statusMsg = cleanStyle.Render("Repository added: " + msg.name)

	var cmds []tea.Cmd
	cmds = append(cmds, rg.model.Init(), a.resizeActiveChild())

	// For sandbox repos, create the sandbox in the background.
	// The preflight check already confirmed sbx is ready (no interactive prompts).
	if msg.mode.Type == "sandbox" && msg.mode.Agent != "" {
		sbxArgs := sandbox.CreateArgs(msg.mode.SandboxName, msg.mode.Agent, msg.path)
		cmds = append(cmds, doCreateSandbox(sbxArgs, msg.name))
	}

	return a, tea.Batch(cmds...)
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
	header := appTitleStyle.Render("biomelab - Git Worktree Agent Manager")

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

	switch a.mode {
	case appModeAddRepo:
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  Repository path: ") + a.textInput.View())
	case appModeSelectRepoMode:
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  Mode: [s]andbox (recommended)  [r]egular  [Esc] cancel"))
	case appModeEnrollAgent, appModeAddSandboxMode:
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  Agent: ") + a.textInput.View())
	default:
		if a.statusMsg != "" {
			b.WriteString("\n")
			b.WriteString(a.statusMsg)
		}
	}

	result := b.String()

	// Overlay modal popups over the full screen.
	if a.mode == appModeConfirmRemove {
		popup := a.renderConfirmRemovePopup()
		result = overlayCenter(result, popup, a.width, a.height)
	} else if a.active >= 0 && a.active < len(a.repos) {
		child := a.repos[a.active].model
		switch child.mode {
		case modeConfirmDelete:
			popup := child.renderConfirmPopup()
			result = overlayCenter(result, popup, a.width, a.height)
		case modeConfirmCreateSandbox:
			popup := child.renderConfirmCreateSandboxPopup()
			result = overlayCenter(result, popup, a.width, a.height)
		case modeConfirmRemoveSandbox:
			popup := child.renderConfirmRemoveSandboxPopup()
			result = overlayCenter(result, popup, a.width, a.height)
		}
	}

	return result
}

// renderConfirmRemovePopup renders a centered confirmation popup for repo removal.
func (a App) renderConfirmRemovePopup() string {
	if a.active < 0 || a.active >= len(a.repos) {
		return ""
	}
	name := a.repos[a.active].name
	msg := fmt.Sprintf("Remove %q from dashboard?\n\n[y]es  [any key] cancel", name)
	return popupStyle.Render(msg)
}

func (a App) renderEmptyState() string {
	title := titleStyle.Render("biomelab - Git Worktree Agent Manager")
	body := "\n\nNo repositories registered.\nPress 'a' to add a repository.\n"

	switch a.mode {
	case appModeAddRepo:
		body += "\n" + inputPromptStyle.Render("  Repository path: ") + a.textInput.View() + "\n"
	case appModeSelectRepoMode:
		body += "\n" + inputPromptStyle.Render("  Mode: [s]andbox (recommended)  [r]egular  [Esc] cancel") + "\n"
	case appModeEnrollAgent, appModeAddSandboxMode:
		body += "\n" + inputPromptStyle.Render("  Agent: ") + a.textInput.View() + "\n"
	}
	if a.statusMsg != "" {
		body += "\n" + a.statusMsg + "\n"
	}

	help := helpStyle.Render("[a]dd repo • [q]uit")
	return title + body + "\n" + help
}

// repoGroupHeight returns the number of lines a group occupies in the tree:
// 1 header line + 1 line per mode.
func repoGroupHeight(rg *repoGroup) int {
	return 1 + len(rg.modes)
}

// totalTreeLines returns the total number of lines all repo groups occupy.
func (a App) totalTreeLines() int {
	total := 0
	for _, rg := range a.repos {
		total += repoGroupHeight(rg)
	}
	return total
}

func (a App) renderRepoList(width, panelHeight int) string {
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}
	maxNameWidth := innerWidth - 4 // margin for marker + emoji
	if maxNameWidth < 5 {
		maxNameWidth = 5
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	repoNameStyle := lipgloss.NewStyle().Faint(true)

	var parts []string
	parts = append(parts, headerStyle.Render("Repos"))

	for gi, rg := range a.repos {
		// Group header: repo name (dimmed, not selectable).
		name := rg.name
		if len(name) > maxNameWidth {
			name = name[:maxNameWidth-1] + "…"
		}
		parts = append(parts, repoNameStyle.Render(name))

		// Mode children: one line each.
		for mi, mode := range rg.modes {
			isSelected := gi == a.active && mi == rg.activeMode
			marker := "  "
			if isSelected {
				marker = "▸ "
			}

			var label string
			if mode.Type == "sandbox" {
				agent := mode.Agent
				if agent == "" {
					agent = "sbx"
				}
				// Status dot: green=running, yellow=stopped, red=not found.
				status := sandbox.StatusNotFound
				if s, ok := a.sbxStatuses[mode.SandboxName]; ok {
					status = s
				}
				var dot string
				switch status {
				case sandbox.StatusRunning:
					dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
				case sandbox.StatusStopped:
					dot = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("●")
				default:
					dot = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("●")
				}
				label = "🐳 [" + agent + "] " + dot
			} else {
				label = "📂 [host]"
			}

			style := unselectedRepoStyle
			if isSelected {
				style = selectedRepoStyle
			}
			parts = append(parts, style.Render(marker+label))
		}
	}

	topContent := strings.Join(parts, "\n")

	help := repoPanelHelpStyle.Render("[a]dd [n]ew sandbox [x]rm")

	content := lipgloss.JoinVertical(lipgloss.Left,
		topContent,
		lipgloss.NewStyle().Height(panelHeight-lipgloss.Height(topContent)-1).Render(""),
		help,
	)

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

// moveModeDown moves to the next mode child in the tree. If at the last mode
// of a group, moves to the first mode of the next group.
func (a App) moveModeDown() (tea.Model, tea.Cmd) {
	if len(a.repos) == 0 {
		return a, nil
	}
	gi := a.active
	mi := a.repos[gi].activeMode

	// Try next mode in current group.
	if mi+1 < len(a.repos[gi].modes) {
		return a.switchMode(gi, mi+1)
	}
	// Try first mode of next group.
	if gi+1 < len(a.repos) {
		return a.switchMode(gi+1, 0)
	}
	return a, nil
}

// moveModeUp moves to the previous mode child in the tree. If at the first mode
// of a group, moves to the last mode of the previous group.
func (a App) moveModeUp() (tea.Model, tea.Cmd) {
	if len(a.repos) == 0 {
		return a, nil
	}
	gi := a.active
	mi := a.repos[gi].activeMode

	// Try previous mode in current group.
	if mi > 0 {
		return a.switchMode(gi, mi-1)
	}
	// Try last mode of previous group.
	if gi > 0 {
		prevGroup := a.repos[gi-1]
		return a.switchMode(gi-1, len(prevGroup.modes)-1)
	}
	return a, nil
}

// switchMode switches the active group and/or mode. If the group changes,
// the old model is paused and the new one resumed.
// seedModelStatuses copies App-level sandbox statuses into a model's modeStatuses
// so SetActiveMode reads correct status without waiting for a refresh.
func (a App) seedModelStatuses(groupIdx int) {
	if groupIdx < 0 || groupIdx >= len(a.repos) {
		return
	}
	for k, v := range a.sbxStatuses {
		a.repos[groupIdx].model.modeStatuses[k] = v
	}
}

func (a App) switchMode(groupIdx, modeIdx int) (tea.Model, tea.Cmd) {
	if groupIdx < 0 || groupIdx >= len(a.repos) {
		return a, nil
	}
	rg := a.repos[groupIdx]
	if modeIdx < 0 || modeIdx >= len(rg.modes) {
		return a, nil
	}

	// Seed the target model's modeStatuses from App-level cache so
	// SetActiveMode reads correct status without waiting for a refresh.
	a.seedModelStatuses(groupIdx)

	if groupIdx == a.active {
		// Same group: just update mode, no pause/resume.
		a.repos[groupIdx].activeMode = modeIdx
		updated, cmd := a.repos[groupIdx].model.SetActiveMode(&a.repos[groupIdx].modes[modeIdx])
		a.repos[groupIdx].model = updated
		a = a.ensureActiveRepoVisible()
		return a, cmd
	}

	// Different group: pause old, resume new.
	if a.active < len(a.repos) {
		a.repos[a.active].model = a.repos[a.active].model.Pause()
	}
	a.active = groupIdx
	a.repos[groupIdx].activeMode = modeIdx
	a = a.ensureActiveRepoVisible()
	resumed, cmd := a.repos[groupIdx].model.Resume()
	a.repos[groupIdx].model = resumed
	updated, modeCmd := a.repos[groupIdx].model.SetActiveMode(&a.repos[groupIdx].modes[modeIdx])
	a.repos[groupIdx].model = updated
	return a, tea.Batch(cmd, modeCmd, a.resizeActiveChild())
}


// visibleRepoLines returns how many content lines fit in the left panel.
func (a App) visibleRepoLines() int {
	contentH := a.height - a.headerHeight() - 2 // -2 for panel border
	if contentH < 1 {
		contentH = 1
	}
	// Subtract 2 for "Repos" header (1) + help footer (1).
	v := contentH - 2
	if v < 1 {
		v = 1
	}
	return v
}

// ensureActiveRepoVisible adjusts repoScrollOffset so the active group is visible.
func (a App) ensureActiveRepoVisible() App {
	// Compute the line offset of the active group's active mode.
	targetLine := 0
	for i, rg := range a.repos {
		if i == a.active {
			targetLine += 1 + rg.activeMode // header + mode offset
			break
		}
		targetLine += repoGroupHeight(rg)
	}
	visible := a.visibleRepoLines()
	if targetLine < a.repoScrollOffset {
		a.repoScrollOffset = targetLine
	}
	if targetLine >= a.repoScrollOffset+visible {
		a.repoScrollOffset = targetLine - visible + 1
	}
	a = a.clampRepoScroll()
	return a
}

// clampRepoScroll ensures repoScrollOffset is within valid bounds.
func (a App) clampRepoScroll() App {
	visible := a.visibleRepoLines()
	maxOffset := a.totalTreeLines() - visible
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
		total:   a.totalTreeLines(),
		visible: a.visibleRepoLines(),
		offset:  a.repoScrollOffset,
	}
}

// headerHeight returns the number of visible rows the header occupies.
func (a App) headerHeight() int {
	return lipgloss.Height(appTitleStyle.Render("biomelab - Git Worktree Agent Manager"))
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
	case cardRefreshMsg:
		return m.repoPath
	case localFlashDoneMsg:
		return m.repoPath
	case netFlashDoneMsg:
		return m.repoPath
	case sandboxCreatedFromCardMsg:
		return m.repoPath
	case sandboxStartedMsg:
		return m.repoPath
	case sandboxStoppedCmdMsg:
		return m.repoPath
	case sandboxRemovedMsg:
		return m.repoPath
	case enrollSandboxRequestMsg:
		return m.repoPath
	case cliCheckMsg:
		return m.repoPath
	}
	return ""
}

// doValidateRepo validates a repo path without saving to config.
// Returns a repoValidatedMsg so the user can choose regular/sandbox mode.
func doValidateRepo(input string) tea.Cmd {
	return func() tea.Msg {
		root, err := git.RepoRoot(input)
		if err != nil {
			return repoValidatedMsg{err: fmt.Errorf("not a git repository: %s", input)}
		}

		repo, err := git.OpenRepository(root)
		if err != nil {
			return repoValidatedMsg{err: fmt.Errorf("cannot open repository: %w", err)}
		}

		return repoValidatedMsg{path: root, name: repo.RepoName(), repo: repo}
	}
}

// doFinalizeAddRepo persists a validated repo to config and returns an addRepoMsg.
func doFinalizeAddRepo(pending *repoValidatedMsg, mode config.ModeEntry, configPath string) tea.Cmd {
	return func() tea.Msg {
		cfg, _ := config.Load(configPath)
		cfg.Add(pending.path, pending.name, mode)
		_ = config.Save(configPath, cfg)

		return addRepoMsg{
			path: pending.path,
			name: pending.name,
			repo: pending.repo,
			mode: mode,
		}
	}
}

// sandboxPreflightMsg carries the result of the sbx readiness check.
type sandboxPreflightMsg struct {
	err error // nil if sbx is ready
}

// doSandboxPreflight checks if sbx is bootstrapped (auth, daemon, policy).
func doSandboxPreflight() tea.Cmd {
	return func() tea.Msg {
		return sandboxPreflightMsg{err: sandbox.Preflight()}
	}
}

// sandboxCreatedMsg is sent after sbx create completes.
type sandboxCreatedMsg struct {
	repoName string
	output   string
	err      error
}

// doCreateSandbox runs sbx create in the background.
func doCreateSandbox(args []string, repoName string) tea.Cmd {
	return func() tea.Msg {
		out, err := sandbox.Create(args)
		return sandboxCreatedMsg{repoName: repoName, output: out, err: err}
	}
}
