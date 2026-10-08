package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/github"
)

func TestDialogTechnicalCommandRefreshesInOpenView(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dark := newBiomeTheme(VariantDark)
	app.Settings().SetTheme(dark)
	w := app.NewWindow("command")
	defer w.Close()
	command := dialogCommand("sbx create --name example '/a path/repo'")
	w.SetContent(command)
	w.Resize(fyne.NewSize(480, 150))
	w.Show()
	text := command.Objects[0].(*container.Scroll).Content.(*canvas.Text)
	before := text.TextSize
	light := newBiomeTheme(VariantLight)
	light.ZoomIn()
	app.Settings().SetTheme(light)
	command.Refresh()
	if text.Color != theme.Color(theme.ColorNameForeground) || text.TextSize != theme.TextSize() || text.TextSize <= before {
		t.Fatalf("open command retained stale style: color=%v size=%v, want color=%v size=%v", text.Color, text.TextSize, theme.Color(theme.ColorNameForeground), theme.TextSize())
	}
	if !text.TextStyle.Monospace {
		t.Fatal("command lost technical typography")
	}
	surface := &dialogBubbleSurface{fill: colorBubbleHuman, stroke: colorBubbleHumanStroke}
	surface.ExtendBaseWidget(surface)
	renderer := surface.CreateRenderer().(*dialogBubbleRenderer)
	app.Settings().SetTheme(newBiomeTheme(VariantDark))
	renderer.Refresh()
	if renderer.rect.FillColor != colorBubbleHuman() || renderer.rect.StrokeColor != colorBubbleHumanStroke() {
		t.Fatal("open activity bubble retained stale palette")
	}
}

func TestDialogIssueFitsNarrowWindowAtZoom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	for _, variant := range []ThemeVariant{VariantDark, VariantLight} {
		t.Run(string(variant), func(t *testing.T) {
			th := newBiomeTheme(variant)
			th.ZoomIn()
			app.Settings().SetTheme(th)
			w := app.NewWindow("issue preview")
			defer w.Close()
			w.Resize(fyne.NewSize(640, 560))
			w.SetContent(widget.NewLabel("Repository dashboard"))
			w.Show()
			preview := showIssuePreview(w, github.IssueInfo{Number: 82, Title: "Improve agent handoff and preserve issue context", Body: strings.Repeat("Requirements remain available to the next agent. ", 100), URL: "https://github.com/example/repository/issues/82", State: "OPEN"}, "example/repository", "/Users/developer/"+strings.Repeat("nested/", 30), "main", func() {}, func(string) error { return nil })
			defer preview.Hide()
			popup := requirePopup(t, w.Canvas().Overlays().Top())
			if popup.Size().Width > 640 || popup.Size().Height > 560 {
				t.Fatalf("zoomed issue popup exceeds window: %v", popup.Size())
			}
			for _, control := range []fyne.CanvasObject{preview.branch, preview.create, preview.cancel} {
				position := app.Driver().AbsolutePositionForObject(control)
				if !control.Visible() || control.Size().Height <= 0 || position.Y < 0 || position.Y+control.Size().Height > 560 {
					t.Fatalf("issue control is outside the initial viewport: position=%v size=%v", position, control.Size())
				}
			}
			if preview.create.Importance != widget.HighImportance || !preview.branch.TextStyle.Monospace {
				t.Fatal("issue action or technical branch hierarchy lost")
			}
			if artifactDir := os.Getenv("BIOMELAB_DIALOG_ARTIFACT_DIR"); artifactDir != "" {
				if err := os.MkdirAll(artifactDir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(artifactDir, "issue-dialog-"+string(variant)+".png"))
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
		})
	}
}

func TestDialogModeChoicesRemainSeparate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(newBiomeTheme(VariantLight))
	w := app.NewWindow("mode selection")
	defer w.Close()
	w.Resize(fyne.NewSize(520, 340))
	d := showModeSelection(w, func() {}, func() {}, func() {})
	defer d.Hide()
	var sandbox, regular *dialogButton
	walkSetupContent(requirePopup(t, w.Canvas().Overlays().Top()).Content, func(object fyne.CanvasObject) {
		if button, ok := object.(*dialogButton); ok {
			switch button.Text {
			case "Sandbox (recommended)":
				sandbox = button
			case "Regular (host)":
				regular = button
			}
		}
	})
	if sandbox == nil || regular == nil {
		t.Fatal("mode choices missing")
	}
	sandboxPosition := app.Driver().AbsolutePositionForObject(sandbox)
	regularPosition := app.Driver().AbsolutePositionForObject(regular)
	if regularPosition.Y < sandboxPosition.Y+sandbox.Size().Height {
		t.Fatalf("mode choices overlap: sandbox=%v height=%v regular=%v", sandboxPosition, sandbox.Size().Height, regularPosition)
	}
}
