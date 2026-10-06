package gui

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	gogit "github.com/go-git/go-git/v6"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/notes"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

func TestMessageDialogRestoresUnderlyingModal(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	w := fa.NewWindow("messages")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))
	w.Show()
	a := &App{window: w}
	a.showInformation("First", "An existing workflow remains open.")
	first := a.activeDialog
	firstFocus := w.Canvas().Focused()
	a.showError(errors.New("Background creation failed"))
	second := a.activeDialog
	if first == second || len(w.Canvas().Overlays().List()) != 2 || !a.dialogOpen {
		t.Fatal("message did not stack above existing modal")
	}
	second.Hide()
	if a.activeDialog != first || !a.dialogOpen || w.Canvas().Focused() != firstFocus {
		t.Fatal("message did not restore previous modal and focus")
	}
	first.Hide()
	if a.activeDialog != nil || a.dialogOpen || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("message retained modal ownership after dismissal")
	}
}

func TestMessageDialogDoesNotClearNewOwnerOrRestoreClosedDialog(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	w := fa.NewWindow("messages")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))
	w.Show()
	a := &App{window: w}
	a.showInformation("First", "First message")
	first := a.activeDialog
	a.showInformation("Second", "Second message")
	second := a.activeDialog
	first.Hide() // completion can dismiss an older overlay out of order
	if a.activeDialog != second || !a.dialogOpen || len(w.Canvas().Overlays().List()) != 1 {
		t.Fatal("older message cleared newer modal or its visible overlay")
	}
	second.Hide()
	if a.activeDialog != nil || a.dialogOpen {
		t.Fatal("message restored an already closed modal")
	}
	a.showError(nil)
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("nil error opened a dialog")
	}
}

func TestSecondaryMessageLeavesMainOwnershipIntact(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	main := fa.NewWindow("main")
	defer main.Close()
	secondary := fa.NewWindow("export")
	defer secondary.Close()
	a := &App{window: main}
	a.showInformation("Main", "Main workflow")
	owner := a.activeDialog
	a.showWindowError(secondary, errors.New("Export failed"))
	if a.activeDialog != owner || !a.dialogOpen || len(secondary.Canvas().Overlays().List()) != 1 {
		t.Fatal("secondary error changed main modal ownership")
	}
}

// Opt-in captures exercise the actual production dialog constructors and
// secondary editor/activity widgets with offline fixtures, without CLI calls.
func TestProductionDialogThemeCaptures(t *testing.T) {
	dir := os.Getenv("BIOMELAB_PRODUCTION_DIALOG_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("set BIOMELAB_PRODUCTION_DIALOG_ARTIFACT_DIR for production dialog images")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	fa := test.NewApp()
	defer fa.Quit()
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		fa.Settings().SetTheme(newBiomeTheme(variant))
		w := fa.NewWindow("BiomeLab")
		w.Resize(fyne.NewSize(1000, 740))
		w.SetContent(widget.NewLabel("BiomeLab workspace"))
		w.Show()
		a := &App{window: w, fyneApp: fa}
		capture := func(name string, win fyne.Window) {
			t.Helper()
			f, err := os.Create(filepath.Join(dir, name+"-"+string(variant)+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, win.Canvas().Capture())
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		d := showBranchInput(w, func() {}, func(string) {})
		capture("create-worktree", w)
		d.Hide()
		d = showSendPRRemoteSelection(w, []git.RemoteInfo{{Name: "origin", Repo: "mdelapenya/biomelab"}, {Name: "upstream", Repo: "biomelab/biomelab"}}, func() {}, func(int) {})
		capture("remote", w)
		d.Hide()
		d = showConfirmCreateSandboxWithKit(w, "biomelab-claude", "claude", "docker.io/example/claude:latest", "/projects/biomelab", []string{"docker.io/example/go:latest"}, func() {}, func() {})
		capture("sandbox", w)
		d.Hide()
		issue := showIssuePreview(w, github.IssueInfo{Number: 82, Title: "Preserve task context when creating worktrees", Body: "Keep the issue requirements available to the next agent.\n\n- Save the issue context in task notes.\n- Preserve existing instruction files.", State: "OPEN", URL: "https://github.com/mdelapenya/biomelab/issues/82"}, "mdelapenya/biomelab", "/projects/biomelab", "main", func() {}, func(string) error { return nil })
		capture("issue", w)
		issue.Hide()
		deps := []sysdeps.Reported{{Check: sysdeps.Check{DisplayName: "GitHub CLI", Reason: "Read pull requests and review status", InstallHint: "brew install gh"}, Result: sysdeps.Result{Status: sysdeps.StatusOK, Version: "gh version 2.76.0"}}, {Check: sysdeps.Check{DisplayName: "Docker Sandbox", Reason: "Create isolated agent environments", InstallHint: "See Docker Sandbox documentation"}, Result: sysdeps.Result{Status: sysdeps.StatusMissing}}}
		cache := sysdeps.NewCache(time.Hour)
		checks := make([]sysdeps.Check, 0, len(deps))
		for index, reported := range deps {
			report := reported
			check := report.Check
			check.Name = fmt.Sprintf("capture-fixture-%d", index)
			check.Probe = func() sysdeps.Result { return report.Result }
			checks = append(checks, check)
		}
		cache.SetChecks(checks)
		cache.Get(nil)
		a.sysdepsCache, a.sysdepsClosed = cache, true
		d = a.showSysDepsDialog()
		capture("dependencies", w)
		d.Hide()
		wt := git.Worktree{Path: t.TempDir(), Branch: "feat/task-context"}
		if _, err := gogit.PlainInit(wt.Path, false); err != nil {
			t.Fatal(err)
		}
		if err := notes.Write(wt.Path, "# Task context\n\nPreserve issue requirements and agent handoff.\n\n- Keep notes available across sessions.\n- Review the PR description before sending."); err != nil {
			t.Fatal(err)
		}
		a.openNoteDialog(wt)
		nw := a.noteWindows[wt.Path]
		capture("notes", nw)
		nw.Close()
		activity := fa.NewWindow("Regent activity — feat/task-context")
		activity.Resize(regentLogWindowInitialSize)
		data := regentLogData{sessionID: "demo-offline", steps: []Step{{Timestamp: time.Date(2026, 10, 6, 10, 30, 0, 0, time.UTC), HumanPrompt: "Preserve task context for the next agent.", AgentReply: "Task notes now contain the issue requirements and the next steps."}}}
		refreshAction := widget.NewButton("Refresh", func() {})
		refreshAction.Importance = widget.HighImportance
		activity.SetContent(regentWindowContent(container.NewVScroll(container.NewVBox(a.buildRegentLogContent(data)...)), refreshAction, widget.NewButton("Export JSON…", func() {}), widget.NewButton("Close", func() { activity.Close() })))
		activity.Show()
		capture("activity", activity)
		activity.Close()
		a.showError(errors.New("The remote could not be reached. Check the connection and try again."))
		capture("error", w)
		a.activeDialog.Hide()
		w.Close()
	}
}

func TestMessageEscapeRestoresWorkflowAtZoom(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	th := newBiomeTheme(VariantLight)
	th.ZoomIn()
	fa.Settings().SetTheme(th)
	w := fa.NewWindow("zoomed workflow")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 460))
	w.Show()
	a := &App{window: w}
	done := a.openDialog()
	workflow := showBranchInput(w, done, func(string) { t.Fatal("message dismissal submitted the worktree") })
	a.activeDialog = workflow
	a.showError(errors.New("An unrelated background operation failed. The branch draft must remain available."))
	popup := w.Canvas().Overlays().Top().(*widget.PopUp)
	if popup.Size().Width > 640 || popup.Size().Height > 460 {
		t.Fatal("zoomed error message exceeded the window")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.activeDialog != workflow || !a.dialogOpen || len(w.Canvas().Overlays().List()) != 1 {
		t.Fatal("Escape discarded the underlying branch workflow")
	}
	a.handleKeyName(fyne.KeyEscape)
	if a.activeDialog != nil || a.dialogOpen || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("workflow Escape did not release ownership")
	}
}

func TestProductionSecondaryActionsFitNarrowZoom(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		t.Run(string(variant), func(t *testing.T) {
			th := newBiomeTheme(variant)
			th.ZoomIn()
			fa.Settings().SetTheme(th)
			w := fa.NewWindow("narrow dependencies")
			defer w.Close()
			w.Resize(fyne.NewSize(640, 480))
			w.Show()
			cache := sysdeps.NewCache(time.Hour)
			cache.SetChecks([]sysdeps.Check{{Name: "offline-fixture", DisplayName: "Long dependency name remains readable", InstallHint: "install dependency", Probe: func() sysdeps.Result { return sysdeps.Result{Status: sysdeps.StatusMissing} }}})
			cache.Get(nil)
			a := &App{window: w, fyneApp: fa, sysdepsCache: cache, sysdepsClosed: true}
			d := a.showSysDepsDialog()
			defer d.Hide()
			popup := w.Canvas().Overlays().Top().(*widget.PopUp)
			if popup.Size().Width > 640 || popup.Size().Height > 480 {
				t.Fatal("zoomed dependencies exceeded parent bounds")
			}
			var close, recheck *dialogButton
			walkPolish(popup.Content, func(obj fyne.CanvasObject) {
				if b, ok := obj.(*dialogButton); ok {
					if b.Text == "Close" {
						close = b
					}
					if b.Text == "Re-check" {
						recheck = b
					}
				}
			})
			assertVisible := func(win fyne.Window, obj fyne.CanvasObject) {
				t.Helper()
				if obj == nil {
					t.Fatal("action missing")
				}
				pos := fa.Driver().AbsolutePositionForObject(obj)
				size := win.Canvas().Size()
				if !obj.Visible() || pos.X < 0 || pos.Y < 0 || pos.X+obj.Size().Width > size.Width || pos.Y+obj.Size().Height > size.Height {
					t.Fatalf("action outside narrow viewport: pos=%v size=%v canvas=%v", pos, obj.Size(), size)
				}
			}
			assertVisible(w, close)
			assertVisible(w, recheck)
			d.Hide()
			wt := git.Worktree{Path: t.TempDir(), Branch: "feat/notes"}
			a.openNoteDialog(wt)
			nw := a.noteWindows[wt.Path]
			defer nw.Close()
			nw.Resize(fyne.NewSize(640, 480))
			_, save := findNoteControls(nw.Content())
			assertVisible(nw, save)
			walkPolish(nw.Content(), func(obj fyne.CanvasObject) {
				if b, ok := obj.(*widget.Button); ok && b.Text == "Cancel" {
					assertVisible(nw, b)
				}
			})
		})
	}
}

func TestOlderWorkflowCompletionPreservesNewerMessage(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	w := fa.NewWindow("background workflow")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))
	w.Show()
	a := &App{window: w}
	done := a.openDialog()
	workflow := showBranchInput(w, done, func(string) {})
	a.activeDialog = workflow
	a.showError(errors.New("Background operation failed"))
	firstMessage := a.activeDialog
	a.showInformation("Another completion", "A second result also needs acknowledgement.")
	message := a.activeDialog
	messageFocus := w.Canvas().Focused()
	workflow.Hide() // a loading stage can complete while an error is on top
	if a.activeDialog != message || !a.dialogOpen || len(w.Canvas().Overlays().List()) != 2 || w.Canvas().Focused() != messageFocus {
		t.Fatalf("older workflow completion changed ownership: active=%T same=%v open=%v overlays=%d generation=%d", a.activeDialog, a.activeDialog == message, a.dialogOpen, len(w.Canvas().Overlays().List()), a.dialogGeneration)
	}
	message.Hide()
	if a.activeDialog != firstMessage || !a.dialogOpen || len(w.Canvas().Overlays().List()) != 1 {
		t.Fatal("dismissal lost the earlier unacknowledged message")
	}
	firstMessage.Hide()
	if a.activeDialog != nil || a.dialogOpen || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("message resurrected the already completed workflow")
	}
}
