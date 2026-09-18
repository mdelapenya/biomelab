package gui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/github"
)

func TestIssueErrorDialogTracksEscapeCleanup(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	w := fa.NewWindow("error")
	defer w.Close()
	a := &App{window: w}
	a.showIssueError(errors.New("lookup failed"))
	if !a.dialogOpen || a.activeDialog == nil {
		t.Fatal("error dialog is not tracked by the app")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.dialogOpen || a.activeDialog != nil {
		t.Fatal("Escape did not clean up tracked error dialog")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.dialogOpen || a.activeDialog != nil {
		t.Fatal("repeated Escape corrupted dialog state")
	}
}

func TestIssueDialogsEscapeCleanupExactlyOnce(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	w := fa.NewWindow("dialogs")
	defer w.Close()
	var closed atomic.Int32
	p := showIssuePreview(w, github.IssueInfo{Number: 1, Title: "title", URL: "https://github.com/o/r/issues/1", State: "OPEN"}, "o/r", "/repo", "main", func() {
		closed.Add(1)
	}, func(string) error { return nil })
	p.branch.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	p.Hide()
	if got := closed.Load(); got != 1 {
		t.Fatalf("cleanup called %d times", got)
	}
}

func TestIssuePreviewBoundsLongContent(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("long issue preview")
	defer w.Close()
	w.Resize(fyne.NewSize(1200, 700))
	p := showIssuePreview(w, github.IssueInfo{
		Number: 82,
		Title:  strings.Repeat("long issue title ", 30),
		Body:   strings.Repeat("A long issue description that must remain scrollable. ", 200),
		URL:    "https://github.com/very-long-owner/very-long-repository/issues/82",
		State:  "OPEN",
	}, "very-long-owner/very-long-repository", "/Users/developer/"+strings.Repeat("nested-repository/", 10), strings.Repeat("long-head-", 12), func() {}, func(string) error { return nil })
	defer p.Hide()

	if got := p.MinSize(); got.Width > 1200 || got.Height > 700 {
		t.Fatalf("dialog minimum size %v exceeds a 1200x700 application window", got)
	}
	popup, ok := w.Canvas().Overlays().Top().(*widget.PopUp)
	if !ok {
		t.Fatal("issue preview did not create a popup overlay")
	}
	if size := popup.Size(); size.Width > 1200 || size.Height > 700 {
		t.Fatalf("popup size %v exceeds the application window", size)
	}
	if !p.create.Visible() || !p.cancel.Visible() || !p.branch.Visible() {
		t.Fatal("branch and action controls must remain visible with long content")
	}
}
