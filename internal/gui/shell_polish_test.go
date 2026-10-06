package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func TestShellReservesCompactRailAndResponsiveWorkspace(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	rail := canvas.NewRectangle(colorPanelBg)
	workspace := canvas.NewRectangle(colorBackground)
	shell := newShellLayout(rail, workspace, nil).(*fyne.Container)
	for _, width := range []float32{1100, 1440, 800, 20} {
		shell.Resize(fyne.NewSize(width, 700))
		side := rail.Size().Width
		if width >= 800 && side != 190 {
			t.Fatalf("rail width %v at window width %v", side, width)
		}
		if workspace.Position().X != side+1 || workspace.Size().Width != width-side-1 || workspace.Size().Height != 700 {
			t.Fatalf("workspace did not fill remaining shell at width %v: pos=%v size=%v", width, workspace.Position(), workspace.Size())
		}
	}
	banner := canvas.NewRectangle(colorYellow)
	banner.SetMinSize(fyne.NewSize(1, 32))
	root := newShellLayout(rail, workspace, banner)
	root.Resize(fyne.NewSize(1100, 700))
	if banner.Size().Width != 1100 || workspace.Size().Height >= 700 {
		t.Fatal("banner does not reserve a full-width row")
	}
	banner.Hide()
	root.Refresh()
	if workspace.Size().Height != 700 {
		t.Fatal("hidden banner still consumes workspace height")
	}
}

func shellAddControl(t *testing.T, panel *RepoPanel) fyne.Tappable {
	t.Helper()
	var found fyne.Tappable
	walkPolish(panel.Content(), func(obj fyne.CanvasObject) {
		tap, ok := obj.(fyne.Tappable)
		if !ok {
			return
		}
		walkPolish(obj, func(child fyne.CanvasObject) {
			if text, ok := child.(*measuredText); ok && text.full == "Add repository" {
				found = tap
			}
		})
	})
	if found == nil {
		t.Fatal("missing Add repository action")
	}
	return found
}

func TestRepoPanelShellCallbacksStatusesAndKeyboardFocus(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(NewTheme(VariantLight))
	t.Cleanup(applyDarkPalette)
	groups := []*RepoGroup{
		{Name: "A very long repository name that must fit the rail", Modes: []config.ModeEntry{{Type: "regular"}, {Type: "sandbox", Agent: "claude", SandboxName: "dev"}}, LinkedWorktreeCount: 5},
		{Name: "docs", Modes: []config.ModeEntry{{Type: "regular"}}},
	}
	panel := NewRepoPanel(groups, map[string]sandbox.Status{"dev": sandbox.StatusRunning})
	w := app.NewWindow("shell")
	defer w.Close()
	w.SetContent(newShellLayout(panel.Content(), container.NewStack(canvas.NewRectangle(colorBackground)), nil))
	w.Resize(fyne.NewSize(900, 600))
	w.Show()
	walkPolish(panel.Content(), func(obj fyne.CanvasObject) {
		if _, ok := obj.(fyne.Focusable); ok {
			t.Errorf("project rail contains focusable %T", obj)
		}
	})
	action := shellAddControl(t, panel)
	first, second := 0, 0
	panel.OnAddRepository = func() { first++ }
	action.Tapped(nil)
	panel.OnAddRepository = func() { second++ }
	action.Tapped(nil)
	if first != 1 || second != 1 {
		t.Fatal("Add action captured a stale callback")
	}
	selectedGroup, selectedMode := -1, -1
	panel.OnModeSelected = func(g, m int) { selectedGroup, selectedMode = g, m }
	// The mode rows are real tappable objects in the scroll list.
	panel.list.Objects[2].(fyne.Tappable).Tapped(nil)
	if selectedGroup != 0 || selectedMode != 1 {
		t.Fatalf("sandbox selected (%d,%d)", selectedGroup, selectedMode)
	}
	for _, entry := range []struct {
		status sandbox.Status
		label  string
	}{{sandbox.StatusRunning, "Running"}, {sandbox.StatusStopped, "Stopped"}, {sandbox.StatusNotFound, "Missing"}} {
		panel.UpdateStatuses(map[string]sandbox.Status{"dev": entry.status})
		found := false
		walkPolish(panel.Content(), func(obj fyne.CanvasObject) {
			if txt, ok := obj.(*canvas.Text); ok && txt.Text == entry.label {
				found = true
			}
		})
		if !found {
			t.Errorf("missing real sandbox status %q", entry.label)
		}
	}
	panel.keyboardActive = true
	panel.RebuildFull()
	action = shellAddControl(t, panel)
	action.Tapped(nil)
	if second != 2 || w.Canvas().Focused() != nil || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("rail rebuild lost callback, stole focus, or opened an unsolicited overlay")
	}
	// Dragging the last project upward delegates the existing reorder callback.
	from, to := -1, -1
	panel.OnReorder = func(a, b int) { from, to = a, b }
	panel.onHeaderDragged(newDragHandle(1, panel), &fyne.DragEvent{Dragged: fyne.Delta{DY: -500}})
	panel.onHeaderDragEnd(nil)
	if from != 1 || to != 0 {
		t.Fatalf("drag reorder callback (%d,%d)", from, to)
	}
}

// Optional captures use production shell and dashboard widgets, with fixtures.
func TestShellRenderArtifacts(t *testing.T) {
	dir := os.Getenv("BIOMELAB_SHELL_SCREENSHOTS")
	if dir == "" {
		t.Skip("set BIOMELAB_SHELL_SCREENSHOTS for shell captures")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	app := test.NewApp()
	defer app.Quit()
	t.Cleanup(applyDarkPalette)
	savedIcon := AppIcon
	defer func() { AppIcon = savedIcon }()
	if data, err := os.ReadFile("../../cmd/biomelab/icon.png"); err == nil {
		AppIcon = fyne.NewStaticResource("icon.png", data)
	}
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		for _, zoom := range []bool{false, true} {
			th := newBiomeTheme(variant)
			name := string(variant)
			if zoom {
				th.ZoomIn()
				th.ZoomIn()
				name += "-zoom"
			}
			app.Settings().SetTheme(th)
			rp := NewRepoPanel([]*RepoGroup{
				{Name: "biomelab", Modes: []config.ModeEntry{{Type: "regular"}, {Type: "sandbox", Agent: "Claude", SandboxName: "dev"}}, LinkedWorktreeCount: 30},
				{Name: "agent-tools", Modes: []config.ModeEntry{{Type: "sandbox", Agent: "Codex", SandboxName: "tools"}}},
				{Name: "docs", Modes: []config.ModeEntry{{Type: "regular"}}},
			}, map[string]sandbox.Status{"dev": sandbox.StatusRunning, "tools": sandbox.StatusStopped})
			d := NewDashboard(polishState())
			d.RepoName = "biomelab"
			d.Rebuild()
			w := app.NewWindow("Shell production fixture")
			w.SetPadded(false)
			w.SetContent(newShellLayout(rp.Content(), d.Content(), nil))
			w.Resize(fyne.NewSize(1100, 740))
			w.Show()
			path := filepath.Join(dir, name+"-shell.png")
			file, err := os.Create(path)
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
			t.Log(path)
		}
	}
}
