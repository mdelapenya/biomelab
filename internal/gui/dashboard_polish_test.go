package gui

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
)

func polishState() *RepoState {
	s := &RepoState{Worktrees: []git.Worktree{{Branch: "main", Path: "/projects/repository", IsMain: true, Sync: git.SyncUpToDate}}, Provider: provider.ProviderGitHub, PRs: provider.PRResult{}}
	for i := 0; i < 30; i++ {
		branch := fmt.Sprintf("feature/%02d-with-a-long-branch-name", i)
		s.Worktrees = append(s.Worktrees, git.Worktree{Branch: branch, Path: "/projects/repository/worktrees/" + branch})
	}
	s.PRs[s.Worktrees[len(s.Worktrees)-1].Branch] = &provider.PRInfo{State: "merged", Number: 42, Title: "A very long pull request title", URL: "https://example.com/pull/42"}
	return s
}
func walkPolish(obj fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(obj)
	if c, ok := obj.(*fyne.Container); ok {
		for _, o := range c.Objects {
			walkPolish(o, fn)
		}
	} else if w, ok := obj.(fyne.Widget); ok {
		for _, o := range test.WidgetRenderer(w).Objects() {
			walkPolish(o, fn)
		}
	}
}
func polishAction(t *testing.T, d *Dashboard, label string) *actionControl {
	t.Helper()
	var found *actionControl
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		if c, ok := o.(*actionControl); ok && c.label == label {
			found = c
		}
	})
	if found == nil {
		t.Fatalf("missing visible action %q", label)
	}
	return found
}
func polishWindow(t *testing.T, app fyne.App, d *Dashboard, width, height float32) fyne.Window {
	t.Helper()
	w := app.NewWindow("test")
	w.SetContent(d.Content())
	w.Resize(fyne.NewSize(width, height))
	w.Show()
	t.Cleanup(w.Close)
	return w
}

func TestDashboardControlsPreserveCanvasKeyboardRouting(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	d := NewDashboard(polishState())
	w := polishWindow(t, app, d, 800, 600)
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		if _, ok := o.(fyne.Focusable); ok {
			t.Errorf("dashboard contains focusable %T", o)
		}
	})
	created, refreshed := 0, 0
	d.OnCreate = func() { created++ }
	d.OnRefresh = func() { refreshed++ }
	test.Tap(polishAction(t, d, "New Worktree"))
	test.Tap(polishAction(t, d, "Refresh"))
	test.Tap(polishAction(t, d, "Grid"))
	if created != 1 || refreshed != 1 || d.state.ViewMode != ViewGrid || w.Canvas().Focused() != nil {
		t.Fatalf("controls lost callback or focus routing: create=%d refresh=%d view=%v focus=%v", created, refreshed, d.state.ViewMode, w.Canvas().Focused())
	}
	test.Tap(polishAction(t, d, "Board"))
	if d.state.ViewMode != ViewKanban {
		t.Fatal("Board control did not switch view")
	}
	disabled := newActionControl("Disabled", nil, false, func() { created++ })
	disabled.disabled = true
	test.Tap(disabled)
	if created != 1 {
		t.Fatal("disabled control invoked callback")
	}
}

func TestDashboardActionsResolveCurrentRepoAndSelectMain(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	a := &App{window: app.NewWindow("test")}
	defer a.window.Close()
	for _, path := range []string{"/one", "/two"} {
		s := polishState()
		s.SelectedCard = 3
		d := NewDashboard(s)
		re := &repoEntry{group: &RepoGroup{Path: path}, state: s, dashboard: d}
		a.repos = append(a.repos, re)
		a.wireDashboardActions(d)
	}
	a.active = 1
	a.dashboard = a.repos[1].dashboard
	a.window.SetContent(a.dashboard.Content())
	a.repos[0].dashboard.OnCreate()
	if a.dialogOpen || a.repos[0].state.SelectedCard != 3 {
		t.Fatal("old dashboard acted on another repository")
	}
	test.Tap(polishAction(t, a.dashboard, "New Worktree"))
	if !a.dialogOpen || a.repos[1].state.SelectedCard != 0 || a.repos[0].state.SelectedCard != 3 {
		t.Fatal("visible create action failed to select the current main worktree")
	}
	a.handleKeyName(fyne.KeyEscape)
	a.dashboard.state.Provider = provider.ProviderGitLab
	a.dashboard.OnFetchPR()
	if a.dialogOpen {
		t.Fatal("GitLab dashboard exposed GitHub PR fetch")
	}
	a.dashboard.state.SelectedCard = 4
	a.dashboard.OnCreateFromIssue()
	if a.dialogOpen || !strings.Contains(a.dashboard.state.StatusMessage, "GitHub") {
		t.Fatal("issue action skipped provider validation")
	}
	a.dashboard.state.Provider = provider.ProviderGitHub
	a.dashboard.OnFetchPR()
	if !a.dialogOpen || a.dashboard.state.SelectedCard != 0 {
		t.Fatal("GitHub fetch action failed to open on main")
	}
	a.handleKeyName(fyne.KeyEscape)
	a.dashboard.OnCreateFromIssue()
	if !a.dialogOpen || a.dashboard.state.SelectedCard != 0 {
		t.Fatal("GitHub issue action failed to open on main")
	}
	a.handleKeyName(fyne.KeyEscape)
	// A refresh click must use the network lane of the active manager.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rm := &RefreshManager{run: &refreshRun{ctx: ctx, cancel: cancel, generation: 1}, cliChecked: true}
	rm.lanes[networkRefresh].running = true // Keep queued work inspectable without starting any process.
	a.refreshMgr = rm
	a.dashboard.OnRefresh()
	if rm.cliChecked || rm.lanes[networkRefresh].pending == nil {
		t.Fatal("Refresh did not invoke the existing network handler")
	}
	a.dashboard.OnViewChanged(ViewGrid)
	if a.dashboard.state.ViewMode != ViewGrid {
		t.Fatal("view action failed to invoke existing toggle")
	}
}

func TestDashboardPreservesDetailsAndScrollAcrossRefreshAndTheme(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	s := polishState()
	d := NewDashboard(s)
	polishWindow(t, app, d, 800, 500)
	test.Tap(polishAction(t, d, "Details"))
	if !d.mainExpanded {
		t.Fatal("main details did not expand")
	}
	d.stageScrolls[1].ScrollToOffset(fyne.NewPos(0, 150))
	d.boardScroll.ScrollToOffset(fyne.NewPos(100, 0))
	stageY, boardX := d.stageScrolls[1].Offset.Y, d.boardScroll.Offset.X
	if stageY == 0 || boardX == 0 {
		t.Fatal("fixture failed to scroll the narrow board")
	}
	s.SelectedCard = 3
	if !d.ApplyRefresh(ops.RefreshResult{Worktrees: s.Worktrees}) {
		t.Fatal("refresh was rejected")
	}
	if !d.mainExpanded || s.SelectedCard != 3 || d.stageScrolls[1].Offset.Y != stageY || d.boardScroll.Offset.X != boardX {
		t.Fatalf("refresh lost state: expanded=%v selected=%d stage=%v board=%v", d.mainExpanded, s.SelectedCard, d.stageScrolls[1].Offset, d.boardScroll.Offset)
	}
	th := newBiomeTheme(VariantLight)
	app.Settings().SetTheme(th)
	d.Rebuild()
	if !d.mainExpanded || d.stageScrolls[1].Offset.Y != stageY || d.boardScroll.Offset.X != boardX {
		t.Fatal("theme rebuild lost detail or board scroll state")
	}
	test.Tap(polishAction(t, d, "Hide details"))
	d.setView(ViewGrid)
	d.scroll.ScrollToOffset(fyne.NewPos(0, 250))
	gridY := d.scroll.Offset.Y
	d.setView(ViewKanban)
	d.setView(ViewGrid)
	if gridY == 0 || d.scroll.Offset.Y != gridY {
		t.Fatalf("switching views lost grid scroll: initial=%v restored=%v viewport=%v min=%v content=%v", gridY, d.scroll.Offset, d.scroll.Size(), d.scroll.Content.MinSize(), d.scroll.Content.Size())
	}
	th.ZoomIn()
	app.Settings().SetTheme(th)
	d.Rebuild()
	if d.scroll.Offset.Y != gridY {
		t.Fatal("zoom rebuild lost grid scroll")
	}
	other := NewDashboard(polishState())
	if other.gridOffset != (fyne.Position{}) || other.boardOffset != (fyne.Position{}) {
		t.Fatal("new repository inherited scroll position")
	}
	// Removed content and window changes must clamp offsets to valid bounds.
	d.state.Worktrees = d.state.Worktrees[:2]
	d.Rebuild()
	if d.scroll.Offset.Y < 0 || d.scroll.Offset.Y > max(float32(0), d.scroll.Content.MinSize().Height-d.scroll.Size().Height)+1 {
		t.Fatalf("removed rows left an invalid scroll offset %v", d.scroll.Offset)
	}
}

func TestDashboardKanbanNavigationReachesAndRevealsFinalColumn(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	s := polishState()
	s.SelectedCard = 1
	d := NewDashboard(s)
	polishWindow(t, app, d, 720, 440)
	a := &App{dashboard: d, focus: focusRight}
	a.navigateKanbanRight()
	if s.SelectedCard != len(s.Worktrees)-1 {
		t.Fatalf("right arrow selected %d, want final merged card %d", s.SelectedCard, len(s.Worktrees)-1)
	}
	col := d.boardColumns[4]
	offset := d.boardScroll.Offset.X
	if offset <= 0 || col.Position().X < offset || col.Position().X+col.Size().Width > offset+d.boardScroll.Size().Width+1 {
		t.Fatalf("final column remains outside viewport: column=%v/%v offset=%v viewport=%v", col.Position(), col.Size(), offset, d.boardScroll.Size())
	}
	a.navigateKanbanLeft()
	if s.SelectedCard != 1 {
		t.Fatal("left arrow failed to return to the created column")
	}
	for i := 0; i < 15; i++ {
		a.navigateKanbanDown()
	}
	if d.stageScrolls[1].Offset.Y <= 0 {
		t.Fatal("vertical navigation failed to reveal the selected row")
	}
	for _, col := range d.boardColumns {
		if col.Size().Width < scaledSize(210) {
			t.Fatal("narrow board compressed a column")
		}
	}
}

func TestMeasuredTextFitsLongUnicodeLabels(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	for _, style := range []fyne.TextStyle{{}, {Bold: true}, {Monospace: true}} {
		for _, prefix := range []bool{false, true} {
			for _, width := range []float32{1, 20, 60, 160} {
				text := fitText("Very long 项目 / WWWiiii / café / repository branch", width, 14, style, prefix)
				if fyne.MeasureText(text, 14, style).Width > width {
					t.Fatalf("measured text exceeds width: %q > %v", text, width)
				}
			}
		}
	}
	m := newMeasuredText("A long repository name with wide WWW glyphs", colorForeground, true, false, false)
	m.Resize(fyne.NewSize(120, 30))
	test.WidgetRenderer(m).Layout(m.Size())
	if m.full == m.txt.Text || !strings.HasSuffix(m.txt.Text, "…") {
		t.Fatal("long label did not truncate while preserving the full value")
	}
}

// Optional artifacts make narrow/zoom/grid evidence reproducible without
// altering the documentation generator or writing files during ordinary tests.
func TestDashboardPolishRenderArtifacts(t *testing.T) {
	dir := os.Getenv("BIOMELAB_POLISH_SCREENSHOTS")
	if dir == "" {
		t.Skip("set BIOMELAB_POLISH_SCREENSHOTS to render additional dashboard fixtures")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	app := test.NewApp()
	defer app.Quit()
	t.Cleanup(applyDarkPalette)
	for _, variant := range []ThemeVariant{VariantDark, VariantLight} {
		for _, scenario := range []string{"narrow-board", "grid", "zoom-board", "main-details"} {
			th := newBiomeTheme(variant)
			if scenario == "zoom-board" {
				th.ZoomIn()
				th.ZoomIn()
			}
			app.Settings().SetTheme(th)
			s := polishState()
			if scenario == "grid" {
				s.ViewMode = ViewGrid
			}
			d := NewDashboard(s)
			d.RepoName = "example/with-a-very-long-repository-name"
			if scenario == "main-details" {
				d.mainExpanded = true
				d.Rebuild()
			}
			rp := NewRepoPanel([]*RepoGroup{{Path: "/repo", Name: d.RepoName, Modes: []config.ModeEntry{{Type: "regular"}, {Type: "sandbox", Agent: "claude", SandboxName: "dev"}}, LinkedWorktreeCount: 30}}, nil)
			split := container.NewHSplit(rp.Content(), d.Content())
			split.Offset = .23
			w := app.NewWindow("render")
			w.SetContent(split)
			w.Resize(fyne.NewSize(1000, 680))
			w.Show()
			if scenario == "narrow-board" || scenario == "zoom-board" {
				s.SelectedCard = len(s.Worktrees) - 1
				d.Rebuild()
				d.EnsureVisible()
			}
			f, err := os.Create(filepath.Join(dir, string(variant)+"-"+scenario+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, w.Canvas().Capture())
			closeErr := f.Close()
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

func TestDashboardStatusTypographyAndPointerFeedback(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	labels := prStatusLabels(&provider.PRInfo{ReviewStatus: "approved", CheckStatus: "failure"})
	for _, label := range labels {
		h := label.(*statusText)
		if h.txt.TextStyle != secondaryText("Activity").TextStyle || h.txt.TextSize != scaledSize(textSecondarySize) {
			t.Fatal("review or CI label differs from the shared secondary sans role")
		}
	}
	control := newActionControl("Refresh", nil, false, func() {})
	test.WidgetRenderer(control)
	initial := control.bg.FillColor
	control.MouseIn(nil)
	if control.bg.StrokeWidth == 0 {
		t.Fatal("hover has no visible feedback")
	}
	control.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if control.bg.FillColor == initial {
		t.Fatal("pressed control has no visible feedback")
	}
	control.MouseUp(nil)
	control.MouseOut()
	control.disabled = true
	control.Refresh()
	if control.content.Objects[0].(*canvas.Text).Color != colorDimGray {
		t.Fatal("disabled control has no visible feedback")
	}
}

func TestDashboardNestedScrollClipsPixelsAndPointerTargets(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := polishState()
	d := NewDashboard(s)
	sidebar := canvas.NewRectangle(colorPanelBg)
	sidebar.SetMinSize(fyne.NewSize(180, 0))
	w := app.NewWindow("clip")
	defer w.Close()
	w.SetContent(container.NewBorder(nil, nil, sidebar, nil, d.Content()))
	w.Resize(fyne.NewSize(980, 600))
	w.Show()
	s.SelectedCard = len(s.Worktrees) - 1
	d.Rebuild()
	d.EnsureVisible()
	if d.stageScrolls[0].Visible() {
		t.Fatal("fully offscreen stage retains an active viewport")
	}
	if !d.stageScrolls[1].Visible() || d.stageScrolls[1].Size().Width >= d.stageViewports[1].Size().Width {
		t.Fatal("partly visible stage was not intersected")
	}
	if d.stageScrolls[4].Size().Width != d.stageViewports[4].Size().Width {
		t.Fatal("fully visible stage was reduced")
	}
	capture := w.Canvas().Capture()
	wantR, wantG, wantB, _ := colorPanelBg.RGBA()
	for y := 200; y < 550; y += 7 {
		for x := 8; x < 174; x += 7 {
			r, g, b, _ := capture.At(x, y).RGBA()
			if r != wantR || g != wantG || b != wantB {
				t.Fatalf("board content painted into sidebar at (%d,%d)", x, y)
			}
		}
	}
	test.TapCanvas(w.Canvas(), fyne.NewPos(100, 300))
	if s.SelectedCard != len(s.Worktrees)-1 {
		t.Fatal("hidden column intercepted a sidebar click")
	}
	// The first card of the partially shown Created stage is still tappable.
	pos := app.Driver().AbsolutePositionForObject(d.stageScrolls[1])
	test.TapCanvas(w.Canvas(), fyne.NewPos(pos.X+12, pos.Y+12))
	if s.SelectedCard != 1 {
		t.Fatalf("partial stage click selected %d, want card 1", s.SelectedCard)
	}
	w.Resize(fyne.NewSize(1500, 600))
	if d.boardScroll.Offset.X != 0 {
		t.Fatal("wider window did not clamp board offset")
	}
	for i, scroll := range d.stageScrolls {
		if !scroll.Visible() || scroll.Offset.X != 0 || scroll.Size().Width != d.stageViewports[i].Size().Width {
			t.Fatalf("stage %d did not restore its full viewport after resize", i)
		}
	}
}

// Hover feedback never creates automatic surfaces or captures canvas routing.
func TestDashboardHoverDoesNotCreateSurfacesOrConsumeClicks(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	defer applyDarkPalette()
	s := polishState()
	s.PRs[s.Worktrees[1].Branch] = &provider.PRInfo{State: "open", ReviewStatus: "approved", CheckStatus: "failure"}
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 1100, 650)
	root := w.Canvas().Content()
	refreshes := 0
	d.OnRefresh = func() { refreshes++ }
	var objects []fyne.CanvasObject
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		switch o.(type) {
		case *actionControl, *measuredText, *statusText:
			objects = append(objects, o)
		}
	})
	for _, obj := range objects {
		pos := app.Driver().AbsolutePositionForObject(obj).Add(fyne.NewPos(8, 8))
		test.MoveMouse(w.Canvas(), pos)
		test.MoveMouse(w.Canvas(), pos.Add(fyne.NewPos(1, 0)))
		if w.Canvas().Overlays().Top() != nil || w.Canvas().Content() != root || w.Canvas().Focused() != nil {
			t.Fatalf("hover over %T introduced a surface or changed routing", obj)
		}
	}
	refresh := polishAction(t, d, "Refresh")
	pos := app.Driver().AbsolutePositionForObject(refresh).Add(fyne.NewPos(8, 8))
	test.MoveMouse(w.Canvas(), pos)
	if !refresh.hovered || refresh.bg.StrokeWidth == 0 {
		t.Fatal("ordinary hover feedback disappeared")
	}
	test.TapCanvas(w.Canvas(), pos)
	if refreshes != 1 {
		t.Fatal("hover consumed the refresh click")
	}
}

func TestMainDetailsExposesFullTechnicalValuesWithoutFocus(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	s := polishState()
	s.Worktrees[0].Branch = "main/" + strings.Repeat("long-branch/", 14)
	s.Worktrees[0].Path = "/projects/" + strings.Repeat("nested-directory/", 14)
	d := NewDashboard(s)
	w := polishWindow(t, app, d, 900, 640)
	test.Tap(polishAction(t, d, "Details"))
	foundBranch, foundPath := false, false
	walkPolish(d.Content(), func(obj fyne.CanvasObject) {
		if label, ok := obj.(*widget.Label); ok && label.TextStyle.Monospace && label.Wrapping == fyne.TextWrapBreak {
			foundBranch = foundBranch || label.Text == s.Worktrees[0].Branch
			foundPath = foundPath || label.Text == s.Worktrees[0].Path
		}
		if _, ok := obj.(fyne.Focusable); ok {
			t.Fatalf("Details introduced focusable %T", obj)
		}
	})
	if !foundBranch || !foundPath || w.Canvas().Focused() != nil {
		t.Fatal("Details did not expose passive full branch/path")
	}
}

func TestRefreshRevealsSelectedCardWhenPRMovesStage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	s := polishState()
	s.SelectedCard = 1
	d := NewDashboard(s)
	polishWindow(t, app, d, 720, 440)
	d.EnsureVisible()
	before := d.boardScroll.Offset.X
	prs := provider.PRResult{s.Worktrees[1].Branch: &provider.PRInfo{State: "merged"}}
	if !d.ApplyRefresh(ops.RefreshResult{HasPRs: true, PRs: prs}) {
		t.Fatal("network refresh was rejected")
	}
	column := d.boardColumns[4]
	if s.SelectedCard != 1 || d.boardScroll.Offset.X <= before || column.Position().X < d.boardScroll.Offset.X || column.Position().X+column.Size().Width > d.boardScroll.Offset.X+d.boardScroll.Size().Width+1 {
		t.Fatal("PR stage update left the selected worktree outside the viewport")
	}
}

func TestRefreshDoesNotRevealSelectionUserScrolledAwayFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantDark))
	s := polishState()
	s.SelectedCard = 1
	d := NewDashboard(s)
	polishWindow(t, app, d, 720, 440)
	d.boardScroll.ScrollToOffset(fyne.NewPos(d.boardScroll.Content.MinSize().Width, 0))
	d.stageScrolls[1].ScrollToBottom()
	before := d.boardScroll.Offset.X
	if d.selectedCardVisible() {
		t.Fatalf("fixture did not scroll away from selection: board offset=%v viewport=%v content=%v min=%v column=%v/%v", d.boardScroll.Offset, d.boardScroll.Size(), d.boardScroll.Content.Size(), d.boardScroll.Content.MinSize(), d.boardColumns[1].Position(), d.boardColumns[1].Size())
	}
	prs := provider.PRResult{s.Worktrees[1].Branch: &provider.PRInfo{State: "closed"}}
	if !d.ApplyRefresh(ops.RefreshResult{HasPRs: true, PRs: prs}) {
		t.Fatal("network refresh was rejected")
	}
	if d.boardScroll.Offset.X != before {
		t.Fatal("PR stage update interrupted manual browsing")
	}
}
