package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

// Fyne's test canvas does not expose desktop key callbacks. This adapter uses
// its real focus manager and reproduces the native GLFW press dispatch order:
// focused widgets receive keys instead of the canvas, then Tab advances focus.
type keyboardCanvas struct {
	fyne.Canvas
	down, up  func(*fyne.KeyEvent)
	modifiers fyne.KeyModifier
}

func (c *keyboardCanvas) OnKeyDown() func(*fyne.KeyEvent)     { return c.down }
func (c *keyboardCanvas) SetOnKeyDown(f func(*fyne.KeyEvent)) { c.down = f }
func (c *keyboardCanvas) OnKeyUp() func(*fyne.KeyEvent)       { return c.up }
func (c *keyboardCanvas) SetOnKeyUp(f func(*fyne.KeyEvent))   { c.up = f }
func (c *keyboardCanvas) press(name fyne.KeyName) {
	event := &fyne.KeyEvent{Name: name}
	if focused := c.Focused(); focused != nil {
		if keyable, ok := focused.(desktop.Keyable); ok {
			keyable.KeyDown(event)
		}
	} else if c.down != nil {
		c.down(event)
	}
	if c.modifiers != 0 && c.modifiers != fyne.KeyModifierShift {
		var shortcut fyne.Shortcut = &desktop.CustomShortcut{KeyName: name, Modifier: c.modifiers}
		if name == fyne.KeyA && c.modifiers == fyne.KeyModifierShortcutDefault {
			shortcut = &fyne.ShortcutSelectAll{}
		}
		if focused, ok := c.Focused().(fyne.Shortcutable); ok {
			if disabled, ok := focused.(interface {
				fyne.Disableable
				SelectedText() string
			}); ok && disabled.Disabled() && shortcut.ShortcutName() != "Copy" {
				return
			}
			focused.TypedShortcut(shortcut)
		} else {
			c.Canvas.(fyne.Shortcutable).TypedShortcut(shortcut)
		}
		return
	}
	if name == fyne.KeyTab {
		c.FocusNext()
		return
	}
	if focused := c.Focused(); focused != nil {
		focused.TypedKey(event)
	} else if typed := c.OnTypedKey(); typed != nil {
		typed(event)
	}
}

func TestWorkspaceViewAndInspectorKeyboardEvents(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantDark))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	d := NewDashboard(s)
	w := polishWindow(t, fa, d, 1000, 740)
	a := &App{fyneApp: fa, window: w, dashboard: d, repos: []*repoEntry{{state: s, dashboard: d}}}
	a.wireDashboardActions(d)
	c := &keyboardCanvas{Canvas: w.Canvas()}
	setupKeyHandlersWithModifiers(c, a.handleKeyName, a.handleRune, func() fyne.KeyModifier { return c.modifiers })
	registerInspectorShortcuts(c, a.toggleInspector)
	selected := s.SelectedCard
	for _, view := range []ViewMode{ViewList, ViewGrid, ViewKanban} {
		c.press(fyne.KeyV)
		if s.ViewMode != view || s.SelectedCard != selected {
			t.Fatalf("v selected view=%v card=%d, want view=%v card=%d", s.ViewMode, s.SelectedCard, view, selected)
		}
	}
	c.press(fyne.KeyG)
	if s.ViewMode != ViewGrid {
		t.Fatal("g lost Board/Grid binding")
	}
	c.press(fyne.KeyG)
	if s.ViewMode != ViewKanban {
		t.Fatal("g lost return to Board")
	}
	for _, selected := range []int{0, 2} {
		s.SelectedCard = selected
		for _, modifier := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
			open := d.inspectorOpen
			c.modifiers = modifier
			c.press(fyne.KeyI)
			if d.inspectorOpen == open || s.SelectedCard != selected || a.issueFlow != nil || a.dialogOpen {
				t.Fatal("inspector chord invoked plain i, lost selection, or failed to toggle")
			}
		}
	}
	c.modifiers = 0
	c.press(fyne.KeyTab)
	open := d.inspectorOpen
	c.modifiers = fyne.KeyModifierSuper
	c.press(fyne.KeyI)
	if d.inspectorOpen != open {
		t.Fatal("inspector toggled in Projects context")
	}
	c.modifiers = 0
	c.press(fyne.KeyTab)
	a.dialogOpen = true
	c.press(fyne.KeyV)
	c.modifiers = fyne.KeyModifierControl
	c.press(fyne.KeyI)
	if s.ViewMode != ViewKanban || d.inspectorOpen != open {
		t.Fatal("workspace keys operated beneath modal")
	}
	if c.Focused() != nil {
		t.Fatalf("workspace captured focus: %T", c.Focused())
	}
}

func TestInspectorChordPreservesPlainIssueKeyAndDialogInput(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	w := a.window
	w.SetContent(a.dashboard.Content())
	w.Show()
	a.wireDashboardActions(a.dashboard)
	c := &keyboardCanvas{Canvas: w.Canvas()}
	setupKeyHandlersWithModifiers(c, a.handleKeyName, a.handleRune, func() fyne.KeyModifier { return c.modifiers })
	registerInspectorShortcuts(c, a.toggleInspector)
	c.modifiers = fyne.KeyModifierSuper
	c.press(fyne.KeyI)
	if a.issueFlow != nil || a.dialogOpen {
		t.Fatal("Cmd+I opened issue creation")
	}
	c.modifiers = 0
	c.press(fyne.KeyI)
	if a.issueFlow == nil || !a.dialogOpen || re.state.SelectedCard != 0 {
		t.Fatal("plain i lost existing issue flow")
	}
	var entry *dialogEntry
	walkPolish(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if e, ok := o.(*dialogEntry); ok {
			entry = e
		}
	})
	if entry == nil {
		t.Fatal("issue entry missing")
	}
	entry.SetText("82")
	w.Canvas().Focus(entry)
	c.modifiers = fyne.KeyModifierShortcutDefault
	c.press(fyne.KeyA)
	if entry.SelectedText() != "82" {
		t.Fatal("dialog select-all shortcut was swallowed")
	}
	open := a.dashboard.inspectorOpen
	c.press(fyne.KeyI)
	if !a.dialogOpen || a.dashboard.inspectorOpen != open {
		t.Fatal("inspector chord changed active modal")
	}
	c.modifiers = 0
	c.press(fyne.KeyEscape)
	if a.dialogOpen || w.Canvas().Focused() != nil {
		t.Fatal("Escape did not restore canvas routing")
	}
}

func TestModifierRoutingPreservesThemeZoomAndShiftRunes(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	th := newBiomeTheme(VariantDark)
	fa.Settings().SetTheme(th)
	t.Cleanup(applyDarkPalette)
	w := fa.NewWindow("keys")
	t.Cleanup(w.Close)
	c := &keyboardCanvas{Canvas: w.Canvas()}
	var keys []fyne.KeyName
	var runes []rune
	setupKeyHandlersWithModifiers(c, func(k fyne.KeyName) { keys = append(keys, k) }, func(r rune) { runes = append(runes, r) }, func() fyne.KeyModifier { return c.modifiers })
	themes, zooms := 0, 0
	registerThemeToggleShortcut(c, func() { themes++ })
	registerZoomShortcuts(c, th, fa, func() { zooms++ })
	for _, mod := range []fyne.KeyModifier{fyne.KeyModifierControl, fyne.KeyModifierSuper} {
		c.modifiers = mod
		c.press(fyne.KeyT)
		c.press(fyne.KeyEqual)
		c.press(fyne.KeyMinus)
		c.press(fyne.Key0)
	}
	if themes != 2 || zooms != 6 || len(keys) != 0 {
		t.Fatalf("modifier commands leaked plain keys or lost shortcuts: themes=%d zooms=%d keys=%v", themes, zooms, keys)
	}
	c.modifiers = fyne.KeyModifierShift
	c.press(fyne.KeyP)
	c.OnTypedRune()('P')
	c.press(fyne.KeyS)
	c.OnTypedRune()('S')
	if len(keys) != 0 || string(runes) != "PS" {
		t.Fatal("Shift+P/S leaked plain Pull/Start or lost uppercase commands")
	}
}

func TestWorkspaceShortcutStripWrapsAcrossInspector(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	d := NewDashboard(s)
	w := polishWindow(t, fa, d, 710, 660)
	d.keyboardActive = true
	d.inspectorOpen = true
	for _, view := range []ViewMode{ViewKanban, ViewList, ViewGrid} {
		s.ViewMode = view
		d.Rebuild()
		root := d.innerSlot.Objects[0].(*fyne.Container)
		footer := root.Objects[len(root.Objects)-1].(*fyne.Container)
		text := footer.Objects[0].(*wrappedValue)
		if footer.Size().Width != root.Size().Width || len(text.lines(text.Size().Width)) < 2 {
			t.Fatal("shortcut strip is truncated inside browser pane")
		}
		for _, hint := range []string{"Board/Grid [g]", "Cycle view [v]", shortcutLabel("Inspector", platformShortcut("I")), "Terminal [Enter]", "Send PR [Shift+P]", shortcutLabel("Theme", platformShortcut("T")), shortcutLabel("Reset zoom", platformShortcut("0"))} {
			if !strings.Contains(text.value, hint) {
				t.Fatalf("missing existing/new hint %q", hint)
			}
		}
		for _, line := range text.lines(text.Size().Width) {
			if strings.Contains(line, "[Shift+P]") && !strings.Contains(line, "Send PR [Shift+P]") {
				t.Fatal("shortcut wrapped away from its action")
			}
		}
		if strings.Contains(text.value, "Clear status [Esc]") {
			t.Fatal("advertised inactive Escape action")
		}
		walkPolish(w.Content(), func(o fyne.CanvasObject) {
			if _, ok := o.(fyne.Focusable); ok {
				t.Errorf("keyboard workspace has focusable %T", o)
			}
		})
	}
	s.StatusMessage = "Refresh complete"
	d.Rebuild()
	root := d.innerSlot.Objects[0].(*fyne.Container)
	footer := root.Objects[len(root.Objects)-1].(*fyne.Container)
	if !strings.Contains(footer.Objects[0].(*wrappedValue).value, "Clear status [Esc]") {
		t.Fatal("status dismissal shortcut missing")
	}
	s.SelectedCard = 0
	d.Rebuild()
	var help *wrappedValue
	walkPolish(d.Content(), func(o fyne.CanvasObject) {
		if text, ok := o.(*wrappedValue); ok && strings.Contains(text.value, "New worktree [c]") {
			help = text
		}
	})
	if help == nil || !strings.Contains(help.value, "From issue [i]") || !strings.Contains(help.value, "Pull [p]") || strings.Contains(help.value, "Delete worktree [d]") {
		t.Fatal("main shortcuts omitted creation or implied main deletion")
	}
}

func TestDependencyBannerPreservesCanvasKeyboardEvents(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	s := workspaceState()
	d := NewDashboard(s)
	w := polishWindow(t, fa, d, 1000, 740)
	a := &App{fyneApp: fa, window: w, dashboard: d, repos: []*repoEntry{{state: s, dashboard: d}}, sysdepsBanner: container.NewVBox()}
	a.updateSysdepsBanner([]sysdeps.Reported{{Check: sysdeps.Check{Name: "fixture"}, Result: sysdeps.Result{Status: sysdeps.StatusMissing}}})
	w.SetContent(container.NewBorder(a.sysdepsBanner, nil, nil, nil, d.Content()))
	c := &keyboardCanvas{Canvas: w.Canvas()}
	setupKeyHandlers(c, a.handleKeyName, a.handleRune)
	c.press(fyne.KeyTab)
	c.press(fyne.KeyG)
	if s.ViewMode != ViewGrid {
		t.Fatalf("Tab followed by g did not reach canvas view binding; focus=%T view=%v", c.Focused(), s.ViewMode)
	}
	if c.Focused() != nil {
		t.Fatalf("dependency banner captured canvas keyboard focus: %T", c.Focused())
	}
}
