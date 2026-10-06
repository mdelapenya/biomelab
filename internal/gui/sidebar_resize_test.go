package gui

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/config"
)

func resizeSidebarFixture(t *testing.T, width, height float32) (*App, fyne.Window) {
	t.Helper()
	fa := test.NewApp()
	t.Cleanup(fa.Quit)
	t.Cleanup(applyDarkPalette)
	th := newBiomeTheme(VariantLight)
	fa.Settings().SetTheme(th)
	s := workspaceState()
	d := NewDashboard(s)
	rp := NewRepoPanel([]*RepoGroup{
		{Name: "biomelab", Modes: []config.ModeEntry{{Type: "regular"}}, LinkedWorktreeCount: 5},
		{Name: "agent-tools", Modes: []config.ModeEntry{{Type: "regular"}}},
	}, nil)
	w := fa.NewWindow("Sidebar resize")
	t.Cleanup(w.Close)
	w.SetPadded(false)
	a := &App{fyneApp: fa, window: w, theme: th, dashboard: d, repoPanel: rp,
		dashSlot: container.NewStack(d.Content()), shellSlot: container.NewStack(), sysdepsBanner: container.NewVBox(),
		repos: []*repoEntry{{state: s, dashboard: d, group: rp.groups[0]}}}
	a.sysdepsBanner.Hide()
	a.wireDashboardActions(d)
	a.refreshShellLayout()
	w.SetContent(a.shellSlot)
	w.Resize(fyne.NewSize(width, height))
	w.Show()
	return a, w
}

func resizeDivider(t *testing.T, a *App) *shellDivider {
	t.Helper()
	var divider *shellDivider
	walkPolish(a.shellSlot, func(o fyne.CanvasObject) {
		if d, ok := o.(*shellDivider); ok {
			divider = d
		}
	})
	if divider == nil {
		t.Fatal("shell has no draggable divider")
	}
	return divider
}

func dragSidebar(t *testing.T, a *App, dx float32, rightEdge bool) {
	t.Helper()
	divider := resizeDivider(t, a)
	x := float32(1)
	if rightEdge {
		x = divider.Size().Width - 1
	}
	// Driver positions exclude the interactive-area inset; test.Drag expects
	// canvas coordinates, including that inset even in an unpadded window.
	inset, _ := a.window.Canvas().InteractiveArea()
	pos := a.fyneApp.Driver().AbsolutePositionForObject(divider).Add(inset).Add(fyne.NewPos(x, 150))
	test.Drag(a.window.Canvas(), pos, dx, 0)
}

func assertSidebarWidth(t *testing.T, a *App, want float32) {
	t.Helper()
	if got := a.repoPanel.Content().Size().Width; math.Abs(float64(got-want)) > 0.1 {
		t.Fatalf("sidebar width=%v, want=%v", got, want)
	}
}

func TestSidebarCanvasDragResizesWithoutReorderingOrFocus(t *testing.T) {
	a, w := resizeSidebarFixture(t, 1100, 740)
	assertSidebarWidth(t, a, 190)
	from, to := -1, -1
	a.repoPanel.OnReorder = func(f, dest int) { from, to = f, dest }
	dragSidebar(t, a, 110, false)
	assertSidebarWidth(t, a, 300)
	// The right half extends into the workspace: both sides of the wider hit
	// target must deliver the drag to the divider.
	dragSidebar(t, a, 30, true)
	assertSidebarWidth(t, a, 330)
	if from != -1 || to != -1 || a.sidebarWidth != 330 {
		t.Fatalf("resize reordered projects or lost preference: (%d,%d), width=%v", from, to, a.sidebarWidth)
	}
	var handle *dragHandle
	walkPolish(a.repoPanel.Content(), func(o fyne.CanvasObject) {
		if h, ok := o.(*dragHandle); ok && h.groupIdx == 1 {
			handle = h
		}
	})
	if handle == nil {
		t.Fatal("repository reorder handle missing")
	}
	inset, _ := w.Canvas().InteractiveArea()
	pos := a.fyneApp.Driver().AbsolutePositionForObject(handle).Add(inset).Add(fyne.NewPos(3, 3))
	test.Drag(w.Canvas(), pos, 0, -200)
	if from != 1 || to != 0 {
		t.Fatalf("divider prevented repository reorder: (%d,%d)", from, to)
	}
	assertSidebarWidth(t, a, 330)
	// Dragging to either bound keeps both panels usable.
	dragSidebar(t, a, -1000, false)
	assertSidebarWidth(t, a, 160)
	dragSidebar(t, a, 10000, true)
	if a.dashSlot.Size().Width < a.dashSlot.MinSize().Width {
		t.Fatal("divider left workspace below its minimum")
	}
	walkPolish(w.Content(), func(o fyne.CanvasObject) {
		if _, ok := o.(fyne.Focusable); ok {
			t.Errorf("shell contains Focusable %T", o)
		}
	})
	c := &keyboardCanvas{Canvas: w.Canvas()}
	setupKeyHandlers(c, a.handleKeyName, a.handleRune)
	c.press(fyne.KeyTab)
	c.press(fyne.KeyG)
	if w.Canvas().Focused() != nil || a.dashboard.state.ViewMode != ViewGrid || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("resizing took keyboard focus or opened an overlay")
	}
}

func TestSidebarPreferenceSurvivesRebuildZoomAndWindowClamp(t *testing.T) {
	a, w := resizeSidebarFixture(t, 1100, 740)
	dragSidebar(t, a, 170, false)
	assertSidebarWidth(t, a, 360)
	for _, view := range []ViewMode{ViewList, ViewGrid, ViewKanban} {
		a.dashboard.setView(view)
		a.dashboard.inspectorOpen = true
		a.dashboard.Rebuild()
		a.repoPanel.RebuildFull()
		a.refreshShellLayout()
		assertSidebarWidth(t, a, 360)
	}
	a.theme.SetVariant(VariantDark)
	a.fyneApp.Settings().SetTheme(a.theme)
	a.repoPanel.RebuildFull()
	a.dashboard.Rebuild()
	a.refreshShellLayout()
	assertSidebarWidth(t, a, 360)
	a.theme.ZoomIn()
	a.theme.ZoomIn()
	a.fyneApp.Settings().SetTheme(a.theme)
	a.repoPanel.RebuildFull()
	a.dashboard.Rebuild()
	a.refreshShellLayout()
	assertSidebarWidth(t, a, scaledSize(360))
	if a.sidebarWidth != 360 {
		t.Fatal("zoom overwrote logical sidebar preference")
	}
	w.Resize(fyne.NewSize(650, 740))
	if a.repoPanel.Content().Size().Width >= scaledSize(360) || a.dashSlot.Size().Width < a.dashSlot.MinSize().Width || a.sidebarWidth != 360 {
		t.Fatalf("window clamp lost preference or minimum: sidebar=%v workspace=%v preference=%v", a.repoPanel.Content().Size(), a.dashSlot.Size(), a.sidebarWidth)
	}
	w.Resize(fyne.NewSize(1440, 740))
	assertSidebarWidth(t, a, scaledSize(360))
	a.theme.ZoomReset()
	a.fyneApp.Settings().SetTheme(a.theme)
	a.repoPanel.RebuildFull()
	a.dashboard.Rebuild()
	a.refreshShellLayout()
	assertSidebarWidth(t, a, 360)
}

func TestSidebarResizingKeepsWrappedToolbarRowsSeparate(t *testing.T) {
	a, _ := resizeSidebarFixture(t, 900, 740)
	dragSidebar(t, a, 130, false)
	a.dashboard.setView(ViewList)
	for _, zoom := range []bool{false, true} {
		if zoom {
			a.theme.ZoomIn()
			a.fyneApp.Settings().SetTheme(a.theme)
			a.dashboard.Rebuild()
			a.refreshShellLayout()
		}
		root := a.dashboard.innerSlot.Objects[0].(*fyne.Container)
		header := root.Objects[0].(*fyne.Container)
		title := header.Objects[0]
		if header.Objects[1].Position().Y < title.Position().Y+title.MinSize().Height || title.Size().Height < title.MinSize().Height {
			t.Fatal("resized workspace toolbar overlaps repository/mode context")
		}
	}
}

// Captures use the real shell, pointer drag, repository rail and dashboard.
func TestSidebarResizeProductionArtifacts(t *testing.T) {
	dir := os.Getenv("BIOMELAB_SIDEBAR_IMAGES")
	if dir == "" {
		t.Skip("set BIOMELAB_SIDEBAR_IMAGES for production captures")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a, w := resizeSidebarFixture(t, 1440, 900)
	savedIcon := AppIcon
	t.Cleanup(func() { AppIcon = savedIcon })
	data, err := os.ReadFile("../../cmd/biomelab/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	AppIcon = fyne.NewStaticResource("icon.png", data)
	dragSidebar(t, a, 130, false)
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		a.theme.SetVariant(variant)
		a.fyneApp.Settings().SetTheme(a.theme)
		a.repoPanel.RebuildFull()
		a.refreshShellLayout()
		for _, scenario := range []struct {
			name          string
			width, height float32
			view          ViewMode
		}{{"wide-board", 1440, 900, ViewKanban}, {"narrow-list", 900, 740, ViewList}} {
			w.Resize(fyne.NewSize(scenario.width, scenario.height))
			a.dashboard.setView(scenario.view)
			a.dashboard.Rebuild()
			f, err := os.Create(filepath.Join(dir, string(variant)+"-"+scenario.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, w.Canvas().Capture())
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
	}
}
