package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/mdelapenya/biomelab/internal/git"
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

func findNoteControls(root fyne.CanvasObject) (*noteEntry, *dialogButton) {
	var entry *noteEntry
	var save *dialogButton
	var visit func(fyne.CanvasObject)
	visit = func(obj fyne.CanvasObject) {
		if e, ok := obj.(*noteEntry); ok && e.MultiLine && !e.Disabled() {
			entry = e
		}
		if b, ok := obj.(*dialogButton); ok && b.Text == "Save" {
			save = b
		}
		if c, ok := obj.(*fyne.Container); ok {
			for _, child := range c.Objects {
				visit(child)
			}
		}
		if split, ok := obj.(*container.Split); ok {
			visit(split.Leading)
			visit(split.Trailing)
		}
	}
	visit(root)
	return entry, save
}

func TestNoteSaveFailureKeepsEditorAndOriginatingRepo(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".biomelab"), []byte("blocks draft directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := git.Worktree{Path: root, Branch: "feature"}
	origin := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	other := &repoEntry{state: &RepoState{}}
	a := &App{fyneApp: fa, repos: []*repoEntry{origin, other}, active: 0}
	a.openNoteDialog(wt)
	w := a.noteWindows[root]
	defer w.Close()
	entry, save := findNoteControls(w.Content())
	if entry == nil || save == nil {
		t.Fatal("note editor controls missing")
	}
	entry.SetText("unsaved draft")
	a.active = 1 // the editor is non-modal, so repo navigation remains possible
	save.Tapped(nil)
	if a.noteWindows[root] != w || entry.Text != "unsaved draft" {
		t.Fatal("failed save closed the editor or discarded its text")
	}
	if !strings.Contains(origin.state.StatusMessage, "Note save failed") || other.state.StatusMessage != "" {
		t.Fatalf("save error went to wrong repo: origin=%q other=%q", origin.state.StatusMessage, other.state.StatusMessage)
	}
}

func TestRegentLogSessionDiscardsOlderAndClosedCompletions(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	got := make(chan string, 3)
	s := newRegentLogSession()
	s.fetch = func(_ context.Context, path string) regentLogData {
		if path == "old" {
			close(oldStarted)
			<-releaseOld
		}
		return regentLogData{sessionID: path}
	}
	s.load("old", func(data regentLogData) { got <- data.sessionID })
	<-oldStarted
	s.load("new", func(data regentLogData) { got <- data.sessionID })
	select {
	case value := <-got:
		if value != "new" {
			t.Fatalf("stale completion rendered: %q", value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new load blocked behind older command")
	}
	close(releaseOld)
	s.close()
	select {
	case value := <-got:
		t.Fatalf("completion rendered after close: %q", value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRegentExportCancellationDropsCompletion(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	started := make(chan struct{})
	finished := make(chan struct{})
	called := make(chan struct{}, 1)
	s := newRegentLogSession()
	s.fetchRaw = func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		defer close(finished)
		return []byte("stale"), nil
	}
	s.export("branch", func([]byte, error) { called <- struct{}{} })
	<-started
	s.close()
	<-finished
	select {
	case <-called:
		t.Fatal("closed window accepted export completion")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRemoteSelectionScrollsToLastOptionInsideWindow(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantDark))
	w := fa.NewWindow("many remotes")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 460))
	remotes := make([]git.RemoteInfo, 40)
	for i := range remotes {
		remotes[i] = git.RemoteInfo{Name: fmt.Sprintf("remote-%02d", i), Repo: "example/project"}
	}
	selected, closed := -1, 0
	d := showSendPRRemoteSelection(w, remotes, func() { closed++ }, func(index int) { selected = index })
	defer d.Hide()
	popup := requirePopup(t, w.Canvas().Overlays().Top())
	if popup.Size().Height > w.Canvas().Size().Height {
		t.Fatal("remote list grew beyond the window")
	}
	var scroll *container.Scroll
	var last *dialogButton
	walkPolish(popup.Content, func(obj fyne.CanvasObject) {
		if s, ok := obj.(*container.Scroll); ok {
			scroll = s
		}
		if button, ok := obj.(*dialogButton); ok && strings.HasPrefix(button.Text, "remote-39") {
			last = button
		}
	})
	if scroll == nil || last == nil {
		t.Fatal("bounded remote list missing")
	}
	scroll.ScrollToBottom()
	pos := fa.Driver().AbsolutePositionForObject(last).Add(fyne.NewPos(8, 8))
	if pos.Y < 0 || pos.Y >= w.Canvas().Size().Height {
		t.Fatal("last remote is unreachable after scrolling")
	}
	test.TapCanvas(w.Canvas(), pos)
	if selected != 39 || closed != 0 {
		t.Fatalf("last remote selected=%d cleanup=%d", selected, closed)
	}
}

func TestNoteEditorExposesCompleteLinkedWorktreeContext(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(NewTheme(VariantDark))
	wt := git.Worktree{Branch: "feature/" + strings.Repeat("long-branch/", 12), Path: t.TempDir()}
	a := &App{fyneApp: fa}
	a.openNoteDialog(wt)
	w := a.noteWindows[wt.Path]
	defer w.Close()
	found := false
	walkPolish(w.Content(), func(obj fyne.CanvasObject) {
		if context, ok := obj.(*noteEntry); ok && context.Text == "Branch: "+wt.Branch+"\nPath: "+wt.Path {
			found = context.TextStyle.Monospace && context.Wrapping == fyne.TextWrapBreak && context.Disabled()
		}
	})
	if !found {
		t.Fatal("existing note editor does not expose complete selectable branch/path")
	}
}
