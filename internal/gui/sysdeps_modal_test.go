package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

func TestDependenciesOwnModalAndAllActionsDismissWithEscape(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantLight))
	defer applyDarkPalette()
	w := fa.NewWindow("dependencies")
	defer w.Close()
	w.Resize(fyne.NewSize(700, 550))
	w.SetContent(container.NewStack())
	cache := sysdeps.NewCache(time.Hour)
	cache.SetChecks([]sysdeps.Check{{Name: "fixture", DisplayName: "Fixture dependency", InstallHint: "install fixture", DocsURL: "https://example.com/docs", Probe: func() sysdeps.Result { return sysdeps.Result{Status: sysdeps.StatusMissing} }}})
	cache.Get(nil)
	// Disable the background tray update; the cached rows are sufficient and
	// no external probe or desktop integration participates in this test.
	a := &App{window: w, fyneApp: fa, sysdepsCache: cache, sysdepsClosed: true}
	for _, target := range []string{"Close", "Copy install cmd", "Docs", "Re-check"} {
		d := a.showSysDepsDialog()
		if d == nil || !a.dialogOpen || a.activeDialog != d {
			t.Fatal("Dependencies did not acquire modal ownership")
		}
		if a.showSysDepsDialog() != nil {
			t.Fatal("Dependencies stacked another dialog over its modal")
		}
		var action *dialogButton
		walkPolish(w.Canvas().Overlays().Top().(*widget.PopUp).Content, func(obj fyne.CanvasObject) {
			if b, ok := obj.(*dialogButton); ok && b.Text == target {
				action = b
			}
		})
		if action == nil {
			t.Fatalf("missing keyboard action %q", target)
		}
		a.handleKeyName(fyne.KeyG)
		if !a.dialogOpen || a.activeDialog != d {
			t.Fatal("global view shortcut escaped modal guard")
		}
		w.Canvas().Focus(action)
		if target == "Copy install cmd" {
			action.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
			if fa.Clipboard().Content() != "install fixture" || !a.dialogOpen {
				t.Fatal("Enter did not copy while preserving the modal")
			}
		}
		action.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		if a.dialogOpen || a.activeDialog != nil || w.Canvas().Focused() != nil || w.Canvas().Overlays().Top() != nil {
			t.Fatalf("Escape from %q did not release modal", target)
		}
	}
	d := a.showSysDepsDialog()
	closeButton := w.Canvas().Focused().(*dialogButton)
	if closeButton.Text != "Close" {
		t.Fatal("Close is not the initial keyboard action")
	}
	closeButton.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if a.dialogOpen || a.activeDialog != nil {
		t.Fatal("Enter on Close did not release modal")
	}
	d.Hide() // repeated dismissal must remain safe
}

func TestDependencyMissingAndDegradedHaveDistinctSemantics(t *testing.T) {
	a := &App{}
	for status, want := range map[sysdeps.Status]widget.Importance{sysdeps.StatusMissing: widget.DangerImportance, sysdeps.StatusDegraded: widget.WarningImportance} {
		row := a.buildSysDepsRow(sysdeps.Reported{Result: sysdeps.Result{Status: status}}).(*fyne.Container)
		var dot *widget.Label
		walkPolish(row, func(object fyne.CanvasObject) {
			if label, ok := object.(*widget.Label); ok && label.Text == statusDot(status) {
				dot = label
			}
		})
		if dot == nil {
			t.Fatal("dependency status marker missing")
		}
		if dot.Importance != want {
			t.Fatalf("status %v has importance %v, want %v", status, dot.Importance, want)
		}
	}
}
