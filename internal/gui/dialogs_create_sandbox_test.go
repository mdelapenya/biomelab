package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func TestConfirmCreateSandboxBoundsLongCommand(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	win := app.NewWindow("create sandbox")
	defer win.Close()
	win.Resize(fyne.NewSize(1200, 700))

	repoPath := "/Users/developer/source/" + strings.Repeat("deeply-nested-repository/", 12)
	kitURLs := []string{
		"docker.io/sbx/code-server-kit:latest",
		"ghcr.io/example/organization/a-long-tooling-kit-reference:2026.09",
		"registry.example.com/platform/sandbox/another-long-kit-reference:latest",
	}
	wantCommand := sandbox.CommandString(sandbox.CreateArgs("long-path", "claude", repoPath, kitURLs))

	d := showConfirmCreateSandbox(win, "long-path", "claude", repoPath, kitURLs, func() {}, func() {})
	defer d.Hide()

	if got := d.MinSize(); got.Width > 1200 || got.Height > 700 {
		t.Fatalf("dialog minimum size %v exceeds a 1200x700 application window", got)
	}

	var commandText *canvas.Text
	walkSetupContent(win.Canvas().Overlays().Top().(*widget.PopUp).Content, func(obj fyne.CanvasObject) {
		if scroll, ok := obj.(*container.Scroll); ok {
			if text, ok := scroll.Content.(*canvas.Text); ok && text.Text == wantCommand {
				commandText = text
			}
		}
		if text, ok := obj.(*canvas.Text); ok && text.Text == wantCommand {
			commandText = text
		}
	})
	if commandText == nil {
		t.Fatal("complete sandbox creation command is not retained in dialog content")
	}
}
