package gui

import (
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// repoEntry holds per-repo runtime state.
type repoEntry struct {
	group      *RepoGroup
	repo       *git.Repository
	prProv     provider.PRProvider
	state      *RepoState
	dashboard  *Dashboard
	refreshMgr *RefreshManager
}

// App is the top-level Fyne application.
type App struct {
	fyneApp fyne.App
	window  fyne.Window

	theme       *biomeTheme
	repoPanel   *RepoPanel
	repos       []*repoEntry
	active      int // active repo entry index
	dashSlot    *fyne.Container
	dashboard   *Dashboard
	refreshMgr  *RefreshManager
	sbxStatuses map[string]sandbox.Status

	configPath      string
	detector        *agent.Detector
	ideDetector     *ide.Detector
	procLister      process.Lister
	refreshInterval time.Duration

	focus        focusPanel
	dialogOpen   bool
	activeDialog interface{ Hide() } // current dialog, for Escape dismissal
	trayMenu     *fyne.Menu
}

// NewApp creates a new biomelab Fyne application.
func NewApp(
	configPath string,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	procLister process.Lister,
	refreshInterval time.Duration,
) *App {
	return &App{
		configPath:      configPath,
		detector:        detector,
		ideDetector:     ideDetector,
		procLister:      procLister,
		refreshInterval: refreshInterval,
		sbxStatuses:     make(map[string]sandbox.Status),
	}
}

// Run initializes the GUI and starts the event loop. Blocks until the window is closed.
func (a *App) Run() {
	a.fyneApp = fyneapp.NewWithID("com.mdelapenya.biomelab")
	a.theme = newBiomeTheme()
	a.fyneApp.Settings().SetTheme(a.theme)

	a.window = a.fyneApp.NewWindow("biomelab")
	a.window.SetIcon(AppIcon)
	a.window.Resize(fyne.NewSize(1200, 700))

	content := a.buildContent()
	a.window.SetContent(content)

	// Register keyboard handlers on the canvas. This works because no child
	// widget implements Focusable — repo panel uses tappable labels (not
	// widget.Tree) and cards use tappableCard (Tappable only).
	setupKeyHandlers(a.window.Canvas(), a.handleKeyName, a.handleRune)

	// Ctrl+/Ctrl- zoom (uses AddShortcut which works with modifiers).
	registerZoomShortcuts(a.window.Canvas(), a.theme, a.fyneApp, func() {
		if a.repoPanel != nil {
			a.repoPanel.RebuildFull()
		}
		if a.dashboard != nil {
			a.dashboard.Rebuild()
		}
	})

	// System tray: closing the window hides to tray instead of quitting.
	// SetCloseIntercept is set inside setupSystemTray.
	a.setupSystemTray()

	a.window.ShowAndRun()
}

func (a *App) buildContent() fyne.CanvasObject {
	cfg, err := config.Load(a.configPath)
	if err != nil || len(cfg.Repos) == 0 {
		return a.emptyState()
	}

	// Check sandbox statuses once up front.
	a.sbxStatuses = make(map[string]sandbox.Status)
	if sandbox.Available() {
		statusMap := sandbox.CheckAllStatuses()
		for k, v := range statusMap {
			a.sbxStatuses[k] = v
		}
	}

	// Build repo groups and entries.
	var groups []*RepoGroup
	for _, entry := range cfg.Repos {
		repo, err := git.OpenRepository(entry.Path)
		if err != nil {
			continue
		}

		group := &RepoGroup{
			Path:  entry.Path,
			Name:  entry.Name,
			Modes: entry.Modes,
		}
		groups = append(groups, group)

		prov := provider.DetectProvider(repo.OriginURL())
		prProv := provider.NewProvider(repo.OriginURL())

		worktrees, _ := repo.ListWorktrees()

		state := &RepoState{
			Provider: prov,
		}
		state.SetWorktrees(worktrees)

		var sbxName string
		if len(entry.Modes) > 0 {
			mode := entry.Modes[0]
			state.ActiveMode = &mode
			if mode.Type == "sandbox" {
				sbxName = mode.SandboxName
				if s, ok := a.sbxStatuses[sbxName]; ok {
					state.SandboxStatus = s
				}
			}
		}

		dash := NewDashboard(state)
		dash.OnCardSelected = func(_ int) {
			a.focus = focusRight // clicking a card means right panel has focus
		}

		rm := NewRefreshManager(repo, a.detector, a.ideDetector, a.procLister, prProv, a.refreshInterval)
		rm.SetSandboxName(sbxName)

		re := &repoEntry{
			group:      group,
			repo:       repo,
			prProv:     prProv,
			state:      state,
			dashboard:  dash,
			refreshMgr: rm,
		}

		// Wire refresh callback.
		rm.OnRefresh = func(result ops.RefreshResult) {
			fyne.Do(func() {
				re.dashboard.ApplyRefresh(result)
				if result.AllSbxStatuses != nil {
					for k, v := range result.AllSbxStatuses {
						a.sbxStatuses[k] = v
					}
					if a.repoPanel != nil {
						a.repoPanel.UpdateStatuses(a.sbxStatuses)
					}
				}
			})
		}

		a.repos = append(a.repos, re)
	}

	if len(a.repos) == 0 {
		return a.emptyState()
	}

	// Build repo panel (left side).
	a.repoPanel = NewRepoPanel(groups, a.sbxStatuses)
	a.repoPanel.OnModeSelected = func(gi, mi int) {
		a.focus = focusLeft // clicking the tree means left panel has focus
		a.switchMode(gi, mi)
	}

	// Active dashboard (right side).
	a.active = 0
	a.dashboard = a.repos[0].dashboard
	a.refreshMgr = a.repos[0].refreshMgr
	a.dashSlot = container.NewStack(a.dashboard.Content())

	// Start the active repo's refresh manager.
	a.repos[0].refreshMgr.Start()

	// Title bar.
	title := monoText("biomelab", colorSelected, true)
	title.TextSize = scaledSize(11)
	title.Alignment = fyne.TextAlignCenter
	titleBg := canvas.NewRectangle(colorPanelBg)
	titleBar := container.NewStack(titleBg, container.NewPadded(title))

	// Two-panel layout.
	split := container.NewHSplit(a.repoPanel.Content(), a.dashSlot)
	split.Offset = 0.18

	return container.NewBorder(titleBar, nil, nil, nil, split)
}

func (a *App) switchMode(groupIdx, modeIdx int) {
	if groupIdx < 0 || groupIdx >= len(a.repos) {
		return
	}

	// Pause the old active repo's refresh.
	if a.active >= 0 && a.active < len(a.repos) {
		a.repos[a.active].refreshMgr.Pause()
	}

	a.active = groupIdx
	re := a.repos[groupIdx]

	if modeIdx >= 0 && modeIdx < len(re.group.Modes) {
		mode := re.group.Modes[modeIdx]
		re.state.ActiveMode = &mode
		re.group.ActiveMode = modeIdx

		sbxName := ""
		if mode.Type == "sandbox" {
			sbxName = mode.SandboxName
			if s, ok := a.sbxStatuses[sbxName]; ok {
				re.state.SandboxStatus = s
			}
		}
		re.refreshMgr.SetSandboxName(sbxName)
	}

	a.dashboard = re.dashboard
	a.refreshMgr = re.refreshMgr
	a.dashboard.Rebuild()
	a.dashSlot.Objects = []fyne.CanvasObject{a.dashboard.Content()}
	a.dashSlot.Refresh()

	if a.repoPanel != nil {
		a.repoPanel.SetActive(groupIdx, modeIdx)
	}

	re.refreshMgr.Resume()
}

func (a *App) emptyState() fyne.CanvasObject {
	msg := widget.NewLabel("No repositories registered.\nRun biomelab from a git repository to auto-add it.")
	msg.Alignment = fyne.TextAlignCenter
	return container.NewCenter(msg)
}

