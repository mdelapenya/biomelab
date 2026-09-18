package gui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/config"
	gitrepo "github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
)

func TestIssueFlowEndToEndWithInjectedOperations(t *testing.T) {
	for _, mode := range []config.ModeEntry{{Type: "regular"}, {Type: "sandbox", Agent: "claude"}} {
		t.Run(mode.Type, func(t *testing.T) {
			a, re := newIssueTestApp(t, provider.ProviderGitHub, mode)
			var fetched, created atomic.Int32
			createStarted := make(chan struct{})
			releaseCreate := make(chan struct{})
			a.issueDeps = &issueDependencies{
				parse: github.ParseIssueRef,
				fetch: func(_ context.Context, dir string, ref github.IssueRef) (github.IssueInfo, error) {
					fetched.Add(1)
					if dir != "/repo" || ref.Number != 82 {
						t.Errorf("fetch got dir=%q ref=%+v", dir, ref)
					}
					return github.IssueInfo{Number: 82, Title: "Issue title", Body: "Body", URL: "https://github.com/o/r/issues/82", State: "OPEN"}, nil
				},
				create: func(_ *gitrepo.Repository, _ github.IssueInfo, branch string) ops.CreateIssueWorktreeResult {
					created.Add(1)
					close(createStarted)
					<-releaseCreate
					return ops.CreateIssueWorktreeResult{BranchName: branch, WtPath: "/repo/.biomelab-worktrees/" + branch}
				},
			}
			a.handleCreateFromIssue()
			previewReady := make(chan struct{})
			complete := make(chan struct{})
			a.issueFlow.onPreview = func() { close(previewReady) }
			a.issueFlow.onComplete = func() { close(complete) }
			popup, ok := a.window.Canvas().Overlays().Top().(*widget.PopUp)
			if !ok {
				t.Fatal("input dialog did not open")
			}
			var entry *dialogEntry
			var lookup *dialogButton
			walkSetupContent(popup.Content, func(obj fyne.CanvasObject) {
				switch obj := obj.(type) {
				case *dialogEntry:
					entry = obj
				case *dialogButton:
					if obj.Text == "Look Up" {
						lookup = obj
					}
				}
			})
			if entry == nil || lookup == nil {
				t.Fatal("input controls not found")
			}
			entry.SetText("82")
			lookup.OnTapped()
			<-previewReady
			if fetched.Load() != 1 || !a.dialogOpen || a.activeDialog != a.issueFlow.preview.Dialog {
				t.Fatalf("lookup transition failed: fetched=%d open=%v", fetched.Load(), a.dialogOpen)
			}
			a.issueFlow.preview.create.OnTapped()
			<-createStarted
			close(releaseCreate)
			<-complete
			if created.Load() != 1 || !strings.Contains(re.state.StatusMessage, "Created issue-82-issue-title") {
				t.Fatalf("creation did not complete: creates=%d status=%q", created.Load(), re.state.StatusMessage)
			}
		})
	}
}

func newIssueTestApp(t *testing.T, prov provider.Provider, mode config.ModeEntry) (*App, *repoEntry) {
	t.Helper()
	fa := test.NewApp()
	t.Cleanup(fa.Quit)
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("issue flow")
	w.Resize(issueDialogSize)
	t.Cleanup(w.Close)
	state := &RepoState{
		Provider:   prov,
		ActiveMode: &mode,
		Worktrees:  []gitrepo.Worktree{{Branch: "main", Path: "/repo"}},
	}
	re := &repoEntry{group: &RepoGroup{Path: "/repo", Name: "repo", Modes: []config.ModeEntry{mode}}, state: state}
	re.dashboard = NewDashboard(state)
	a := &App{window: w, repos: []*repoEntry{re}, active: 0, dashboard: re.dashboard}
	return a, re
}

func waitIssueTest(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for issue flow")
}

func TestHandleCreateFromIssueGating(t *testing.T) {
	t.Run("linked card", func(t *testing.T) {
		a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
		re.state.Worktrees = append(re.state.Worktrees, gitrepo.Worktree{Branch: "task"})
		re.state.SelectedCard = 1
		a.handleCreateFromIssue()
		if a.dialogOpen {
			t.Fatal("linked-card shortcut opened issue dialog")
		}
	})
	t.Run("unsupported provider", func(t *testing.T) {
		a, re := newIssueTestApp(t, provider.ProviderGitLab, config.ModeEntry{Type: "regular"})
		a.handleCreateFromIssue()
		if a.dialogOpen {
			t.Fatal("GitLab shortcut opened GitHub issue dialog")
		}
		if !strings.Contains(re.state.StatusMessage, "requires a GitHub") || !re.state.StatusIsError {
			t.Fatalf("unexpected provider status: %q", re.state.StatusMessage)
		}
	})
}

func TestIssueLookupCancellationIgnoresLateResult(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	release := make(chan struct{})
	deps := &issueDependencies{fetch: func(context.Context, string, github.IssueRef) (github.IssueInfo, error) {
		<-release
		return github.IssueInfo{Number: 82, Title: "late", URL: "https://github.com/o/r/issues/82", State: "OPEN"}, nil
	}}
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
	a.issueFlow = f
	f.startLookup(deps, github.IssueRef{Number: 82}, "82")
	loading := a.activeDialog
	loading.Hide()
	close(release)
	waitIssueTest(t, func() bool { return !a.dialogOpen })
	time.Sleep(20 * time.Millisecond)
	if f.preview != nil {
		t.Fatal("late lookup reopened a preview after cancellation")
	}
}

func TestStaleInputCallbackCannotReplaceNewFlowDialog(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	var fetches atomic.Int32
	deps := &issueDependencies{fetch: func(context.Context, string, github.IssueRef) (github.IssueInfo, error) {
		fetches.Add(1)
		return github.IssueInfo{}, nil
	}}
	stale := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
	current := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
	a.issueFlow = current
	stale.startLookup(deps, github.IssueRef{Number: 82}, "82")
	if fetches.Load() != 0 || a.dialogOpen {
		t.Fatalf("stale callback opened lookup: fetches=%d open=%v", fetches.Load(), a.dialogOpen)
	}
}

func TestIssuePreviewDoesNotReplaceUnrelatedDialog(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
	a.issueFlow = f
	unrelated := &mockDialog{}
	a.dialogOpen = true
	a.activeDialog = unrelated
	f.showPreviewIfOwned(&issueDependencies{}, github.IssueInfo{Number: 82, Title: "late", URL: "https://github.com/o/r/issues/82", State: "OPEN"})
	if f.preview != nil || a.activeDialog != unrelated || unrelated.hidden != 0 {
		t.Fatal("late preview replaced or dismissed an unrelated dialog")
	}
}

func TestIssueLookupDoesNotCrossRepoOrMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*App, *repoEntry)
	}{
		{name: "repo switch", change: func(a *App, _ *repoEntry) {
			otherState := &RepoState{Provider: provider.ProviderGitHub, ActiveMode: &config.ModeEntry{Type: "regular"}, Worktrees: []gitrepo.Worktree{{Branch: "main"}}}
			other := &repoEntry{group: &RepoGroup{Path: "/other"}, state: otherState, dashboard: NewDashboard(otherState)}
			a.repos = append(a.repos, other)
			a.active = 1
		}},
		{name: "mode switch", change: func(_ *App, re *repoEntry) { re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", Agent: "claude"} }},
		{name: "repo removal", change: func(a *App, _ *repoEntry) { a.repos = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
			release := make(chan struct{})
			deps := &issueDependencies{fetch: func(context.Context, string, github.IssueRef) (github.IssueInfo, error) {
				<-release
				return github.IssueInfo{Number: 82, Title: "issue", URL: "https://github.com/o/r/issues/82", State: "OPEN"}, nil
			}}
			f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
			a.issueFlow = f
			f.startLookup(deps, github.IssueRef{Number: 82}, "82")
			tc.change(a, re)
			close(release)
			time.Sleep(30 * time.Millisecond)
			if f.preview != nil {
				t.Fatal("lookup opened preview for a stale origin")
			}
			if re.state.StatusMessage != "" {
				t.Fatalf("late lookup mutated status: %q", re.state.StatusMessage)
			}
		})
	}
}

func TestIssuePreviewValidationAndDoubleSubmit(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "sandbox", Agent: "claude"})
	started := make(chan struct{})
	release := make(chan struct{})
	var creates atomic.Int32
	deps := &issueDependencies{create: func(*gitrepo.Repository, github.IssueInfo, string) ops.CreateIssueWorktreeResult {
		creates.Add(1)
		close(started)
		<-release
		return ops.CreateIssueWorktreeResult{BranchName: "issue-82-title", WtPath: "/repo/.biomelab-worktrees/issue-82-title"}
	}}
	var refreshes atomic.Int32
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() { refreshes.Add(1) }}
	a.issueFlow = f
	issue := github.IssueInfo{Number: 82, Title: "Title", Body: strings.Repeat("long body ", 1000), URL: "https://github.com/o/r/issues/82", State: "CLOSED"}
	f.showPreview(deps, issue)
	f.preview.branch.SetText("bad/name")
	f.preview.create.OnTapped()
	if !strings.Contains(f.preview.error.Text, "only letters") || creates.Load() != 0 {
		t.Fatalf("invalid branch was not rejected: %q", f.preview.error.Text)
	}
	f.preview.branch.SetText("issue-82-title")
	f.preview.create.OnTapped()
	<-started
	f.preview.create.OnTapped()
	if creates.Load() != 1 {
		t.Fatalf("create called %d times", creates.Load())
	}
	close(release)
	waitIssueTest(t, func() bool { return refreshes.Load() == 1 })
	if re.state.StatusIsError || !strings.Contains(re.state.StatusMessage, "Created issue-82-title") {
		t.Fatalf("unexpected success status: %q", re.state.StatusMessage)
	}
}

func TestIssueCreationPartialSuccessRefreshesOrigin(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	var refreshes atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	deps := &issueDependencies{create: func(*gitrepo.Repository, github.IssueInfo, string) ops.CreateIssueWorktreeResult {
		close(started)
		<-release
		return ops.CreateIssueWorktreeResult{BranchName: "issue-9", WtPath: "/repo/wt", NotesErr: errors.New("bootstrap failed")}
	}}
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() { refreshes.Add(1) }}
	complete := make(chan struct{})
	f.onComplete = func() { close(complete) }
	a.issueFlow = f
	f.showPreview(deps, github.IssueInfo{Number: 9, Title: "x", URL: "https://github.com/o/r/issues/9", State: "OPEN"})
	f.preview.create.OnTapped()
	<-started
	close(release)
	<-complete
	if refreshes.Load() != 1 {
		t.Fatalf("origin refreshed %d times", refreshes.Load())
	}
	if !re.state.StatusIsError || !strings.Contains(re.state.StatusMessage, "Created issue-9") || !strings.Contains(re.state.StatusMessage, "bootstrap failed") {
		t.Fatalf("partial success was not reported: %q", re.state.StatusMessage)
	}
	if a.activeDialog != nil {
		a.activeDialog.Hide()
	}
}

func TestIssueCreationReportsAfterDialogClosedAndNewFlowStarted(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	started := make(chan struct{})
	release := make(chan struct{})
	deps := &issueDependencies{create: func(*gitrepo.Repository, github.IssueInfo, string) ops.CreateIssueWorktreeResult {
		close(started)
		<-release
		return ops.CreateIssueWorktreeResult{BranchName: "issue-12", WtPath: "/repo/wt"}
	}}
	var refreshes atomic.Int32
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() { refreshes.Add(1) }}
	a.issueFlow = f
	f.showPreview(deps, github.IssueInfo{Number: 12, Title: "x", URL: "https://github.com/o/r/issues/12", State: "OPEN"})
	f.preview.create.OnTapped()
	<-started
	f.preview.Hide()
	a.issueFlow = &issueFlow{app: a, origin: re, modeKey: issueModeKey(re)}
	close(release)
	waitIssueTest(t, func() bool { return refreshes.Load() == 1 })
	if got := re.state.StatusMessage; !strings.Contains(got, "Created issue-12") {
		t.Fatalf("completed creation result was lost: %q", got)
	}
}

func TestIssuePreviewRejectsCreateAfterOriginChanges(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	var creates atomic.Int32
	deps := &issueDependencies{create: func(*gitrepo.Repository, github.IssueInfo, string) ops.CreateIssueWorktreeResult {
		creates.Add(1)
		return ops.CreateIssueWorktreeResult{}
	}}
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: func() {}}
	a.issueFlow = f
	f.showPreview(deps, github.IssueInfo{Number: 12, Title: "x", URL: "https://github.com/o/r/issues/12", State: "OPEN"})
	re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", Agent: "claude"}
	f.preview.create.OnTapped()
	if creates.Load() != 0 || !strings.Contains(f.preview.error.Text, "no longer selected") {
		t.Fatalf("stale preview submitted: creates=%d error=%q", creates.Load(), f.preview.error.Text)
	}
}
