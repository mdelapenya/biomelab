package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	gogit "github.com/go-git/go-git/v6"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/kits"
	"github.com/mdelapenya/biomelab/internal/ops"
)

func completionFixture(t *testing.T) *App {
	t.Helper()
	fa := test.NewApp()
	t.Cleanup(fa.Quit)
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("completion")
	t.Cleanup(w.Close)
	a := &App{fyneApp: fa, window: w, active: -1, configPath: filepath.Join(t.TempDir(), "repos.json")}
	cfg := &config.Config{}
	for range 2 {
		path := t.TempDir()
		if _, err := gogit.PlainInit(path, false); err != nil {
			t.Fatal(err)
		}
		repo, err := git.OpenRepository(path)
		if err != nil {
			t.Fatal(err)
		}
		mode := config.ModeEntry{Type: "regular"}
		state := &RepoState{ActiveMode: &mode, Worktrees: []git.Worktree{{Path: path, Branch: "main", IsMain: true}, {Path: filepath.Join(path, "scratch"), Branch: "scratch"}}}
		rm := NewRefreshManager(nil, nil, nil, nil, nil, nil, time.Hour)
		rm.work = func(context.Context, refreshKind, refreshInputs) ops.RefreshResult { return ops.RefreshResult{} }
		t.Cleanup(rm.Stop)
		re := &repoEntry{repo: repo, state: state, dashboard: NewDashboard(state), refreshMgr: rm, group: &RepoGroup{Path: path, Name: "fixture", Modes: []config.ModeEntry{mode}}}
		a.repos = append(a.repos, re)
		cfg.Add(path, "fixture", mode)
	}
	if err := config.Save(a.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	a.repoPanel = NewRepoPanel(a.collectGroups(), nil)
	a.dashSlot = container.NewStack(a.repos[0].dashboard.Content())
	w.SetContent(newShellLayout(a.repoPanel.Content(), a.dashSlot, nil))
	w.Resize(fyne.NewSize(1000, 740))
	w.Show()
	a.switchMode(0, 0)
	return a
}

func TestSandboxCompletionPreservesNewerWorkspaceNavigation(t *testing.T) {
	for _, navigation := range []string{"stayed", "other repository", "away and back", "newer origin mode", "other repository with modal"} {
		t.Run(navigation, func(t *testing.T) {
			a := completionFixture(t)
			origin, other := a.repos[0], a.repos[1]
			if navigation == "newer origin mode" {
				origin.group.Modes = []config.ModeEntry{newSandboxMode(origin.group.Path, "claude"), newSandboxMode(origin.group.Path, "gemini")}
				cfg, err := config.Load(a.configPath)
				if err != nil {
					t.Fatal(err)
				}
				cfg.Repos[0].Modes = origin.group.Modes
				if err := config.Save(a.configPath, cfg); err != nil {
					t.Fatal(err)
				}
				a.switchMode(0, 0)
			}
			started := a.workspaceGeneration
			mode := newSandboxMode(origin.group.Path, "shell")
			other.state.StatusMessage = "Other operation is current"
			if navigation != "stayed" && navigation != "newer origin mode" {
				a.switchMode(1, 0)
				other.state.SelectedCard = 1
			}
			if navigation == "newer origin mode" {
				a.switchMode(0, 1)
			}
			if navigation == "away and back" {
				a.switchMode(0, 0)
				origin.state.SelectedCard = 1
			}
			if navigation == "other repository with modal" {
				done := a.openDialog()
				a.activeDialog = showFetchPRInput(a.window, done, func(string) { t.Fatal("background completion submitted modal") })
			}
			current, dashboard, manager := a.activeRepo(), a.dashboard, a.refreshMgr
			selected, generation := current.state.SelectedCard, a.workspaceGeneration
			modal, focused := a.activeDialog, a.window.Canvas().Focused()
			selectedKits := []kits.Kit{{Name: "playwright", Kind: kits.KindMixin}}
			a.completeProjectSandbox(origin.group.Path, origin.group.Name, mode, mode.SandboxName, true, selectedKits, started, origin)
			cfg, err := config.Load(a.configPath)
			if err != nil {
				t.Fatal(err)
			}
			modes := cfg.Repos[cfg.IndexOf(origin.group.Path)].Modes
			registered, expectedMode, expectedIndex := modes[len(modes)-1], mode.SandboxName, 0
			if navigation == "newer origin mode" {
				expectedMode, expectedIndex = modes[1].SandboxName, 1
			}
			if registered.Type != "sandbox" || registered.SandboxName != mode.SandboxName || len(registered.Kits) != 1 || origin.state.ActiveMode.SandboxName != expectedMode || origin.group.ActiveMode != expectedIndex {
				t.Fatal("completed sandbox registration or valid origin mode was lost")
			}
			if origin.state.StatusMessage != "Created "+mode.SandboxName || origin.state.StatusIsError {
				t.Fatal("creation status did not stay with origin")
			}
			if navigation == "stayed" {
				if a.activeRepo() != origin || a.workspaceGeneration <= generation || a.dashboard != origin.dashboard {
					t.Fatal("same-context completion lost expected mode activation")
				}
			} else if a.activeRepo() != current || a.dashboard != dashboard || a.refreshMgr != manager || current.state.SelectedCard != selected || a.workspaceGeneration != generation || other.state.StatusMessage != "Other operation is current" {
				t.Fatal("completion stole newer repository/card/manager intent or status")
			}
			if a.activeDialog != modal || a.window.Canvas().Focused() != focused || (modal != nil && !a.dialogOpen) {
				t.Fatal("completion disturbed newer modal ownership/focus")
			}
		})
	}
}

func TestFetchPRCompletionClearsPriorErrorAndStaysWithOrigin(t *testing.T) {
	a := completionFixture(t)
	origin, other := a.repos[0], a.repos[1]
	origin.state.StatusMessage, origin.state.StatusIsError = "invalid worktree name: bad branch", true
	a.setRepoStatus(origin, "Fetching PR 93…", false)
	if origin.state.StatusIsError {
		t.Fatal("fetch start retained previous error")
	}
	a.switchMode(1, 0)
	other.state.StatusMessage = "Other operation is current"
	a.applyFetchPRResult(origin, ops.FetchPRResult{BranchName: "pr-93", WtPath: filepath.Join(origin.group.Path, "pr-93")})
	if origin.state.StatusMessage != "Fetched pr-93" || origin.state.StatusIsError || a.activeRepo() != other || other.state.StatusMessage != "Other operation is current" {
		t.Fatal("successful fetch retained stale error or changed another repository")
	}
	a.applyFetchPRResult(origin, ops.FetchPRResult{Err: errors.New("PR lookup failed")})
	if origin.state.StatusMessage != "PR lookup failed" || !origin.state.StatusIsError {
		t.Fatal("failed fetch lost actionable error")
	}
	a.removeRepoEntry(origin)
	a.applyFetchPRResult(origin, ops.FetchPRResult{BranchName: "late-pr"})
	if origin.state.StatusMessage != "PR lookup failed" || a.activeRepo() != other || other.state.StatusMessage != "Other operation is current" {
		t.Fatal("removed origin accepted stale fetch completion")
	}
}

func TestSandboxCompletionDoesNotRestoreRemovedRegistration(t *testing.T) {
	for _, scenario := range []string{"other repository", "empty app", "same path re-registered", "newer modal"} {
		for _, created := range []bool{false, true} {
			t.Run(scenario+fmt.Sprint("/created=", created), func(t *testing.T) {
				a := completionFixture(t)
				origin := a.repos[0]
				started := a.workspaceGeneration
				mode := newSandboxMode(origin.group.Path, "shell")
				a.switchMode(1, 0)
				cfg, err := config.Load(a.configPath)
				if err != nil {
					t.Fatal(err)
				}
				cfg.Remove(origin.group.Path)
				a.removeRepoEntry(origin)
				if scenario == "empty app" {
					cfg.Remove(a.repos[0].group.Path)
					a.removeRepoEntry(a.repos[0])
				}
				if scenario == "same path re-registered" {
					// A fresh registration at the same path is a different user intent.
					replacement := *origin
					group := *origin.group
					replacement.group = &group
					a.repos = append(a.repos, &replacement)
					cfg.Add(group.Path, group.Name, config.ModeEntry{Type: "regular"})
				}
				if err := config.Save(a.configPath, cfg); err != nil {
					t.Fatal(err)
				}
				if scenario == "newer modal" {
					done := a.openDialog()
					a.activeDialog = showFetchPRInput(a.window, done, func(string) { t.Fatal("completion submitted modal") })
				}
				current, dashboard, manager := a.activeRepo(), a.dashboard, a.refreshMgr
				modal, focused := a.activeDialog, a.window.Canvas().Focused()
				count, generation, overlays := len(a.repos), a.workspaceGeneration, len(a.window.Canvas().Overlays().List())
				if current != nil {
					current.state.StatusMessage = "Newer operation"
				}
				before, err := os.ReadFile(a.configPath)
				if err != nil {
					t.Fatal(err)
				}
				a.completeProjectSandbox(origin.group.Path, origin.group.Name, mode, mode.SandboxName, created, nil, started, origin)
				// Both async routes use this handler, including failures and
				// existing-kit conflicts that would otherwise open a modal.
				for _, resultErr := range []error{nil, errors.New("late sandbox failure"), &ops.ExistingSandboxWithKitsError{Name: mode.SandboxName}} {
					a.applyProjectSandboxResult(origin.group.Path, origin.group.Name, mode, mode.SandboxName, created, nil, started, origin, resultErr)
				}
				after, err := os.ReadFile(a.configPath)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("late completion changed repository registration")
				}
				if len(a.repos) != count || a.activeRepo() != current || a.dashboard != dashboard || a.refreshMgr != manager || a.workspaceGeneration != generation {
					t.Fatal("late completion resurrected or activated a removed registration")
				}
				if a.activeDialog != modal || a.window.Canvas().Focused() != focused || len(a.window.Canvas().Overlays().List()) != overlays || (modal != nil && !a.dialogOpen) {
					t.Fatal("late completion disturbed newer dialog or focus")
				}
				if current != nil && current.state.StatusMessage != "Newer operation" {
					t.Fatal("late completion overwrote another repository's status")
				}
			})
		}
	}
}

func TestSandboxCompletionRegistersOriginallyNewRepository(t *testing.T) {
	a := completionFixture(t)
	path := t.TempDir()
	if _, err := gogit.PlainInit(path, false); err != nil {
		t.Fatal(err)
	}
	if a.projectRepo(path) != nil {
		t.Fatal("new repository unexpectedly has an initiating registration")
	}
	started := a.workspaceGeneration
	a.switchMode(1, 0)
	current := a.activeRepo()
	mode := newSandboxMode(path, "shell")
	a.completeProjectSandbox(path, "new fixture", mode, mode.SandboxName, true, nil, started, nil)
	registered := a.projectRepo(path)
	if registered == nil {
		t.Fatal("legitimate new repository was not registered")
	}
	t.Cleanup(registered.refreshMgr.Stop)
	cfg, err := config.Load(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	idx := cfg.IndexOf(path)
	if idx < 0 || len(cfg.Repos[idx].Modes) != 1 || cfg.Repos[idx].Modes[0].SandboxName != mode.SandboxName || registered.state.StatusMessage != "Created "+mode.SandboxName {
		t.Fatal("legitimate new sandbox registration or status was lost")
	}
	if a.activeRepo() != current {
		t.Fatal("new repository registration stole newer navigation")
	}
}
