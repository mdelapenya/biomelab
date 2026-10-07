package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

type resizeRefreshProbe struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	refreshes int
}

func (p *resizeRefreshProbe) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}
func (p *resizeRefreshProbe) Refresh() { p.refreshes++; p.BaseWidget.Refresh() }

func TestSidebarDragRelayoutsWithoutRefreshingOrRebuildingWorkspace(t *testing.T) {
	a, w := resizeSidebarFixture(t, 1100, 740)
	probe := &resizeRefreshProbe{content: a.dashboard.Content()}
	probe.ExtendBaseWidget(probe)
	a.dashSlot.Objects = []fyne.CanvasObject{probe}
	a.dashSlot.Refresh()
	selected := a.dashboard.state.SelectedCard
	for _, view := range []ViewMode{ViewKanban, ViewList, ViewGrid} {
		a.dashboard.setView(view)
		root := a.dashboard.innerSlot.Objects[0]
		probe.refreshes = 0
		for i := 0; i < 10; i++ {
			delta := float32(1)
			if i%2 != 0 {
				delta = -1
			}
			dragSidebar(t, a, delta, i%2 == 0)
		}
		if probe.refreshes != 0 || a.dashboard.innerSlot.Objects[0] != root || a.dashboard.state.SelectedCard != selected {
			t.Fatal("pure pointer resize refreshed/rebuilt workspace content or lost selection")
		}
		assertSidebarWidth(t, a, 190)
		w.Resize(fyne.NewSize(1110, 740))
		if probe.refreshes != 0 {
			t.Fatal("window resize recursively refreshed workspace content")
		}
		w.Resize(fyne.NewSize(1100, 740))
	}
}

func TestWrappedValuesReflowForTextStyleWidthAndZoom(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	t.Cleanup(applyDarkPalette)
	th := newBiomeTheme(VariantLight)
	fa.Settings().SetTheme(th)
	w := newWrappedValue("/projects/biomelab-worktrees/fix-authentication-timeout-東京", true)
	check := func(width float32) []string {
		t.Helper()
		lines := w.lines(width)
		strip := func(s string) string { return strings.ReplaceAll(s, " ", "") }
		if strip(strings.Join(lines, "")) != strip(w.value) {
			t.Fatal("wrapping lost or reused stale text")
		}
		for _, line := range lines {
			if fyne.MeasureText(line, scaledSize(12), fyne.TextStyle{Monospace: w.mono}).Width > width {
				t.Fatalf("line overflowed after reflow: %q", line)
			}
		}
		return lines
	}
	if len(check(120)) <= len(check(360)) {
		t.Fatal("narrowing failed to wrap technical value")
	}
	w.value = "Enter Terminal · e Editor · m Notes · Ctrl/Cmd+I Inspector"
	w.mono = false
	check(180)
	th.ZoomIn()
	th.ZoomIn()
	fa.Settings().SetTheme(th)
	check(180)
	th.SetVariant(VariantDark)
	fa.Settings().SetTheme(th)
	check(180)
}
