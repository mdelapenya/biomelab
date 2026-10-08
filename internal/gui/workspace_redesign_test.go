package gui

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/notes"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func workspaceState() *RepoState {
	return &RepoState{Worktrees: []git.Worktree{
		{Branch: "main", Path: "/projects/biomelab", IsMain: true, Sync: git.SyncUpToDate},
		{Branch: "fix/auth-timeout", Path: "/projects/biomelab-worktrees/auth-timeout", Sync: git.SyncAhead},
		{Branch: "feat/dashboard-layout", Path: "/projects/biomelab-worktrees/dashboard-layout", IsDirty: true},
		{Branch: "docs/installation", Path: "/projects/biomelab-worktrees/installation"},
		{Branch: "fix/terminal-resume", Path: "/projects/biomelab-worktrees/terminal-resume"},
		{Branch: "refactor/provider-cache", Path: "/projects/biomelab-worktrees/provider-cache"},
	}, Provider: provider.ProviderGitHub, SelectedCard: 2, LastLocalRefresh: time.Date(2026, 10, 6, 14, 32, 10, 0, time.UTC), LastNetworkRefresh: time.Date(2026, 10, 6, 14, 31, 58, 0, time.UTC),
		PRs: provider.PRResult{
			"fix/auth-timeout":        {Number: 41, Title: "Fix authentication timeout", State: "closed", URL: "https://example.com/pull/41", CheckStatus: "failure"},
			"docs/installation":       {Number: 43, Title: "Document the installation steps", State: "open", URL: "https://example.com/pull/43", CheckStatus: "pending"},
			"fix/terminal-resume":     {Number: 44, Title: "Resume existing terminal sessions", State: "open", URL: "https://example.com/pull/44", ReviewStatus: "approved", CheckStatus: "failure"},
			"refactor/provider-cache": {Number: 45, Title: "Cache provider status responses", State: "merged", URL: "https://example.com/pull/45", ReviewStatus: "approved", CheckStatus: "success"},
		}}
}
func TestWorkspaceListSelectionNavigationAndPerViewState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := polishState()
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 1000, 700)
	a := &App{window: w, dashboard: d, repos: []*repoEntry{{state: s, dashboard: d, group: &RepoGroup{Path: "/projects/repository"}}}}
	a.wireDashboardActions(d)
	test.Tap(polishAction(t, d, "List"))
	if s.ViewMode != ViewList || !d.inspectorOpen || len(d.listRows) != len(s.Worktrees)-1 {
		t.Fatal("List did not expose all real worktrees")
	}
	test.Tap(d.listRows[3].(*tappableCard))
	if s.SelectedCard != 4 {
		t.Fatal("list selected wrong worktree")
	}
	a.navigateDown()
	if s.SelectedCard != 5 {
		t.Fatal("List down must move one item")
	}
	a.navigateRight()
	if s.SelectedCard != 5 {
		t.Fatal("List right moved selection")
	}
	d.listScroll.ScrollToOffset(fyne.NewPos(0, 140))
	offset := d.listScroll.Offset
	test.Tap(polishAction(t, d, "Close"))
	if d.inspectorOpen {
		t.Fatal("inspector did not close")
	}
	test.Tap(polishAction(t, d, "Board"))
	test.Tap(polishAction(t, d, "List"))
	if d.inspectorOpen || d.listScroll.Offset != offset {
		t.Fatalf("list state lost: inspector=%v scroll=%v want=%v", d.inspectorOpen, d.listScroll.Offset, offset)
	}
	a.toggleView()
	if s.ViewMode != ViewKanban {
		t.Fatal("g from List must return Board")
	}
	a.toggleView()
	if s.ViewMode != ViewGrid {
		t.Fatal("legacy Board/Grid toggle changed")
	}
	a.dialogOpen = true
	d.OnViewChanged(ViewList)
	if s.ViewMode != ViewGrid {
		t.Fatal("view action operated beneath modal")
	}
}
func TestWorkspaceInspectorUsesCurrentTerminalAndNoteTarget(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	s.Worktrees[2].Path = t.TempDir()
	s.ViewMode = ViewList
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 1000, 700)
	re := &repoEntry{state: s, dashboard: d, group: &RepoGroup{Path: "/projects/biomelab"}}
	a := &App{fyneApp: app, window: w, dashboard: d, repos: []*repoEntry{re}}
	a.wireDashboardActions(d)
	seedCardTerminal(a, re, s.Worktrees[2])
	test.Tap(polishAction(t, d, "Open Terminal"))
	if got := a.cardTerminals.key.path; got != canonicalTerminalPath(s.Worktrees[2].Path) {
		t.Fatalf("terminal opened %q", got)
	}
	test.Tap(polishAction(t, d, "Notes"))
	if a.noteWindows[s.Worktrees[2].Path] == nil {
		t.Fatal("Notes did not open selected worktree editor")
	}
	for _, window := range a.noteWindows {
		window.Close()
	}
	a.dialogOpen = true
	d.OnEditNotes()
	if len(a.noteWindows) != 0 {
		t.Fatal("notes opened under modal")
	}
	a.dialogOpen = false
	a.dashboard = NewDashboard(workspaceState())
	d.OnEditNotes()
	if len(a.noteWindows) != 0 {
		t.Fatal("stale inspector operated on new dashboard")
	}
}
func TestWorkspaceInspectorPreservesPassiveCanvas(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	s.ViewMode = ViewList
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 700, 600)
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		if _, ok := o.(fyne.Focusable); ok {
			t.Errorf("workspace contains Focusable %T", o)
		}
	})
	overlays := len(w.Canvas().Overlays().List())
	root := w.Content()
	calls := 0
	d.OnOpenTerminal = func() { calls++ }
	d.Rebuild()
	c := polishAction(t, d, "Open Terminal")
	test.MoveMouse(w.Canvas(), fyne.CurrentApp().Driver().AbsolutePositionForObject(c).Add(fyne.NewPos(3, 3)))
	test.Tap(c)
	if calls != 1 || len(w.Canvas().Overlays().List()) != overlays || w.Content() != root || w.Canvas().Focused() != nil {
		t.Fatal("hover/click changed canvas routing")
	}
}
func TestWorkspaceProductionArtifacts(t *testing.T) {
	dir := os.Getenv("BIOMELAB_PRODUCTION_IMAGES")
	if dir == "" {
		t.Skip("set BIOMELAB_PRODUCTION_IMAGES for production captures")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	app := test.NewApp()
	defer app.Quit()
	t.Cleanup(applyDarkPalette)
	savedIcon := AppIcon
	defer func() { AppIcon = savedIcon }()
	data, err := os.ReadFile("../../cmd/biomelab/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	AppIcon = fyne.NewStaticResource("icon.png", data)
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		for _, scenario := range []struct {
			name              string
			view              ViewMode
			open, zoom, empty bool
			width, height     float32
		}{
			{"board", ViewKanban, false, false, false, 1440, 900}, {"board-inspector", ViewKanban, true, false, false, 1440, 900},
			{"list", ViewList, true, false, false, 1200, 760}, {"grid", ViewGrid, false, false, false, 1200, 760},
			{"narrow-board", ViewKanban, false, false, false, 900, 700}, {"narrow-list", ViewList, true, false, false, 900, 700},
			{"zoom-board", ViewKanban, true, true, false, 1100, 760}, {"empty", ViewKanban, false, false, true, 1000, 700},
		} {
			th := newBiomeTheme(variant)
			if scenario.zoom {
				th.ZoomIn()
				th.ZoomIn()
			}
			app.Settings().SetTheme(th)
			s := workspaceState()
			s.ViewMode = scenario.view
			if scenario.open {
				s.SelectedCard = 4
			}
			d := NewDashboard(s)
			d.RepoName = "biomelab"
			d.inspectorOpen = scenario.open
			d.Rebuild()
			rp := NewRepoPanel([]*RepoGroup{{Name: "biomelab", Path: "/projects/biomelab", LinkedWorktreeCount: 5, Modes: []config.ModeEntry{{Type: "regular"}, {Type: "sandbox", Agent: "Claude", SandboxName: "dev"}}}, {Name: "agent-tools", Modes: []config.ModeEntry{{Type: "regular"}}}}, map[string]sandbox.Status{"dev": sandbox.StatusRunning})
			content := newShellLayout(rp.Content(), d.Content(), nil)
			if scenario.empty {
				a := &App{}
				content = a.emptyState()
			}
			w := app.NewWindow("Biomelab production fixture")
			w.SetPadded(false)
			if !scenario.empty {
				a := &App{fyneApp: app, window: w, dashboard: d, repos: []*repoEntry{{state: s, dashboard: d, group: &RepoGroup{Path: "/projects/biomelab"}}}}
				a.wireDashboardActions(d)
			}
			w.SetContent(content)
			w.Resize(fyne.NewSize(scenario.width, scenario.height))
			w.Show()
			d.EnsureVisible()

			file, err := os.Create(filepath.Join(dir, string(variant)+"-"+scenario.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, w.Canvas().Capture())
			closeErr := file.Close()
			w.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
	}
}

func TestWorkspaceGridInspectorNavigationUsesRenderedColumns(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := polishState()
	s.ViewMode = ViewGrid
	s.SelectedCard = 1
	d := NewDashboard(s)
	d.inspectorOpen = true
	d.Rebuild()
	w := polishWindow(t, app, d, 1440, 900)
	a := &App{window: w, dashboard: d, dashSlot: container.NewStack(d.Content())}
	a.dashSlot.Resize(fyne.NewSize(1249, 900))
	grid := d.scroll.Content.(*fyne.Container).Objects[1].(*fyne.Container)
	cols := grid.Layout.(*flexGridLayout).colCount
	if cols < 1 {
		t.Fatal("grid not laid out")
	}
	a.navigateDown()
	if s.SelectedCard != 1+cols {
		t.Fatalf("Down selected%d using%d actual columns", s.SelectedCard, cols)
	}
}
func TestWorkspaceListKeepsProviderCreateActionsReachable(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	s.ViewMode = ViewList
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 900, 700)
	called := 0
	d.OnCreateFromIssue = func() { called++ }
	d.OnFetchPR = func() { called++ }
	d.inspectorOpen = false
	d.Rebuild()
	test.Tap(polishAction(t, d, "More"))
	if len(w.Canvas().Overlays().List()) == 0 {
		t.Fatal("List did not expose explicit creation menu")
	}
	for _, item := range d.createMenu().Items {
		if item.Disabled {
			t.Fatal("GitHub command unexpectedly disabled")
		}
		item.Action()
	}
	if called != 2 {
		t.Fatal("creation menu callbacks not invoked")
	}
	s.Provider = provider.ProviderGitLab
	for _, item := range d.createMenu().Items {
		if !item.Disabled {
			t.Fatal("GitHub command enabled for GitLab")
		}
	}
}

func TestWorkspaceInspectorRetainsRefreshScrollAndResetsForNewPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	s.ViewMode = ViewList
	s.Terminals = map[string][]terminal.Info{}
	for range 12 {
		s.Terminals[s.Worktrees[2].Path] = append(s.Terminals[s.Worktrees[2].Path], terminal.Info{ShellPID: 42})
	}
	d := NewDashboard(s)
	polishWindow(t, app, d, 1000, 400)
	d.inspectorScroll.ScrollToOffset(fyne.NewPos(0, 150))
	offset := d.inspectorScroll.Offset
	if offset.Y == 0 {
		t.Fatal("fixture inspector did not overflow")
	}
	scroll := d.inspectorScroll
	d.Rebuild()
	if d.inspectorScroll != scroll || d.inspectorScroll.Offset != offset {
		t.Fatalf("refresh reset inspector reading position: %v want%v", d.inspectorScroll.Offset, offset)
	}
	app.Settings().SetTheme(NewTheme(VariantDark))
	d.Rebuild()
	if d.inspectorScroll.Offset != offset {
		t.Fatal("theme reset inspector position")
	}
	d.selectCard(3)
	if d.inspectorScroll.Offset.Y != 0 {
		t.Fatal("new worktree retained previous inspector position")
	}
}
func TestWorkspaceInspectorRecognizesTitleOnlyDraft(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(notes.TitlePath(dir)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes.TitlePath(dir), []byte("feat(gui): compact workspace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspaceState()
	s.ViewMode = ViewList
	s.Worktrees[2].Path = dir
	d := NewDashboard(s)
	polishWindow(t, app, d, 1000, 700)
	found := false
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		if v, ok := o.(*wrappedValue); ok && strings.Contains(v.value, "Task notes available") {
			found = true
		}
	})
	if !found {
		t.Fatal("title-only draft was shown as no task notes")
	}
}
func TestWorkspacePropertyHeightFollowsAllocatedTextWidth(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	property := inspectorProperty("Path", "/projects/biomelab-worktrees/terminal-resume", true).(*fyne.Container)
	property.Resize(fyne.NewSize(230, 100))
	narrow := property.MinSize().Height
	property.Resize(fyne.NewSize(650, 100))
	wide := property.MinSize().Height
	if wide >= narrow {
		t.Fatalf("property height did not contract for unwrapped text: wide%v narrow%v", wide, narrow)
	}
}

func TestWorkspaceWorkflowCompletionRestoresUnderlyingMessage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	d := NewDashboard(workspaceState())
	w := polishWindow(t, app, d, 1000, 700)
	a := &App{fyneApp: app, window: w, dashboard: d}
	a.showInformation("Refresh error", "Provider unavailable")
	message := a.activeDialog
	done := a.openDialog()
	flow := showBranchInput(w, done, func(string) {})
	a.activeDialog = flow
	flow.Hide()
	if a.activeDialog != message || !a.dialogOpen || len(w.Canvas().Overlays().List()) == 0 {
		t.Fatal("workflow lost underlying visible message ownership")
	}
	selected := d.state.SelectedCard
	a.handleKeyName(fyne.KeyDown)
	if d.state.SelectedCard != selected {
		t.Fatal("background navigation escaped restored modal")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.dialogOpen || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("restored message could not dismiss cleanly")
	}
}
func TestWorkspaceFullLaneCardPaintStaysInsideViewport(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	d := NewDashboard(workspaceState())
	w := polishWindow(t, app, d, 1440, 900)
	stage := d.KanbanColumnOf(d.state.SelectedCard)
	card := d.stageCards[stage][0].(*kanbanCardWrapper).inner
	pos := app.Driver().AbsolutePositionForObject(card)
	scrollPos := app.Driver().AbsolutePositionForObject(d.stageScrolls[stage])
	if pos.X+card.Size().Width >= scrollPos.X+d.stageScrolls[stage].Size().Width {
		t.Fatal("card border has no paint room inside full lane viewport")
	}
	capture := w.Canvas().Capture()
	// Driver positions are relative to the interactive safe area; capture
	// coordinates include that inset.
	inset, _ := w.Canvas().InteractiveArea()
	pos = pos.Add(inset)
	y := int(pos.Y + card.Size().Height/2)
	r, g, b, _ := capture.At(int(pos.X+card.Size().Width-2), y).RGBA()
	wr, wg, wb, _ := colorActionBg.RGBA()
	if r != wr || g != wg || b != wb {
		t.Fatal("selected card right edge was clipped")
	}
	r, g, b, _ = capture.At(int(pos.X+card.Size().Width+3), y).RGBA()
	wr, wg, wb, _ = colorBackground.RGBA()
	if r != wr || g != wg || b != wb {
		t.Fatalf("selected card painted beyond its lane: got%d/%d/%d want%d/%d/%d pos%v size%v scroll%v", r, g, b, wr, wg, wb, pos, card.Size(), scrollPos)
	}
}

func TestWorkspaceRendersSubagentDistinctionInGridAndInspector(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	state := workspaceState()
	state.ViewMode = ViewGrid
	state.Agents = agent.DetectionResult{state.Worktrees[2].Path: {{Kind: agent.Claude, PID: "100", State: "S"}, {Kind: agent.Claude, PID: "101", State: "S", IsSubAgent: true}}}
	d := NewDashboard(state)
	polishWindow(t, app, d, 1200, 700)
	contains := func(root fyne.CanvasObject, want string) bool {
		found := false
		walkPolish(root, func(o fyne.CanvasObject) {
			if text, ok := o.(*canvas.Text); ok && strings.Contains(text.Text, want) {
				found = true
			}
		})
		return found
	}
	if !contains(d.Content(), "claude · PID 100") || !contains(d.Content(), "↳ claude · PID 101") || contains(d.Content(), "↳ claude · PID 100") {
		t.Fatal("Grid lost detected parent/subagent distinction")
	}
	d.inspectorOpen = true
	d.Rebuild()
	if !contains(d.inspectorScroll.Content, "↳ claude") || !contains(d.inspectorScroll.Content, "PID 101") {
		t.Fatal("inspector lost detected subagent label or process metadata")
	}
}

func TestWorkspaceMainCardPinnedAcrossViewsAndNarrowInspector(t *testing.T) {
	for _, view := range []ViewMode{ViewKanban, ViewList, ViewGrid} {
		for _, width := range []float32{700, 1200} {
			t.Run(fmt.Sprintf("view%d-width%.0f", view, width), func(t *testing.T) {
				app := test.NewApp()
				defer app.Quit()
				app.Settings().SetTheme(NewTheme(VariantLight))
				t.Cleanup(applyDarkPalette)
				state := workspaceState()
				state.ViewMode = view
				d := NewDashboard(state)
				d.inspectorOpen = true
				d.Rebuild()
				w := polishWindow(t, app, d, width, 600)
				if d.mainCard == nil {
					t.Fatal("view omitted main checkout card")
				}
				main := d.mainCard
				mainPos := app.Driver().AbsolutePositionForObject(main)
				if main.Size().Width < d.Content().Size().Width-scaledSize(spaceMD*2)-1 {
					t.Fatalf("main is confined to a pane in view%v", view)
				}
				root := d.innerSlot.Objects[0].(*fyne.Container)
				bodyPos := app.Driver().AbsolutePositionForObject(root.Objects[2])
				if mainPos.Y+main.Size().Height > bodyPos.Y {
					t.Fatal("main overlaps linked-worktree panes")
				}
				heading := false
				walkPolish(main, func(o fyne.CanvasObject) {
					if m, ok := o.(*measuredText); ok && m.full == "Main checkout" {
						heading = true
					}
					if _, ok := o.(fyne.Focusable); ok {
						t.Errorf("main contains Focusable%T", o)
					}
				})
				if !heading {
					t.Fatal("main heading missing")
				}
				if view == ViewList {
					if len(d.listRows) != len(state.LinkedWorktrees()) {
						t.Fatal("List duplicates main or loses linked worktrees")
					}
					test.Tap(d.listRows[0].(*tappableCard))
					if state.SelectedCard != 1 {
						t.Fatal("first list row no longer selects first linked worktree")
					}
				}
				test.Tap(d.mainCard)
				if state.SelectedCard != 0 {
					t.Fatal("main card did not select index0")
				}
				a := &App{window: w, dashboard: d}
				a.navigateDown()
				if state.SelectedCard != 1 {
					t.Fatal("Down from pinned main failed")
				}
				a.navigateUp()
				if state.SelectedCard != 0 {
					t.Fatal("Up from first linked failed to return to main")
				}
			})
		}
	}
}
func TestWorkspaceMainActionsTargetCurrentMainAndRespectGuards(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	state := workspaceState()
	state.ViewMode = ViewList
	state.Worktrees[0].Path = t.TempDir()
	d := NewDashboard(state)
	w := polishWindow(t, app, d, 1000, 700)
	re := &repoEntry{state: state, dashboard: d, group: &RepoGroup{Path: state.Worktrees[0].Path}}
	a := &App{fyneApp: app, window: w, dashboard: d, repos: []*repoEntry{re}}
	a.wireDashboardActions(d)
	seedCardTerminal(a, re, state.Worktrees[0])
	a.dialogOpen = true
	d.OnMainTerminal()
	d.OnMainEditor()
	d.OnMainNotes()
	if state.SelectedCard != 2 || len(a.noteWindows) > 0 {
		t.Fatal("main action operated under modal")
	}
	a.dialogOpen = false
	test.Tap(polishAction(t, d, "Terminal"))
	if got := a.cardTerminals.key.path; got != canonicalTerminalPath(state.Worktrees[0].Path) {
		t.Fatalf("main terminal opened%q", got)
	}
	state.SelectedCard = 2
	d.Rebuild()
	d.OnMainNotes()
	if state.SelectedCard != 0 || a.noteWindows[state.Worktrees[0].Path] == nil {
		t.Fatal("main Notes action targeted linked checkout")
	}
	for _, nw := range a.noteWindows {
		nw.Close()
	}
	menu := d.createMenu()
	if menu.Items[0].Disabled || menu.Items[1].Disabled {
		t.Fatal("main context creation actions lost")
	}
	main := state.MainWorktree()
	if main == nil || !main.IsMain {
		t.Fatal("main safety metadata changed")
	}
	before := state.StatusMessage
	a.handleDeleteOrRemoveSandbox()
	if state.SelectedCard != 0 || a.dialogOpen || state.StatusMessage != before {
		t.Fatal("main deletion safety was lost")
	}

	a.dashboard = NewDashboard(workspaceState())
	state.SelectedCard = 2
	d.OnMainTerminal()
	d.OnMainEditor()
	d.OnMainNotes()
	if state.SelectedCard != 2 || len(a.noteWindows) != 0 {
		t.Fatal("stale main action operated on another dashboard")
	}
}

func TestExpandedListTerminalShowsOwningCardAboveDrawer(t *testing.T) {
	a, re, w := terminalFixture(t)
	d := a.dashboard
	d.RepoName = "biomelab"
	re.group.Name = "biomelab"
	re.state.ViewMode = ViewList
	re.state.SelectedCard = 4
	d.Rebuild()
	owner := re.state.Worktrees[4]
	seedCardTerminal(a, re, owner)
	a.handleEnter()
	p := a.cardTerminals
	p.expanded = true
	d.Rebuild()
	pinnedTexts := func() string {
		root := d.innerSlot.Objects[0].(*fyne.Container)
		var values []string
		walkPolish(root.Objects[1], func(o fyne.CanvasObject) {
			if label, ok := o.(*measuredText); ok {
				values = append(values, label.full)
			}
		})
		return strings.Join(values, "\n")
	}
	assertOwner := func(title, branch string) {
		t.Helper()
		text := pinnedTexts()
		if !strings.Contains(text, "Terminal · biomelab · regular") || !strings.Contains(text, title) || !strings.Contains(text, branch) || strings.Contains(text, "Main checkout") {
			t.Fatalf("expanded owner summary missing context: %q", text)
		}
	}
	assertOwner("Resume existing terminal sessions", owner.Branch)
	if p.slot.Size().Height < 100 {
		t.Fatalf("expanded terminal did not render: %v", p.slot.Size())
	}
	capture := func(name string) {
		dir := os.Getenv("BIOMELAB_N16_SCREENSHOTS")
		if dir == "" {
			return
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	capture("biomelab-n16-expanded-list.png")
	d.selectCard(3)
	assertOwner("Document the installation steps", re.state.Worktrees[3].Branch)
	if p.sessions[cardKey(re, re.state.Worktrees[3])] != nil {
		t.Fatal("switching cards started a shell")
	}
	foundPrompt := false
	walkPolish(p.slot, func(o fyne.CanvasObject) {
		if button, ok := o.(*widget.Button); ok && button.Text == "Open terminal" {
			foundPrompt = true
		}
	})
	if !foundPrompt {
		t.Fatal("selected card without session lost its terminal prompt")
	}
	d.selectCard(0)
	assertOwner("main", re.state.Worktrees[0].Branch)
	d.selectCard(4)
	p.expanded = false
	d.Rebuild()
	if text := pinnedTexts(); !strings.Contains(text, "Main checkout") || strings.Contains(text, "Resume existing terminal sessions") {
		t.Fatalf("collapsed terminal did not restore Main summary: %q", text)
	}
	capture("biomelab-n16-restored-list.png")
	w.Resize(fyne.NewSize(1200, 520))
	fyne.DoAndWait(func() {})
	if !p.autoExpanded {
		t.Fatal("short window did not automatically expand the terminal")
	}
	assertOwner("Resume existing terminal sessions", owner.Branch)
	w.Resize(fyne.NewSize(1200, 820))
	fyne.DoAndWait(func() {})
	if p.autoExpanded || !strings.Contains(pinnedTexts(), "Main checkout") {
		t.Fatal("growing window did not restore the Main summary")
	}
	d.mainExpanded = true
	d.Rebuild()
	onAutoChanged := p.onAutoExpandedChanged
	changes := 0
	p.onAutoExpandedChanged = func() {
		changes++
		if changes <= 4 {
			onAutoChanged()
		}
	}
	nearThreshold := float32(0)
	sawNormal := false
	for height := float32(480); height <= 1100; height += 20 {
		changes = 0
		w.Resize(fyne.NewSize(1200, height))
		fyne.DoAndWait(func() {})
		stable := p.autoExpanded
		for i := 0; i < 3; i++ {
			d.Rebuild()
			fyne.DoAndWait(func() {})
			if p.autoExpanded != stable {
				t.Fatalf("auto expansion oscillated at height %.0f", height)
			}
		}
		if changes > 1 {
			t.Fatalf("auto expansion rebuilt %d times at height %.0f", changes, height)
		}
		if stable {
			nearThreshold = height
			assertOwner("Resume existing terminal sessions", owner.Branch)
			if p.slot.Size().Height < 100 {
				t.Fatalf("terminal unusable at height %.0f: %v", height, p.slot.Size())
			}
		} else {
			sawNormal = true
			if !strings.Contains(pinnedTexts(), "Main checkout") {
				t.Fatalf("normal Main summary missing at height %.0f", height)
			}
		}
	}
	if nearThreshold == 0 || !sawNormal {
		t.Fatal("height sweep did not cross the automatic expansion threshold")
	}
	w.Resize(fyne.NewSize(1200, nearThreshold))
	fyne.DoAndWait(func() {})
	d.selectCard(3)
	assertOwner("Document the installation steps", re.state.Worktrees[3].Branch)
	d.selectCard(0)
	assertOwner("main", re.state.Worktrees[0].Branch)
	d.selectCard(4)
	if !p.autoExpanded {
		t.Fatal("switching tasks near threshold lost automatic expansion")
	}
	w.Resize(fyne.NewSize(1200, 1100))
	fyne.DoAndWait(func() {})
	p.expanded = true
	d.Rebuild()
	assertOwner("Resume existing terminal sessions", owner.Branch)
	p.expanded = false
	d.Rebuild()
	if !strings.Contains(pinnedTexts(), "Main checkout") {
		t.Fatal("restoring manual expansion did not restore Main summary")
	}
}

func TestNarrowListTerminalAutoExpansionStaysStable(t *testing.T) {
	for _, mainExpanded := range []bool{false, true} {
		for _, width := range []float32{500, 600, 700, 800} {
			name := fmt.Sprintf("width_%.0f_details_%v", width, mainExpanded)
			t.Run(name, func(t *testing.T) {
				a, re, w := terminalFixture(t)
				d := a.dashboard
				re.state.ViewMode = ViewList
				re.state.SelectedCard = 4
				d.mainExpanded = mainExpanded
				d.Rebuild()
				seedCardTerminal(a, re, re.state.Worktrees[4])
				a.handleEnter()
				p := a.cardTerminals
				original := p.onAutoExpandedChanged
				changes := 0
				p.onAutoExpandedChanged = func() {
					changes++
					if changes <= 6 {
						original()
					}
				}
				for height := float32(500); height <= 1200; height += 10 {
					changes = 0
					w.Resize(fyne.NewSize(width, height))
					fyne.DoAndWait(func() {})
					stable := p.autoExpanded
					for i := 0; i < 2; i++ {
						d.Rebuild()
						fyne.DoAndWait(func() {})
						if p.autoExpanded != stable {
							t.Fatalf("auto expansion flipped after rebuild at %.0fx%.0f", width, height)
						}
					}
					if changes > 1 {
						t.Fatalf("auto expansion changed %d times at %.0fx%.0f", changes, width, height)
					}
					if p.slot.Size().Height < 100 {
						t.Fatalf("terminal drawer unusable at %.0fx%.0f: %v", width, height, p.slot.Size())
					}
					root := d.innerSlot.Objects[0].(*fyne.Container)
					header, mainPanel, browser := root.Objects[0], root.Objects[1], root.Objects[2]
					if header.Position().Y+header.Size().Height > mainPanel.Position().Y+1 || mainPanel.Position().Y+mainPanel.Size().Height > browser.Position().Y+1 {
						t.Fatalf("toolbar/Main/browser overlap at %.0fx%.0f", width, height)
					}
					d.selectCard(3)
					if p.autoExpanded != stable {
						t.Fatalf("card switch changed expansion at %.0fx%.0f", width, height)
					}
					d.selectCard(4)
				}
			})
		}
	}
}
func TestWorkspaceMainOnlyAndNoDataStates(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	for _, view := range []ViewMode{ViewKanban, ViewList, ViewGrid} {
		state := workspaceState()
		state.Worktrees = state.Worktrees[:1]
		state.SelectedCard = 0
		state.ViewMode = view
		d := NewDashboard(state)
		polishWindow(t, app, d, 900, 600)
		if d.mainCard == nil {
			t.Fatal("main-only repo lost prominent main")
		}
		created := 0
		d.OnCreate = func() { created++ }
		test.Tap(polishAction(t, d, "Create a worktree"))
		if created != 1 {
			t.Fatal("main-only repo lost create CTA")
		}
		state.Worktrees = nil
		d.Rebuild()
		if d.mainCard != nil {
			t.Fatal("no-data repo fabricated a main checkout")
		}
	}
}
