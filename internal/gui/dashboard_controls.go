package gui

import (
	"image/color"
	"runtime"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/provider"
)

// actionControl deliberately implements tap and pointer interfaces only: a
// dashboard action must never capture keyboard focus from the canvas router.
type actionControl struct {
	widget.BaseWidget
	label                                         string
	keyHint                                       string
	icon                                          fyne.Resource
	onTap                                         func()
	primary, selected, disabled, hovered, pressed bool
	bg                                            *canvas.Rectangle
	content                                       *fyne.Container
}

func newActionControl(label string, icon fyne.Resource, primary bool, onTap func()) *actionControl {
	c := &actionControl{label: label, icon: icon, primary: primary, onTap: onTap}
	c.ExtendBaseWidget(c)
	return c
}
func (c *actionControl) Tapped(_ *fyne.PointEvent) {
	if !c.disabled && c.onTap != nil {
		c.onTap()
	}
}
func (c *actionControl) MouseIn(_ *desktop.MouseEvent) {
	c.hovered = true
	c.Refresh()
}
func (c *actionControl) MouseMoved(_ *desktop.MouseEvent) {}
func (c *actionControl) MouseOut() {
	c.hovered = false
	c.pressed = false
	c.Refresh()
}
func (c *actionControl) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonPrimary && !c.disabled {
		c.pressed = true
		c.Refresh()
	}
}
func (c *actionControl) MouseUp(_ *desktop.MouseEvent) { c.pressed = false; c.Refresh() }
func (c *actionControl) CreateRenderer() fyne.WidgetRenderer {
	c.bg = canvas.NewRectangle(colorSecondaryBg)
	c.bg.CornerRadius = scaledSize(radiusControl)
	var parts []fyne.CanvasObject
	if c.icon != nil {
		resource := c.icon
		if c.primary {
			resource = theme.NewColoredResource(resource, theme.ColorNameForegroundOnPrimary)
		}
		i := widget.NewIcon(resource)
		parts = append(parts, shellIcon(i, 14))
	}
	text := shortcutLabel(c.label, c.keyHint)
	label := uiText(text, colorForeground, c.primary)
	label.TextSize = scaledSize(12)
	parts = append(parts, label)
	c.content = container.NewHBox(parts...)
	c.updateColors()
	return widget.NewSimpleRenderer(container.NewStack(c.bg, inset(c.content, spaceSM, 5)))
}
func (c *actionControl) Refresh() { c.updateColors(); c.BaseWidget.Refresh() }
func (c *actionControl) updateColors() {
	if c.bg == nil {
		return
	}
	c.bg.FillColor = colorSecondaryBg
	c.bg.StrokeWidth = 0
	if c.selected {
		c.bg.FillColor = colorSelection
	}
	if c.primary {
		c.bg.FillColor = colorActionBg
	}
	if c.hovered && !c.disabled {
		c.bg.StrokeColor = colorSelected
		c.bg.StrokeWidth = 1
	}
	if c.pressed {
		if !c.primary {
			c.bg.FillColor = colorHover
		}
		c.bg.StrokeWidth = 2
	}
	if c.disabled {
		c.bg.FillColor = colorPanelBg
		c.bg.StrokeWidth = 0
	}
	if len(c.content.Objects) > 0 {
		if t, ok := c.content.Objects[len(c.content.Objects)-1].(*canvas.Text); ok {
			t.Color = colorForeground
			if c.primary {
				t.Color = colorOnAction
			}
			if c.disabled {
				t.Color = colorDimGray
			}
			t.Refresh()
		}
	}
	c.bg.Refresh()
}

// measuredText measures the actual selected font when truncating technical
// or proportional labels. Full values belong in explicit detail views.
type measuredText struct {
	widget.BaseWidget
	full   string
	prefix bool
	txt    *canvas.Text
}

func newMeasuredText(text string, c color.Color, bold, mono, prefix bool) *measuredText {
	t := uiText(text, c, bold)
	t.TextStyle.Monospace = mono
	m := &measuredText{full: text, prefix: prefix, txt: t}
	m.ExtendBaseWidget(m)
	return m
}
func (m *measuredText) CreateRenderer() fyne.WidgetRenderer { return &measuredTextRenderer{m: m} }

type measuredTextRenderer struct{ m *measuredText }

func (r *measuredTextRenderer) Layout(size fyne.Size) {
	r.m.txt.Text = fitText(r.m.full, size.Width, r.m.txt.TextSize, r.m.txt.TextStyle, r.m.prefix)
	r.m.txt.Resize(size)
	r.m.txt.Refresh()
}
func (r *measuredTextRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, r.m.txt.MinSize().Height) }
func (r *measuredTextRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.m.txt} }
func (r *measuredTextRenderer) Refresh()                     { r.Layout(r.m.Size()) }
func (r *measuredTextRenderer) Destroy()                     {}
func fitText(full string, width, size float32, style fyne.TextStyle, prefix bool) string {
	if fyne.MeasureText(full, size, style).Width <= width {
		return full
	}
	if fyne.MeasureText("…", size, style).Width > width {
		return ""
	}
	runes := []rune(full)
	lo, hi := 0, utf8.RuneCountInString(full)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		s := string(runes[:mid]) + "…"
		if prefix {
			s = "…" + string(runes[len(runes)-mid:])
		}
		if fyne.MeasureText(s, size, style).Width <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if prefix {
		return "…" + string(runes[len(runes)-lo:])
	}
	return string(runes[:lo]) + "…"
}

type insetLayout struct{ x, y float32 }

func inset(obj fyne.CanvasObject, x, y float32) *fyne.Container {
	return container.New(&insetLayout{scaledSize(x), scaledSize(y)}, obj)
}
func (p *insetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return objects[0].MinSize().Add(fyne.NewSize(2*p.x, 2*p.y))
}
func (p *insetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.NewPos(p.x, p.y))
	objects[0].Resize(fyne.NewSize(max(0, size.Width-2*p.x), max(0, size.Height-2*p.y)))
}

// boardLayout enforces readable columns while stretching their height to the
// viewport. Horizontal scrolling handles windows narrower than the board.
type boardLayout struct{}

func (*boardLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(float32(len(objects))*scaledSize(212)+float32(len(objects)-1)*scaledSize(spaceSM), scaledSize(120))
}
func (b *boardLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	gap := scaledSize(spaceSM)
	width := max(scaledSize(212), (size.Width-float32(len(objects)-1)*gap)/float32(len(objects)))
	for i, obj := range objects {
		obj.Move(fyne.NewPos(float32(i)*(width+gap), 0))
		obj.Resize(fyne.NewSize(width, size.Height))
	}
}

// wireDashboardActions resolves the active repository on every click. A
// callback from an old dashboard is ignored after a repository switch.
func (a *App) wireDashboardActions(d *Dashboard) {
	run := func(main bool, fn func()) {
		re := a.activeRepo()
		if a.dialogOpen || re == nil || re.dashboard != d || a.dashboard != d {
			return
		}
		a.focus = focusRight
		a.updatePanelFocus()
		if main {
			re.state.SelectedCard = 0
			d.Rebuild()
		}
		fn()
	}
	d.OnCreate = func() { run(true, a.handleCreate) }
	d.OnCreateFromIssue = func() { run(true, a.handleCreateFromIssue) }
	d.OnFetchPR = func() {
		if d.state.Provider == provider.ProviderGitHub {
			run(true, a.handleFetchPR)
		}
	}
	d.OnRefresh = func() { run(false, a.handleRefresh) }
	d.OnMainTerminal = func() { run(true, a.handleEnter) }
	d.OnMainEditor = func() { run(true, a.handleOpenEditor) }
	d.OnMainNotes = func() { run(true, a.handleEditNote) }
	d.OnOpenTerminal = func() { run(false, a.handleEnter) }
	d.OnOpenEditor = func() { run(false, a.handleOpenEditor) }
	d.OnEditNotes = func() { run(false, a.handleEditNote) }
	d.OnActivity = func() { run(false, a.handleRegentLog) }
	d.OnPull = func() { run(false, a.handlePull) }
	d.OnSendPR = func() { run(false, a.handleSendPR) }
	d.OnDelete = func() { run(false, a.handleDeleteOrRemoveSandbox) }
	d.OnStartSandbox = func() { run(false, a.handleStartSandbox) }
	d.OnStopSandbox = func() { run(false, a.handleStopSandbox) }
	d.OnSetupSandbox = func() { run(true, a.handleCreateOrEnrollSandbox) }
	d.OnViewChanged = func(v ViewMode) {
		run(false, func() {
			if v < ViewKanban || v > ViewList {
				return
			}
			if d.state.ViewMode != v {
				d.state.ViewMode = v
				d.Rebuild()
			}
		})
	}
	d.OnCardSelected = func(_ int) {
		if a.dashboard == d {
			a.focus = focusRight
			a.updatePanelFocus()
		}
	}
	d.OnNoteRequested = func(wt git.Worktree) { run(false, func() { a.openNoteDialog(wt) }) }
	d.Rebuild()
}

var _ desktop.Hoverable = (*actionControl)(nil)
var _ desktop.Mouseable = (*actionControl)(nil)
var _ fyne.Tappable = (*actionControl)(nil)

// Fyne 2.7 replaces an outer scroll clip when walking an inner scroll.
// Intersect each stage's viewport ourselves, retaining its natural content
// width, so horizontally scrolled cards cannot paint into the sidebar.
type stageViewport struct {
	widget.BaseWidget
	dash   *Dashboard
	stage  int
	scroll *container.Scroll
	body   *fyne.Container
}

func newStageViewport(d *Dashboard, stage int, scroll *container.Scroll, body *fyne.Container) *stageViewport {
	v := &stageViewport{dash: d, stage: stage, scroll: scroll, body: body}
	v.ExtendBaseWidget(v)
	return v
}
func (v *stageViewport) CreateRenderer() fyne.WidgetRenderer { return &stageViewportRenderer{v: v} }

type stageViewportRenderer struct{ v *stageViewport }

func (r *stageViewportRenderer) Layout(size fyne.Size) {
	v := r.v
	v.body.Layout.(*stageBodyLayout).width = size.Width
	left, right := float32(0), size.Width
	if v.dash.boardScroll != nil && len(v.dash.boardColumns) > v.stage {
		x := v.dash.boardColumns[v.stage].Position().X + scaledSize(spaceSM) - v.dash.boardScroll.Offset.X
		left = max(float32(0), -x)
		right = min(size.Width, v.dash.boardScroll.Size().Width-x)
	}
	if right <= left {
		v.scroll.Hide()
		return
	}
	v.scroll.Show()
	v.scroll.Move(fyne.NewPos(left, 0))
	v.scroll.Resize(fyne.NewSize(right-left, size.Height))
	v.body.Layout.Layout(v.body.Objects, v.body.Size())
	// Scroll.Refresh also recursively refreshes Content. Its base updates bars
	// and layout only, which is sufficient for the changed viewport geometry.
	v.scroll.Base.Refresh()
	v.scroll.ScrollToOffset(fyne.NewPos(left, v.scroll.Offset.Y))
}
func (r *stageViewportRenderer) MinSize() fyne.Size           { return fyne.NewSize(1, 32) }
func (r *stageViewportRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.v.scroll} }
func (r *stageViewportRenderer) Refresh()                     { r.Layout(r.v.Size()) }
func (r *stageViewportRenderer) Destroy()                     {}

type stageBodyLayout struct{ width float32 }

func (l *stageBodyLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(max(float32(1), l.width), objects[0].MinSize().Height)
}
func (l *stageBodyLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(max(l.width, size.Width), size.Height))
}

type boardViewport struct {
	widget.BaseWidget
	dash *Dashboard
}

func newBoardViewport(d *Dashboard) *boardViewport {
	v := &boardViewport{dash: d}
	v.ExtendBaseWidget(v)
	return v
}
func (v *boardViewport) CreateRenderer() fyne.WidgetRenderer { return &boardViewportRenderer{v: v} }

type boardViewportRenderer struct{ v *boardViewport }

func (r *boardViewportRenderer) Layout(size fyne.Size) {
	r.v.dash.boardScroll.Move(fyne.Position{})
	r.v.dash.boardScroll.Resize(size)
	// Also update the renderer's content pointer after a dashboard rebuild,
	// where the viewport size may be unchanged.
	r.v.dash.boardScroll.Base.Refresh()
	r.v.dash.refreshStageViewports()
}
func (r *boardViewportRenderer) MinSize() fyne.Size { return r.v.dash.boardScroll.MinSize() }
func (r *boardViewportRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.v.dash.boardScroll}
}
func (r *boardViewportRenderer) Refresh() { r.Layout(r.v.Size()) }
func (r *boardViewportRenderer) Destroy() {}
func (d *Dashboard) refreshStageViewports() {
	for _, v := range d.stageViewports {
		if v != nil {
			v.Refresh()
		}
	}
}

// shortcutLabel separates an action from its keyboard hint on every surface.
func shortcutLabel(action, key string) string {
	if key == "" {
		return action
	}
	return action + " [" + key + "]"
}

func platformShortcut(key string) string {
	if runtime.GOOS == "darwin" {
		return "Cmd+" + key
	}
	return "Ctrl+" + key
}
