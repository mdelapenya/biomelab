package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/sandbox"
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

func TestAsyncStatusAndRefreshStayWithOriginatingRepo(t *testing.T) {
	origin := &repoEntry{state: &RepoState{}}
	other := &repoEntry{state: &RepoState{}}
	a := &App{repos: []*repoEntry{origin, other}, active: 1}
	refreshes := 0
	a.setRepoStatus(origin, "created branch", false)
	a.refreshRepo(origin, func() { refreshes++ })
	if origin.state.StatusMessage != "created branch" || other.state.StatusMessage != "" || refreshes != 1 {
		t.Fatalf("completion routed incorrectly: origin=%q other=%q refreshes=%d",
			origin.state.StatusMessage, other.state.StatusMessage, refreshes)
	}
	a.repos = []*repoEntry{other}
	a.setRepoStatus(origin, "late error", true)
	a.refreshRepo(origin, func() { refreshes++ })
	if origin.state.StatusMessage != "created branch" || refreshes != 1 {
		t.Fatal("removed repository accepted a late completion")
	}
}

func TestSandboxInventoryReplacesSuccessfulEmptySnapshot(t *testing.T) {
	a := &App{sbxStatuses: map[string]sandbox.Status{"removed": sandbox.StatusRunning}}
	a.applySandboxInventory(ops.RefreshResult{HasSbxInventory: true, AllSbxStatuses: map[string]sandbox.Status{}})
	if len(a.sbxStatuses) != 0 {
		t.Fatalf("removed sandbox survived successful empty listing: %v", a.sbxStatuses)
	}
	a.sbxStatuses["current"] = sandbox.StatusStopped
	a.applySandboxInventory(ops.RefreshResult{})
	if len(a.sbxStatuses) != 1 || a.sbxStatuses["current"] != sandbox.StatusStopped {
		t.Fatal("failed listing erased the last known inventory")
	}
}
