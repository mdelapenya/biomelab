package gui

import (
	"image/png"
	"os"
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/kits"
)

func kitForPicker(name, kind string) kits.Kit {
	return kits.Kit{Name: name, Kind: kind, DisplayName: name}
}

func TestKitPickerSelectionAcrossPagesAndReplacement(t *testing.T) {
	bases := []kits.Kit{kitForPicker("alpha", kits.KindSandbox), kitForPicker("beta", kits.KindSandbox)}
	mixins := make([]kits.Kit, 7)
	for i := range mixins {
		mixins[i] = kitForPicker(string(rune('a'+i))+"-mixin", kits.KindMixin)
	}
	picker := newKitPicker(bases, mixins)
	if picker.pages() != 2 || len(picker.visible()) != kitPageSize {
		t.Fatalf("initial pagination: pages=%d visible=%d", picker.pages(), len(picker.visible()))
	}
	picker.selectKit(bases[0], true)
	picker.page = 1
	picker.selectKit(mixins[6], true)
	picker.page = 0
	if !picker.selected[bases[0].OCIReference()] || !picker.selected[mixins[6].OCIReference()] {
		t.Fatal("page transition lost selection")
	}
	picker.selectKit(bases[1], true)
	if picker.selected[bases[0].OCIReference()] || !picker.selected[bases[1].OCIReference()] {
		t.Fatal("new sandbox should replace old sandbox across pages")
	}
	want := []string{"beta", mixins[6].Name}
	got := make([]string, 0, 2)
	for _, k := range picker.selection() {
		got = append(got, k.Name)
	}
	if !reflect.DeepEqual(got, want) || picker.validation() != "" {
		t.Fatalf("selection=%v validation=%q, want %v", got, picker.validation(), want)
	}
}

func TestKitPickerRequiresBaseAndCompatibleMixins(t *testing.T) {
	base := kitForPicker("claude", kits.KindSandbox)
	mixin := kitForPicker("codex-only", kits.KindMixin)
	mixin.Requires.Agent = "codex"
	picker := newKitPicker([]kits.Kit{base}, []kits.Kit{mixin})
	if picker.validation() == "" {
		t.Fatal("missing base should be invalid")
	}
	picker.selectKit(base, true)
	picker.selectKit(mixin, true)
	if picker.validation() == "" {
		t.Fatal("incompatible mixin should be invalid")
	}
	if !picker.selected[mixin.OCIReference()] {
		t.Fatal("validation should preserve incompatible selection for review")
	}
	picker.selectKit(mixin, false)
	if picker.validation() != "" {
		t.Fatal("base alone should be valid")
	}
}

func TestKitPickerAcceptsMixinForExtendedSandboxLineage(t *testing.T) {
	root := kitForPicker("claude", kits.KindSandbox)
	parent := kitForPicker("claude-safe", kits.KindSandbox)
	parent.Extends = "claude"
	child := kitForPicker("claude-safe-team", kits.KindSandbox)
	child.Extends = parent.OCIReference()
	mixin := kitForPicker("claude-tools", kits.KindMixin)
	mixin.Requires.Agent = "claude"
	picker := newKitPicker([]kits.Kit{root, parent, child}, []kits.Kit{mixin})
	picker.selectKit(child, true)
	picker.selectKit(mixin, true)
	if got := picker.validation(); got != "" {
		t.Fatalf("inherited Claude mixin rejected: %s", got)
	}
	mixin.Requires.Agent = "codex"
	picker.all[len(picker.all)-1] = mixin
	if got := picker.validation(); got == "" {
		t.Fatal("unrelated agent should be incompatible")
	}
}

func TestKitPickerLineageStopsAtCycleAndDepthLimit(t *testing.T) {
	a := kitForPicker("a", kits.KindSandbox)
	b := kitForPicker("b", kits.KindSandbox)
	a.Extends, b.Extends = "b", "a"
	picker := newKitPicker([]kits.Kit{a, b}, nil)
	names := picker.sandboxLineage(a)
	if !names["a"] || !names["b"] || len(names) != 2 {
		t.Fatalf("cyclic lineage = %v", names)
	}
	long := make([]kits.Kit, 7)
	for i := range long {
		long[i] = kitForPicker(string(rune('a'+i)), kits.KindSandbox)
		if i+1 < len(long) {
			long[i].Extends = string(rune('b' + i))
		}
	}
	picker = newKitPicker(long, nil)
	names = picker.sandboxLineage(long[0])
	if names["g"] {
		t.Fatalf("lineage exceeded five ancestors: %v", names)
	}
}

func walkKitDialog(obj fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	visit(obj)
	switch o := obj.(type) {
	case *fyne.Container:
		for _, child := range o.Objects {
			walkKitDialog(child, visit)
		}
	case *widget.Card:
		walkKitDialog(o.Content, visit)
	case *container.Scroll:
		walkKitDialog(o.Content, visit)
	}
}

func kitDialogControls(win fyne.Window) (cards []*widget.Card, checks []*dialogCheck, buttons map[string]*dialogButton, labels []string) {
	buttons = make(map[string]*dialogButton)
	walkKitDialog(win.Canvas().Overlays().Top().(*widget.PopUp).Content, func(obj fyne.CanvasObject) {
		switch w := obj.(type) {
		case *widget.Card:
			cards = append(cards, w)
		case *dialogCheck:
			checks = append(checks, w)
		case *dialogButton:
			buttons[w.Text] = w
		case *widget.Label:
			labels = append(labels, w.Text)
		}
	})
	return
}

func TestKitDialogPagesAndContinuesWithFullSelection(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("kits")
	defer win.Close()
	win.Resize(fyne.NewSize(900, 720))
	bases := []kits.Kit{kitForPicker("alpha", kits.KindSandbox)}
	mixins := make([]kits.Kit, 8)
	for i := range mixins {
		mixins[i] = kitForPicker(string(rune('a'+i))+"-mixin", kits.KindMixin)
	}
	var done, submitted int
	var got []kits.Kit
	d := showKitsDialog(win, "repo", bases, mixins, func() { done++ }, func(selected []kits.Kit) {
		submitted++
		got = selected
	})
	defer d.Hide()
	cards, checks, buttons, labels := kitDialogControls(win)
	if len(cards) != kitPageSize || len(checks) != kitPageSize || !buttons["Continue"].Disabled() {
		t.Fatalf("initial cards=%d checks=%d continue disabled=%v", len(cards), len(checks), buttons["Continue"].Disabled())
	}
	if !containsString(labels, "SANDBOX") || !containsString(labels, "MIXIN") {
		t.Fatalf("missing kind badges: %v", labels)
	}
	var fallbackLogos int
	walkKitDialog(win.Canvas().Overlays().Top().(*widget.PopUp).Content, func(obj fyne.CanvasObject) {
		if img, ok := obj.(*canvas.Image); ok && img.Resource != nil && img.Resource.Name() == "kit-fallback.svg" {
			fallbackLogos++
		}
	})
	if fallbackLogos != kitPageSize {
		t.Fatalf("fallback logos=%d, want %d", fallbackLogos, kitPageSize)
	}
	checks[0].SetChecked(true)
	_, checks, buttons, _ = kitDialogControls(win)
	checks[1].SetChecked(true)
	_, _, buttons, _ = kitDialogControls(win)
	buttons["Next"].Tapped(nil)
	_, checks, buttons, labels = kitDialogControls(win)
	if len(checks) != 1 || !containsString(labels, "Page 2 of 2") {
		t.Fatalf("second page checks=%d labels=%v", len(checks), labels)
	}
	checks[0].SetChecked(true)
	_, _, buttons, _ = kitDialogControls(win)
	buttons["Previous"].Tapped(nil)
	_, checks, buttons, _ = kitDialogControls(win)
	if !checks[0].Checked || !checks[1].Checked || buttons["Continue"].Disabled() {
		t.Fatal("selection was not retained across pages")
	}
	buttons["Continue"].Tapped(nil)
	if done != 1 || submitted != 1 || len(got) != 3 || got[0].Name != "alpha" || got[1].Name != mixins[0].Name || got[2].Name != mixins[7].Name {
		t.Fatalf("done=%d submitted=%d selected=%v", done, submitted, got)
	}
}

func TestKitDialogEmptyAndCancellation(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("empty kits")
	defer win.Close()
	var done, submitted int
	d := showKitsDialog(win, "repo", nil, nil, func() { done++ }, func([]kits.Kit) { submitted++ })
	_, checks, buttons, labels := kitDialogControls(win)
	if len(checks) != 0 || !buttons["Continue"].Disabled() || !containsString(labels, "No Docker Sandbox kits are available.") {
		t.Fatalf("empty catalog controls: checks=%d labels=%v", len(checks), labels)
	}
	d.Hide()
	if done != 1 || submitted != 0 {
		t.Fatalf("cancel done=%d submitted=%d", done, submitted)
	}
}

func TestKitDialogScreenshot(t *testing.T) {
	path := os.Getenv("BIOMELAB_KIT_PICKER_SCREENSHOT")
	if path == "" {
		t.Skip("set BIOMELAB_KIT_PICKER_SCREENSHOT to capture a visual review image")
	}
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("kits")
	defer win.Close()
	win.Resize(fyne.NewSize(900, 720))
	base := kitForPicker("claude", kits.KindSandbox)
	base.DisplayName = "Claude Code"
	base.Description = "A complete agent environment"
	base.LogoURL = os.Getenv("BIOMELAB_KIT_PICKER_LOGO_URL")
	if base.LogoURL != "" {
		base.Name = "openclaw"
		base.DisplayName = "OpenClaw"
	}
	mixins := []kits.Kit{
		kitForPicker("playwright", kits.KindMixin), kitForPicker("task", kits.KindMixin),
		kitForPicker("code-server", kits.KindMixin), kitForPicker("gitlab", kits.KindMixin),
		kitForPicker("neovim", kits.KindMixin), kitForPicker("trivy", kits.KindMixin),
		kitForPicker("mise", kits.KindMixin),
	}
	d := showKitsDialog(win, "example/repo", []kits.Kit{base}, mixins, func() {}, func([]kits.Kit) {})
	defer d.Hide()
	if base.LogoURL != "" {
		time.Sleep(3 * time.Second) // screenshot-only live logo review
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, win.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
