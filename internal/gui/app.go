package gui

import (
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/regent"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
	"github.com/mdelapenya/biomelab/internal/terminal"
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

	theme             *biomeTheme
	repoPanel         *RepoPanel
	repos             []*repoEntry
	active            int // active repo entry index
	dashSlot          *fyne.Container
	dashboard         *Dashboard
	refreshMgr        *RefreshManager
	sbxStatuses       map[string]sandbox.Status
	creatingSandboxes map[string]bool // UI-thread owned, keyed by globally unique sandbox name

	// Title bar primitives that capture theme colors at construction time.
	// Held so toggleTheme can refresh them without rebuilding the window.
	titleBg   *canvas.Rectangle
	titleText *canvas.Text

	configPath      string
	detector        *agent.Detector
	ideDetector     *ide.Detector
	termDetector    *terminal.Detector
	procLister      process.Lister
	refreshInterval time.Duration

	focus        focusPanel
	dialogOpen   bool
	activeDialog interface{ Hide() } // current dialog, for Escape dismissal
	// noteWindows tracks the currently-open note editor windows keyed by
	// the worktree path so a repeated 'm' or Review click raises the
	// existing window instead of spawning duplicates.
	noteWindows map[string]fyne.Window
	// regentLogWindow is the single shared regent log window. Pressing
	// 'l' on any card reuses this window (updating title + content via
	// regentLogReload), so the user always has at most one open.
	regentLogWindow fyne.Window
	regentLogReload func(wt git.Worktree)
	trayMenu        *fyne.Menu
	// System tray theme submenu items, held so we can update Checked state.
	trayThemeLight *fyne.MenuItem
	trayThemeDark  *fyne.MenuItem
	// trayDepsItem is the "Dependencies: N/M ✓" line; held so we can
	// rewrite its label after re-probing.
	trayDepsItem *fyne.MenuItem
	// sysdepsCache memoizes Probe results for the dependencies dialog and
	// systray summary. TTL is short enough to feel live; the dialog also
	// exposes an explicit Re-check button that forces a refresh.
	sysdepsCache *sysdeps.Cache

	// issueDeps is per-App so GUI tests can replace network and filesystem
	// operations without process-wide mutable hooks.
	issueDeps *issueDependencies
	issueFlow *issueFlow
}

// NewApp creates a new biomelab Fyne application.
func NewApp(
	configPath string,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	termDetector *terminal.Detector,
	procLister process.Lister,
	refreshInterval time.Duration,
) *App {
	return &App{
		configPath:      configPath,
		detector:        detector,
		ideDetector:     ideDetector,
		termDetector:    termDetector,
		procLister:      procLister,
		refreshInterval: refreshInterval,
		sbxStatuses:     make(map[string]sandbox.Status),
		sysdepsCache:    sysdeps.NewCache(sysdepsCacheTTL),
		issueDeps:       defaultIssueDependencies(),
	}
}

// sysdepsCacheTTL bounds how long the dependencies cache memoizes probe
// results. Short enough that "user just installed rgt" feels live; long
// enough that opening the systray menu twice in a row doesn't re-exec.
const sysdepsCacheTTL = 30 * time.Second

// Run initializes the GUI and starts the event loop. Blocks until the window is closed.
func (a *App) Run() {
	a.fyneApp = fyneapp.NewWithID("com.mdelapenya.biomelab")
	a.theme = newBiomeTheme(a.loadInitialVariant())
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

	// Ctrl+T / Cmd+T toggle between dark and light themes.
	registerThemeToggleShortcut(a.window.Canvas(), a.toggleTheme)

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

	// Check sandbox statuses once up front. When sbx isn't installed, the
	// deps banner + dialog surface that — buildContent doesn't need to
	// emit a separate warning here.
	a.sbxStatuses = make(map[string]sandbox.Status)
	if sandbox.Available() {
		statusMap := sandbox.CheckAllStatuses()
		for k, v := range statusMap {
			a.sbxStatuses[k] = v
		}
	}

	// Reconcile any stored sandbox name that doesn't match what sbx reports
	// but whose alternative candidates do (user created the sandbox manually
	// under a different naming convention). Running this BEFORE building
	// repo entries means the initial repo panel render already shows the
	// correct status dot for those repos — no 5s wait for the first tick.
	if a.reconcileConfigOnLoad(cfg) {
		_ = config.Save(a.configPath, cfg)
	}

	for _, entry := range cfg.Repos {
		if re := a.buildRepoEntry(entry); re != nil {
			a.repos = append(a.repos, re)
		}
	}

	if len(a.repos) == 0 {
		return a.emptyState()
	}

	return a.buildMainLayout()
}

// buildRepoEntry constructs a repoEntry (go-git repo, provider, dashboard,
// refresh manager) for a single config entry. Returns nil if the repo can't
// be opened. The refresh manager is created but not started; the caller
// decides when to Start it (e.g., buildMainLayout starts only the first).
func (a *App) buildRepoEntry(entry config.RepoEntry) *repoEntry {
	repo, err := git.OpenRepository(entry.Path)
	if err != nil {
		return nil
	}

	group := &RepoGroup{
		Path:  entry.Path,
		Name:  entry.Name,
		Modes: entry.Modes,
	}

	prov := provider.DetectProvider(repo.OriginURL())
	prProv := provider.NewProvider(repo.OriginURL())

	worktrees, _ := repo.ListWorktrees()

	state := &RepoState{
		Provider: prov,
	}
	state.SetWorktrees(worktrees)
	group.LinkedWorktreeCount = len(state.LinkedWorktrees())

	var sbxCandidates []string
	if len(entry.Modes) > 0 {
		mode := entry.Modes[0]
		state.ActiveMode = &mode
		if mode.Type == "sandbox" {
			sbxCandidates = sandbox.Candidates(mode.SandboxName, repo.RepoName(), repo.Root(), mode.Agent)
			if _, s, ok := sandbox.MatchStatus(a.sbxStatuses, sbxCandidates); ok {
				state.SandboxStatus = s
			}
		}
	}

	dash := NewDashboard(state)
	dash.OnCardSelected = func(_ int) {
		a.focus = focusRight // clicking a card means right panel has focus
	}
	dash.OnNoteRequested = func(wt git.Worktree) {
		a.focus = focusRight
		a.openNoteDialog(wt)
	}

	rm := NewRefreshManager(repo, a.detector, a.ideDetector, a.termDetector, a.procLister, prProv, a.refreshInterval)
	rm.SetSandboxCandidates(sbxCandidates)

	re := &repoEntry{
		group:      group,
		repo:       repo,
		prProv:     prProv,
		state:      state,
		dashboard:  dash,
		refreshMgr: rm,
	}

	// Migrate existing worktrees to ensure .biomelab dir exists (background task).
	go repo.MigrateWorktreeDirs()

	// Bootstrap re_gent on existing worktrees too, but only when at least
	// one of the repo's configured modes is non-sandbox. Sandbox-only repos
	// install rgt inside the container (via the regent kit), not on the
	// host. EnsureInit is itself a no-op when rgt is missing, so this is
	// safe to schedule unconditionally — the mode gate just keeps us from
	// creating empty .regent/ dirs for sandbox-only repos.
	if hasNonSandboxMode(entry.Modes) {
		go migrateRegentForRepo(repo)
	}

	rm.OnRefresh = func(result ops.RefreshResult) {
		fyne.Do(func() {
			// Reconcile the stored sandbox name FIRST so ApplyRefresh's
			// dashboard rebuild renders the real name in the same tick.
			// Triggered when refresh matched a sandbox under a name that
			// differs from what was stored (e.g. user created it manually
			// via `sbx run`).
			if result.SbxMatchedName != "" && re.state.ActiveMode != nil &&
				re.state.ActiveMode.Type == "sandbox" &&
				result.SbxMatchedName != re.state.ActiveMode.SandboxName {
				a.reconcileSandboxName(re, re.state.ActiveMode.SandboxName, result.SbxMatchedName)
			}
			if !re.dashboard.ApplyRefresh(result) {
				// Snapshot predates a later mutation already applied —
				// skip the worktree-count and sbx-status updates too, both
				// derive from the same stale snapshot.
				return
			}

			// Keep the left-panel worktree count in sync with refresh
			// results so users see "tasks in flight" without opening
			// the dashboard.
			if result.Worktrees != nil {
				newCount := max(len(result.Worktrees)-1, 0)
				if newCount != re.group.LinkedWorktreeCount {
					re.group.LinkedWorktreeCount = newCount
					if a.repoPanel != nil {
						a.repoPanel.rebuildList()
					}
				}
			}

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

	return re
}

// reconcileConfigOnLoad rewrites any sandbox mode in cfg whose stored name
// doesn't match a sandbox in a.sbxStatuses but whose alternative candidates
// do. Pure cfg mutation — does NOT save to disk and does not touch runtime
// state (no repoEntry exists yet at this point). Returns true if anything
// changed so the caller can persist.
func (a *App) reconcileConfigOnLoad(cfg *config.Config) bool {
	if len(a.sbxStatuses) == 0 {
		return false
	}
	changed := false
	for i := range cfg.Repos {
		repoPath := cfg.Repos[i].Path
		var repoName string
		if repo, err := git.OpenRepository(repoPath); err == nil {
			repoName = repo.RepoName()
		}
		for j := range cfg.Repos[i].Modes {
			m := &cfg.Repos[i].Modes[j]
			if m.Type != "sandbox" || m.SandboxName == "" || m.Agent == "" {
				continue
			}
			candidates := sandbox.Candidates(m.SandboxName, repoName, repoPath, m.Agent)
			matched, _, ok := sandbox.MatchStatus(a.sbxStatuses, candidates)
			if ok && matched != m.SandboxName {
				m.SandboxName = matched
				changed = true
			}
		}
	}
	return changed
}

// reconcileSandboxName updates an in-memory mode and its on-disk config entry
// from oldName to newName, then refreshes the repo panel and the refresh
// manager's candidate list. No-op when oldName == newName or newName is empty.
func (a *App) reconcileSandboxName(re *repoEntry, oldName, newName string) {
	if newName == "" || oldName == newName {
		return
	}

	// Update in-memory group modes.
	for i := range re.group.Modes {
		m := &re.group.Modes[i]
		if m.Type == "sandbox" && m.SandboxName == oldName {
			m.SandboxName = newName
		}
	}
	// state.ActiveMode points to a separate copy (see buildRepoEntry /
	// switchMode) — update it too so callers that dereference it see the
	// new name.
	if re.state.ActiveMode != nil && re.state.ActiveMode.Type == "sandbox" && re.state.ActiveMode.SandboxName == oldName {
		re.state.ActiveMode.SandboxName = newName
	}

	// Regenerate candidates with the new stored name first so subsequent
	// refreshes prefer it.
	agent := ""
	if re.state.ActiveMode != nil {
		agent = re.state.ActiveMode.Agent
	}
	re.refreshMgr.SetSandboxCandidates(
		sandbox.Candidates(newName, re.repo.RepoName(), re.repo.Root(), agent),
	)

	// Rebuild the repo panel since mode labels and sandbox-name lookups
	// depend on the stored name.
	if a.repoPanel != nil {
		a.repoPanel.groups = a.collectGroups()
		a.repoPanel.rebuildList()
	}

	// Persist to disk. Failure is non-fatal: the in-memory state is correct
	// for this session.
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return
	}
	if cfg.UpdateSandboxName(re.group.Path, oldName, newName) {
		_ = config.Save(a.configPath, cfg)
	}
}

// buildMainLayout assembles the two-panel window layout from the current
// a.repos slice. Assumes len(a.repos) >= 1. Starts the first repo's refresh
// manager and initializes the title bar, repo panel, and dashboard slot.
func (a *App) buildMainLayout() fyne.CanvasObject {
	// Build repo panel (left side).
	a.repoPanel = NewRepoPanel(a.collectGroups(), a.sbxStatuses)
	a.repoPanel.OnModeSelected = func(gi, mi int) {
		a.focus = focusLeft // clicking the tree means left panel has focus
		a.switchMode(gi, mi)
	}
	a.repoPanel.OnReorder = a.reorderRepos

	// Active dashboard (right side).
	a.active = 0
	a.dashboard = a.repos[0].dashboard
	a.refreshMgr = a.repos[0].refreshMgr
	a.dashSlot = container.NewStack(a.dashboard.Content())

	// Start the active repo's refresh manager.
	a.repos[0].refreshMgr.Start()

	// Title bar. Kept on the App so toggleTheme can recolor without rebuild.
	a.titleText = monoText("biomelab", colorSelected, true)
	a.titleText.TextSize = scaledSize(11)
	a.titleText.Alignment = fyne.TextAlignCenter
	a.titleBg = canvas.NewRectangle(colorPanelBg)
	titleBar := container.NewStack(a.titleBg, container.NewPadded(a.titleText))

	// Two-panel layout.
	split := container.NewHSplit(a.repoPanel.Content(), a.dashSlot)
	split.Offset = 0.18

	// First-run banner: shown above the split when one or more primary
	// dependencies are missing or degraded. Click → opens the dialog.
	// buildDepsBanner returns nil when everything is OK, in which case
	// we omit the row entirely.
	top := fyne.CanvasObject(titleBar)
	if banner := a.buildDepsBanner(); banner != nil {
		top = container.NewVBox(titleBar, banner)
	}

	return container.NewBorder(top, nil, nil, nil, split)
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

		var sbxCandidates []string
		if mode.Type == "sandbox" {
			re.state.SandboxStatus = sandbox.StatusNotFound
			sbxCandidates = sandbox.Candidates(mode.SandboxName, re.repo.RepoName(), re.repo.Root(), mode.Agent)
			if _, s, ok := sandbox.MatchStatus(a.sbxStatuses, sbxCandidates); ok {
				re.state.SandboxStatus = s
			}
		}
		re.refreshMgr.SetSandboxCandidates(sbxCandidates)
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

// loadInitialVariant reads the saved theme variant from the config,
// defaulting to dark.
func (a *App) loadInitialVariant() ThemeVariant {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return VariantDark
	}
	if cfg.Theme == string(VariantLight) {
		return VariantLight
	}
	return VariantDark
}

// hasNonSandboxMode returns true when any mode in the list is not "sandbox".
// Used to gate host-side regent bootstrap: a sandbox-only repo has rgt
// installed inside the container (via the regent kit), so the host has no
// business creating .regent/ for it.
func hasNonSandboxMode(modes []config.ModeEntry) bool {
	for _, m := range modes {
		if m.Type != "sandbox" {
			return true
		}
	}
	return false
}

// migrateRegentForRepo lists the repo's worktrees and runs regent.EnsureInit
// on each. Safe to call when rgt isn't installed (EnsureInit no-ops).
// Runs as a background goroutine — no return value, errors are best-effort.
func migrateRegentForRepo(repo *git.Repository) {
	wts, err := repo.ListWorktrees()
	if err != nil {
		return
	}
	paths := make([]string, 0, len(wts))
	for _, wt := range wts {
		paths = append(paths, wt.Path)
	}
	regent.MigrateAll(paths)
}

// toggleTheme flips the theme variant. Used by the Ctrl+T shortcut.
func (a *App) toggleTheme() {
	next := VariantDark
	if a.theme.Variant() == VariantDark {
		next = VariantLight
	}
	a.applyThemeVariant(next)
}

// applyThemeVariant installs the given variant, refreshes the UI, syncs the
// system tray checkmarks, and persists the choice to the config file.
// A no-op if the variant is already active.
func (a *App) applyThemeVariant(v ThemeVariant) {
	if v != VariantLight && v != VariantDark {
		return
	}
	if a.theme.Variant() == v {
		return
	}
	a.theme.SetVariant(v)
	a.fyneApp.Settings().SetTheme(a.theme)

	if a.titleBg != nil {
		a.titleBg.FillColor = colorPanelBg
		a.titleBg.Refresh()
	}
	if a.titleText != nil {
		a.titleText.Color = colorSelected
		a.titleText.Refresh()
	}
	if a.repoPanel != nil {
		a.repoPanel.RebuildFull()
	}
	if a.dashboard != nil {
		a.dashboard.Rebuild()
	}
	a.refreshTrayTheme()

	cfg, err := config.Load(a.configPath)
	if err != nil {
		return
	}
	cfg.Theme = string(a.theme.Variant())
	_ = config.Save(a.configPath, cfg)
}

// refreshTrayTheme updates the system tray theme submenu checkmarks to match
// the current theme variant.
func (a *App) refreshTrayTheme() {
	if a.trayThemeLight == nil || a.trayThemeDark == nil {
		return
	}
	a.trayThemeLight.Checked = a.theme.Variant() == VariantLight
	a.trayThemeDark.Checked = a.theme.Variant() == VariantDark
	if desk, ok := a.fyneApp.(desktop.App); ok && a.trayMenu != nil {
		desk.SetSystemTrayMenu(a.trayMenu)
	}
}

func (a *App) emptyState() fyne.CanvasObject {
	msg := widget.NewLabel("No repositories registered.\nPress [a] to add one, or run biomelab from a git repository to auto-add it.")
	msg.Alignment = fyne.TextAlignCenter
	return container.NewCenter(msg)
}
