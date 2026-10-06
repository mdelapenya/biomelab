package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/git"
	"testing"
)

func TestSendPRRemoteSelectionRetainsModalGuardThroughConfirmation(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("send PR")
	defer w.Close()
	a := &App{window: w}
	done := a.openDialog()
	selection := showSendPRRemoteSelection(w, []git.RemoteInfo{{Name: "origin"}, {Name: "upstream"}}, done, func(int) {
		a.activeDialog = showSendPRConfirm(w, "feature", git.RemoteInfo{Name: "origin"}, nil, false, done, func(bool) {}, nil)
	})
	a.activeDialog = selection
	button, ok := w.Canvas().Focused().(*dialogButton)
	if !ok {
		t.Fatalf("remote button was not focused: %T", w.Canvas().Focused())
	}
	button.Tapped(nil)
	if !a.dialogOpen || a.activeDialog == nil || a.activeDialog == selection {
		t.Fatal("remote selection cleared modal ownership before final confirmation")
	}
	a.handleKeyName(fyne.KeyE)
	if !a.dialogOpen {
		t.Fatal("global shortcut escaped through the final confirmation")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.dialogOpen {
		t.Fatal("final confirmation did not release modal guard")
	}
}
