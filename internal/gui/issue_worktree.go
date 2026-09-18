package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	gitrepo "github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
)

type issueDependencies struct {
	parse  func(string) (github.IssueRef, error)
	fetch  func(context.Context, string, github.IssueRef) (github.IssueInfo, error)
	create func(*gitrepo.Repository, github.IssueInfo, string) ops.CreateIssueWorktreeResult
}

func defaultIssueDependencies() *issueDependencies {
	return &issueDependencies{parse: github.ParseIssueRef, fetch: github.FetchIssue, create: ops.CreateWorktreeFromIssue}
}

type issueFlow struct {
	app        *App
	origin     *repoEntry
	modeKey    string
	refresh    func()
	cancel     context.CancelFunc
	lookupID   uint64
	creating   bool
	preview    *issuePreviewDialog
	onPreview  func()
	onComplete func()
}

func (a *App) handleCreateFromIssue() {
	re := a.activeRepo()
	if re == nil || re.state.SelectedCard != 0 {
		return
	}
	if re.state.Provider != provider.ProviderGitHub {
		a.setProjectStatus(re.group.Path, "Creating a worktree from an issue currently requires a GitHub repository.", true)
		return
	}
	deps := a.issueDeps
	if deps == nil {
		deps = defaultIssueDependencies()
	}
	refresh := func() {}
	if re.refreshMgr != nil {
		refresh = re.refreshMgr.TriggerQuick
	}
	f := &issueFlow{app: a, origin: re, modeKey: issueModeKey(re), refresh: refresh}
	a.issueFlow = f
	f.showInput(deps)
}

func (f *issueFlow) showInput(deps *issueDependencies) {
	done := f.app.openDialog()
	var acceptedRef github.IssueRef
	var acceptedDisplay string
	input := showIssueInput(f.app.window, done, func(raw string) error {
		ref, err := deps.parse(raw)
		if err != nil {
			return err
		}
		acceptedRef = ref
		acceptedDisplay = strings.TrimSpace(raw)
		return nil
	}, func() {
		f.startLookup(deps, acceptedRef, acceptedDisplay)
	})
	f.app.activeDialog = input.Dialog
	f.focusWhenCurrent(input.Dialog, input.entry)
}

func (f *issueFlow) startLookup(deps *issueDependencies, ref github.IssueRef, display string) {
	if f.app.issueFlow != f || f.app.dialogOpen || !f.originPresentAndActive() {
		return
	}
	f.lookupID++
	id := f.lookupID
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	done := f.app.openDialog()
	loading := showIssueLoading(f.app.window, display, done, cancel)
	f.app.activeDialog = loading
	go func() {
		issue, err := deps.fetch(ctx, f.origin.group.Path, ref)
		fyne.Do(func() { f.lookupComplete(deps, id, ctx, loading, ref, issue, err) })
	}()
}

func (f *issueFlow) lookupComplete(deps *issueDependencies, id uint64, ctx context.Context, loading dialog.Dialog,
	ref github.IssueRef, issue github.IssueInfo, err error) {
	if id != f.lookupID || ctx.Err() != nil || f.app.issueFlow != f {
		return
	}
	loading.Hide()
	if !f.originPresentAndActive() {
		return
	}
	if err != nil {
		f.app.setProjectStatus(f.origin.group.Path, ops.FirstNonEmptyLine(err.Error()), true)
		fyne.Do(func() {
			if f.app.issueFlow == f && f.originPresentAndActive() && !f.app.dialogOpen && f.app.activeDialog == nil {
				f.app.showIssueError(err)
			}
		})
		return
	}
	fyne.Do(func() {
		f.showPreviewIfOwned(deps, issue)
	})
}

func (f *issueFlow) showPreviewIfOwned(deps *issueDependencies, issue github.IssueInfo) {
	if f.app.issueFlow != f || !f.originPresentAndActive() || f.app.dialogOpen || f.app.activeDialog != nil {
		return
	}
	f.showPreview(deps, issue)
}

func (f *issueFlow) showPreview(deps *issueDependencies, issue github.IssueInfo) {
	source := sourceRepoFromIssueURL(issue.URL)
	head := "unknown"
	if wt := f.origin.state.MainWorktree(); wt != nil && wt.Branch != "" {
		head = wt.Branch
	}
	done := f.app.openDialog()
	p := showIssuePreview(f.app.window, issue, source, f.origin.group.Path, head, done, func(branch string) error {
		if !f.originPresentAndActive() {
			return errors.New("the originating repository or mode is no longer selected")
		}
		if err := ops.ValidateIssueBranch(branch); err != nil {
			return err
		}
		if f.creating {
			return errors.New("this worktree is already being created")
		}
		f.creating = true
		go f.create(deps, issue, branch)
		return nil
	})
	f.preview = &p
	f.app.activeDialog = p.Dialog
	f.focusWhenCurrent(p.Dialog, p.branch)
	if f.onPreview != nil {
		f.onPreview()
	}
}

func (f *issueFlow) create(deps *issueDependencies, issue github.IssueInfo, branch string) {
	result := deps.create(f.origin.repo, issue, branch)
	fyne.Do(func() {
		f.creating = false
		if f.app.issueFlow == f && f.app.dialogOpen && f.preview != nil && f.app.activeDialog == f.preview.Dialog {
			f.preview.Hide()
		}
		if !f.originPresent() {
			return
		}
		var message string
		var isErr bool
		switch {
		case result.Err != nil:
			message, isErr = "Create issue worktree failed: "+ops.FirstNonEmptyLine(result.Err.Error()), true
		case result.NotesErr != nil:
			message, isErr = fmt.Sprintf("Created %s at %s, but issue context or agent instructions need repair: %s", result.BranchName, result.WtPath, ops.FirstNonEmptyLine(result.NotesErr.Error())), true
		default:
			message = fmt.Sprintf("Created %s from issue #%d", result.BranchName, issue.Number)
		}
		f.app.setProjectStatus(f.origin.group.Path, message, isErr)
		if result.Err == nil {
			f.refresh()
		}
		if result.Err != nil {
			fyne.Do(func() {
				if f.canShowCompletionDialog() {
					f.app.showIssueError(result.Err)
				}
			})
		} else if result.NotesErr != nil {
			fyne.Do(func() {
				if f.canShowCompletionDialog() {
					f.app.showIssueError(fmt.Errorf("worktree created at %s, but setup was incomplete: %w", result.WtPath, result.NotesErr))
				}
			})
		}
		if f.onComplete != nil {
			f.onComplete()
		}
	})
}

func (f *issueFlow) canShowCompletionDialog() bool {
	return f.app.issueFlow == f && f.originPresentAndActive() && !f.app.dialogOpen && f.app.activeDialog == nil
}

func (f *issueFlow) originPresent() bool {
	for _, re := range f.app.repos {
		if re == f.origin {
			return true
		}
	}
	return false
}

func (f *issueFlow) originPresentAndActive() bool {
	if f.app.activeRepo() != f.origin || issueModeKey(f.origin) != f.modeKey {
		return false
	}
	return f.originPresent()
}

func issueModeKey(re *repoEntry) string {
	if re == nil || re.state == nil || re.state.ActiveMode == nil {
		return ""
	}
	m := re.state.ActiveMode
	return m.Type + "\x00" + m.Agent + "\x00" + m.SandboxName
}

func (f *issueFlow) focusWhenCurrent(d dialog.Dialog, target fyne.Focusable) {
	fyne.Do(func() {
		if f.app.dialogOpen && f.app.activeDialog == d {
			f.app.window.Canvas().Focus(target)
		}
	})
}

func sourceRepoFromIssueURL(raw string) string {
	const prefix = "https://github.com/"
	rest := strings.TrimPrefix(raw, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && rest != raw {
		return parts[0] + "/" + parts[1]
	}
	return "GitHub repository"
}
