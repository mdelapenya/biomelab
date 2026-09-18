package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// mockDialog is a stand-in for fyne dialog.Dialog used to inspect Hide() calls.
type mockDialog struct{ hidden int }

func (m *mockDialog) Hide() { m.hidden++ }

// TestHandleKeyName_EscapeClearsDialogStateEvenWhenHideSkipsCallback is a
// regression test for the bug where dialog.ConfirmDialog.Hide() in Fyne v2.7
// does NOT invoke the func(ok bool) callback when called programmatically
// (only button clicks do). The openDialog() cleanup runs from that callback,
// so an Escape dismissal left a.dialogOpen=true forever, swallowing every
// subsequent keypress — `e`, `c`, `f`, `p` etc.
//
// handleKeyName's global-Escape branch now mirrors the cleanup explicitly.
// This test guarantees that contract holds for any future refactor.
func TestHandleKeyName_EscapeClearsDialogStateEvenWhenHideSkipsCallback(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()
	win := testApp.NewWindow("test")
	defer win.Close()

	dlg := &mockDialog{}
	a := &App{
		window:       win,
		dialogOpen:   true,
		activeDialog: dlg,
	}

	a.handleKeyName(fyne.KeyEscape)

	if a.dialogOpen {
		t.Error("dialogOpen should be false after Escape dismissal (regression: was getting stuck true)")
	}
	if a.activeDialog != nil {
		t.Errorf("activeDialog should be nil after Escape, got %v", a.activeDialog)
	}
	if dlg.hidden != 1 {
		t.Errorf("Hide() should be called exactly once, got %d", dlg.hidden)
	}
}

// TestHandleKeyName_EscapeWithNilActiveDialog ensures the Escape branch is
// robust against a logical inconsistency (dialogOpen=true but activeDialog=nil).
// State must still be cleared so future keys reach their handlers.
func TestHandleKeyName_EscapeWithNilActiveDialog(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()
	win := testApp.NewWindow("test")
	defer win.Close()

	a := &App{
		window:       win,
		dialogOpen:   true,
		activeDialog: nil,
	}

	a.handleKeyName(fyne.KeyEscape)

	if a.dialogOpen {
		t.Error("dialogOpen should be cleared even when activeDialog is nil")
	}
}

// TestHandleKeyName_NonEscapeIsSwallowedWhileDialogOpen documents the
// short-circuit on the second guard: any non-Escape key is dropped while a
// dialog is open. Combined with the Escape regression test above, this
// confirms the fix isolation — only Escape clears state, other keys still
// honour the dialog-open guard.
func TestHandleKeyName_NonEscapeIsSwallowedWhileDialogOpen(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()
	win := testApp.NewWindow("test")
	defer win.Close()

	dlg := &mockDialog{}
	a := &App{
		window:       win,
		dialogOpen:   true,
		activeDialog: dlg,
	}

	a.handleKeyName(fyne.KeyE)

	if !a.dialogOpen {
		t.Error("dialogOpen should remain true after a non-Escape key")
	}
	if a.activeDialog != dlg {
		t.Error("activeDialog should remain set after a non-Escape key")
	}
	if dlg.hidden != 0 {
		t.Errorf("Hide() should NOT be called for non-Escape keys, got %d calls", dlg.hidden)
	}
}

// TestHandleKeyName_AddRepoFromEmptyState ensures the initial empty screen
// does not require switching focus to a repository panel that does not exist.
func TestHandleKeyName_AddRepoFromEmptyState(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()
	win := testApp.NewWindow("test")
	defer win.Close()

	a := &App{
		window: win,
		focus:  focusRight,
	}

	a.handleKeyName(fyne.KeyA)

	if !a.dialogOpen {
		t.Fatal("a should open the add-repository dialog when no repositories are registered")
	}
	if a.activeDialog == nil {
		t.Fatal("add-repository dialog should be tracked as the active dialog")
	}
}

// TestOpenDialog_CleanupResetsState verifies the cleanup closure returned by
// openDialog() resets every piece of dialog state. The Escape regression
// fix duplicates this cleanup inline; this test pins the canonical version
// so the two paths stay in sync.
func TestOpenDialog_CleanupResetsState(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()
	win := testApp.NewWindow("test")
	defer win.Close()

	a := &App{window: win}

	cleanup := a.openDialog()
	if !a.dialogOpen {
		t.Fatal("openDialog should set dialogOpen=true")
	}
	a.activeDialog = &mockDialog{}

	cleanup()

	if a.dialogOpen {
		t.Error("cleanup should reset dialogOpen to false")
	}
	if a.activeDialog != nil {
		t.Error("cleanup should reset activeDialog to nil")
	}
}
