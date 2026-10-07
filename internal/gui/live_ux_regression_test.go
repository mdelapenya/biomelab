package gui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func TestBranchCreateRejectsBlankWithoutClosing(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(newBiomeTheme(VariantDark))
	w := fa.NewWindow("create")
	defer w.Close()
	w.SetContent(container.NewStack())
	w.Resize(fyne.NewSize(900, 700))
	w.Show()
	closed := 0
	var submissions []string
	showBranchInput(w, func() { closed++ }, func(name string) { submissions = append(submissions, name) })
	entry, ok := w.Canvas().Focused().(*dialogEntry)
	if !ok {
		t.Fatal("branch entry not focused")
	}
	var create *dialogButton
	walkPolish(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if b, ok := o.(*dialogButton); ok && b.Text == "Create" {
			create = b
		}
	})
	if create == nil {
		t.Fatal("Create button missing")
	}
	c := &keyboardCanvas{Canvas: w.Canvas()}
	for _, blank := range []string{"", " \t "} {
		entry.SetText(blank)
		if !create.Disabled() {
			t.Fatal("blank Create enabled")
		}
		test.Tap(create)
		w.Canvas().Focus(entry)
		c.press(fyne.KeyReturn)
		if closed != 0 || len(submissions) != 0 || w.Canvas().Overlays().Top() == nil {
			t.Fatal("blank submission closed form or submitted")
		}
	}
	entry.SetText("  scratch-branch  ")
	if create.Disabled() {
		t.Fatal("valid branch remained disabled")
	}
	w.Canvas().Focus(entry)
	c.press(fyne.KeyReturn)
	if closed != 1 || len(submissions) != 1 || submissions[0] != "scratch-branch" {
		t.Fatalf("valid submission closed=%d values=%v", closed, submissions)
	}
}

func TestRemoveLastHostRetiresRegistrationAndRefresh(t *testing.T) {
	for _, count := range []int{1, 3} {
		t.Run(strings.Repeat("repo", count), func(t *testing.T) {
			fa := test.NewApp()
			defer fa.Quit()
			fa.Settings().SetTheme(newBiomeTheme(VariantDark))
			w := fa.NewWindow("remove registration")
			defer w.Close()
			a := &App{fyneApp: fa, window: w, configPath: filepath.Join(t.TempDir(), "repos.json"), focus: focusLeft}
			cfg := &config.Config{}
			for range count {
				path := filepath.Join(t.TempDir(), "repo")
				mode := config.ModeEntry{Type: "regular"}
				cfg.Add(path, "scratch", mode)
				state := &RepoState{Worktrees: []git.Worktree{{Path: path, Branch: "main", IsMain: true}}, ActiveMode: &mode}
				rm := NewRefreshManager(nil, nil, nil, nil, nil, nil, time.Hour)
				rm.work = func(context.Context, refreshKind, refreshInputs) ops.RefreshResult { return ops.RefreshResult{} }
				re := &repoEntry{state: state, group: &RepoGroup{Path: path, Name: "scratch", Modes: []config.ModeEntry{mode}}, dashboard: NewDashboard(state), refreshMgr: rm}
				a.repos = append(a.repos, re)
				a.wireDashboardActions(re.dashboard)
				t.Cleanup(rm.Stop)
			}
			if err := config.Save(a.configPath, cfg); err != nil {
				t.Fatal(err)
			}
			a.active = count / 2
			removed := a.repos[a.active]
			a.dashboard, a.refreshMgr = removed.dashboard, removed.refreshMgr
			a.dashSlot = container.NewStack(a.dashboard.Content())
			a.repoPanel = NewRepoPanel(a.collectGroups(), nil)
			w.SetContent(container.NewStack(a.repoPanel.Content(), a.dashSlot))
			w.Resize(fyne.NewSize(1000, 740))
			w.Show()
			removed.refreshMgr.Start()
			generation := removed.refreshMgr.generation
			a.handleRemoveMode()
			c := &keyboardCanvas{Canvas: w.Canvas()}
			setupKeyHandlersWithModifiers(c, a.handleKeyName, a.handleRune, func() fyne.KeyModifier { return c.modifiers })
			c.press(fyne.KeyReturn)
			cfg, err := config.Load(a.configPath)
			if err != nil || cfg.IndexOf(removed.group.Path) >= 0 || len(a.repos) != count-1 || a.hasRepoEntry(removed) || removed.refreshMgr.IsCurrent(generation) {
				t.Fatalf("registration/refresh survived removal: error=%v repos=%d", err, len(a.repos))
			}
			removed.refreshMgr.Resume()
			if removed.refreshMgr.run != nil || !removed.refreshMgr.stopped {
				t.Fatal("removed manager restarted")
			}
			removed.dashboard.OnCreate()
			if a.dialogOpen || w.Canvas().Focused() != nil {
				t.Fatal("stale dashboard opened a dialog or retained focus")
			}
			if count == 1 {
				if a.activeRepo() != nil || a.dashboard != nil || a.refreshMgr != nil || a.repoPanel != nil || a.dashSlot != nil || a.shellSlot != nil || a.focus != focusRight {
					t.Fatal("empty app retained an active workspace")
				}
				var empty bool
				walkPolish(w.Content(), func(o fyne.CanvasObject) {
					if l, ok := o.(*widget.Label); ok && strings.Contains(l.Text, "No repositories registered") {
						empty = true
					}
				})
				if !empty {
					t.Fatal("empty app did not show registration instruction")
				}
				c.press(fyne.KeyA)
				if !a.dialogOpen {
					t.Fatal("empty app lost keyboard Add repository")
				}
			} else if a.active != 1 || a.activeRepo() != a.repos[1] || a.dashboard != a.repos[1].dashboard || a.refreshMgr != a.repos[1].refreshMgr || len(a.repoPanel.groups) != 2 {
				t.Fatal("remaining repository selection/index did not settle")
			}
		})
	}
}

func TestShiftActionEventsNeverInvokePlainCommands(t *testing.T) {
	a, re := newIssueTestApp(t, provider.ProviderUnknown, config.ModeEntry{Type: "regular"})
	re.state.Worktrees = append(re.state.Worktrees, git.Worktree{Path: "/repo/scratch", Branch: "scratch"})
	re.state.SelectedCard = 1
	re.state.HasCLIAvail = true
	re.state.CLIAvail = provider.CLIAvailable
	re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", SandboxName: "fixture"}
	re.state.SandboxStatus = sandbox.StatusStopped
	c := &keyboardCanvas{Canvas: a.window.Canvas(), modifiers: fyne.KeyModifierShift}
	pulls, starts, sends, stops := 0, 0, 0, 0
	setupKeyHandlersWithModifiers(c, func(key fyne.KeyName) {
		switch key {
		case fyne.KeyP:
			pulls++
		case fyne.KeyS:
			starts++
		default:
			a.handleKeyName(key)
		}
	}, func(r rune) {
		switch r {
		case 'P':
			sends++
		case 'S':
			stops++
		}
		a.handleRune(r)
	}, func() fyne.KeyModifier { return c.modifiers })
	c.press(fyne.KeyP)
	c.OnTypedRune()('P')
	if pulls != 0 || sends != 1 || re.state.StatusMessage != "Add a supported GitHub or GitLab remote to create a PR" {
		t.Fatal("Shift+P invoked Pull or lost guarded Send PR")
	}
	status := re.state.StatusMessage
	c.press(fyne.KeyS)
	c.OnTypedRune()('S')
	if starts != 0 || stops != 1 || re.state.StatusMessage != status {
		t.Fatal("Shift+S invoked Start before Stop or bypassed stopped-state guard")
	}
	c.modifiers = 0
	c.press(fyne.KeyP)
	c.press(fyne.KeyS)
	if pulls != 1 || starts != 1 {
		t.Fatal("unshifted Pull/Start lost routing")
	}
	for _, blocked := range []string{"modal", "Projects"} {
		a.dialogOpen = blocked == "modal"
		a.focus = focusRight
		if blocked == "Projects" {
			a.focus = focusLeft
		}
		re.state.StatusMessage = "unchanged"
		c.modifiers = fyne.KeyModifierShift
		c.press(fyne.KeyP)
		c.OnTypedRune()('P')
		if re.state.StatusMessage != "unchanged" || pulls != 1 {
			t.Fatal("uppercase command operated outside Worktrees context")
		}
	}
}

func TestSendPRMissingRemoteIsNotReportedAsMissingCLI(t *testing.T) {
	for _, probed := range []bool{false, true} {
		a, re := newIssueTestApp(t, provider.ProviderUnknown, config.ModeEntry{Type: "regular"})
		re.state.Worktrees = append(re.state.Worktrees, git.Worktree{Path: "/repo/scratch", Branch: "scratch"})
		re.state.SelectedCard = 1
		re.state.HasCLIAvail, re.state.CLIAvail = probed, provider.CLIAvailable
		c := &keyboardCanvas{Canvas: a.window.Canvas(), modifiers: fyne.KeyModifierShift}
		setupKeyHandlersWithModifiers(c, a.handleKeyName, a.handleRune, func() fyne.KeyModifier { return c.modifiers })
		c.press(fyne.KeyP)
		c.OnTypedRune()('P')
		if !re.state.StatusIsError || re.state.StatusMessage != "Add a supported GitHub or GitLab remote to create a PR" || a.dialogOpen {
			t.Fatalf("missing remote produced misleading prerequisite: %q", re.state.StatusMessage)
		}
	}
	// A recognized remote still uses its real CLI prerequisite. These early
	// guards must not perform remote lookup, Pull, or PR publishing.
	a, re := newIssueTestApp(t, provider.ProviderGitHub, config.ModeEntry{Type: "regular"})
	re.state.Worktrees = append(re.state.Worktrees, git.Worktree{Path: "/repo/scratch", Branch: "scratch"})
	re.state.SelectedCard = 1
	re.state.HasCLIAvail, re.state.CLIAvail = true, provider.CLINotFound
	a.handleSendPR()
	if re.state.StatusMessage != "CLI tool required for PR creation" || !re.state.StatusIsError || a.dialogOpen {
		t.Fatal("supported provider lost CLI prerequisite guard")
	}
}
