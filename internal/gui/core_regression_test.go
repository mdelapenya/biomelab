package gui

import (
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"testing"
)

func TestTrayVisibilityTracksWindowAcrossHideAndShow(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	window := fyneApp.NewWindow("test")
	defer window.Close()
	window.SetContent(widget.NewLabel("content"))
	window.Show()

	a := &App{window: window, mainWindowVisible: true}
	if a.toggleMainWindowVisible() || a.mainWindowVisible {
		t.Fatal("first tray toggle did not hide the window")
	}
	if !a.toggleMainWindowVisible() || !a.mainWindowVisible {
		t.Fatal("second tray toggle did not show the window")
	}
	// Closing to tray uses the same setter as the menu; the next toggle
	// must show the window even though its content object is still visible.
	a.setMainWindowVisible(false)
	if !window.Content().Visible() {
		t.Fatal("test precondition: hiding a window unexpectedly hid its content")
	}
	if !a.toggleMainWindowVisible() {
		t.Fatal("tray toggle failed to restore a window hidden by close")
	}
}
