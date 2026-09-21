package gui

import (
	"errors"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

func noTerminal([]string, string, []*terminal.Session) (*terminal.Session, error) { return nil, nil }

func TestTerminalActionCoalescesAndReportsFailure(t *testing.T) {
	wt := git.Worktree{Path: "/repo/work", Branch: "feature"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	dispatch := make(chan func(), 10)
	var opens atomic.Int32
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		find: noTerminal,
		open: func(dir, command, identifier string) (*terminal.Session, error) {
			if dir != wt.Path || command != "" || identifier != wt.Branch {
				t.Error("wrong terminal target")
			}
			opens.Add(1)
			started <- struct{}{}
			<-release
			return nil, errors.New("terminal unavailable")
		}, dispatch: func(f func()) { dispatch <- f },
	}}
	t.Cleanup(func() { close(release) })
	a.handleEnter()
	receiveRefresh(t, started)
	for range 100 {
		a.handleEnter()
	}
	if opens.Load() != 1 {
		t.Fatal("duplicate pending launch")
	}
	other := &repoEntry{state: &RepoState{}}
	a.repos = append(a.repos, other)
	a.active = 1
	release <- struct{}{}
	receiveRefresh(t, dispatch)()
	if re.state.StatusMessage != "terminal unavailable" || !re.state.StatusIsError {
		t.Fatal("originating repo did not receive error")
	}
	if other.state.StatusMessage != "" {
		t.Fatal("failure misdirected to active repo")
	}
	if len(a.terminalActions) != 0 || opens.Load() != 1 {
		t.Fatal("failure retried automatically")
	}
	a.active = 0
	a.handleEnter()
	receiveRefresh(t, started)
	release <- struct{}{}
	receiveRefresh(t, dispatch)()
	if opens.Load() != 2 {
		t.Fatal("deliberate retry was blocked")
	}
}

func TestTerminalActionReusesRegularAndSandboxSessions(t *testing.T) {
	for _, mode := range []string{"regular", "sandbox-main", "sandbox-linked"} {
		t.Run(mode, func(t *testing.T) {
			wt := git.Worktree{Path: "/repo/work", Branch: "feature", IsMain: mode == "sandbox-main"}
			re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
			if strings.HasPrefix(mode, "sandbox") {
				re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", SandboxName: "box", Agent: "claude"}
			}
			dispatch := make(chan func(), 1)
			var opens, activations, finds int
			live := true
			first := &terminal.Session{}
			activationErr := error(nil)
			a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
				find: func(paths []string, path string, _ []*terminal.Session) (*terminal.Session, error) {
					finds++
					if path != wt.Path || len(paths) != 1 {
						t.Error("wrong discovery paths")
					}
					return nil, nil
				},
				open: func(dir, command, title string) (*terminal.Session, error) {
					opens++
					if mode == "regular" && (dir != wt.Path || command != "" || title != wt.Branch) {
						t.Error("wrong regular launch")
					}
					if mode == "sandbox-main" && command != "sbx run --name box" {
						t.Errorf("wrong main attach: %s", command)
					}
					if mode == "sandbox-linked" && (!strings.Contains(command, "exec") || !strings.Contains(command, "claude") || !strings.Contains(command, wt.Path)) {
						t.Errorf("wrong linked attach: %s", command)
					}
					return first, nil
				},
				activate: func(s *terminal.Session) (bool, error) {
					activations++
					if s != first {
						t.Error("wrong session")
					}
					return live, activationErr
				},
				dispatch: func(f func()) { dispatch <- f },
			}}
			act := func() { a.handleEnter(); receiveRefresh(t, dispatch)() }
			act()
			// Periodic detection remains empty, as with cd outside the worktree or sbx.
			for range 3 {
				act()
			}
			if opens != 1 || activations != 3 {
				t.Fatalf("opens=%d activations=%d", opens, activations)
			}
			activationErr = errors.New("Automation permission denied")
			act()
			act()
			if opens != 1 || !strings.Contains(re.state.StatusMessage, "permission") {
				t.Fatal("activation failure launched a duplicate")
			}
			activationErr = nil
			live = false
			act()
			if opens != 2 {
				t.Fatal("closed session did not get exactly one replacement")
			}
			if re.state.StatusIsError || re.state.StatusMessage != "" {
				t.Fatal("successful replacement retained an old terminal error")
			}
			if mode != "regular" && finds != 0 {
				t.Fatal("sandbox incorrectly inferred association from host CWD")
			}
		})
	}
}

func TestTerminalActionFreshDiscoveryAndInspectionErrors(t *testing.T) {
	for _, failure := range []bool{false, true} {
		wt := git.Worktree{Path: "/repo"}
		re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
		dispatch := make(chan func(), 1)
		session := &terminal.Session{}
		activations := 0
		a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
			find: func([]string, string, []*terminal.Session) (*terminal.Session, error) {
				if failure {
					return nil, errors.New("process inspection unavailable")
				}
				return session, nil
			},
			activate: func(s *terminal.Session) (bool, error) { activations++; return true, nil },
			// Opening would panic: a fresh live session or inspection error forbids it.
			dispatch: func(f func()) { dispatch <- f },
		}}
		a.handleEnter()
		receiveRefresh(t, dispatch)()
		if failure && re.state.StatusMessage == "" {
			t.Fatal("inspection error lost")
		}
		if !failure && activations != 1 {
			t.Fatal("fresh session not reused")
		}
	}
}

func TestTerminalActionRetainsPendingLaunch(t *testing.T) {
	wt := git.Worktree{Path: "/repo"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	dispatch := make(chan func(), 1)
	pending := &terminal.Session{}
	opens := 0
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		find: noTerminal,
		open: func(string, string, string) (*terminal.Session, error) {
			opens++
			return pending, errors.New("still starting")
		},
		activate: func(s *terminal.Session) (bool, error) {
			if s != pending {
				t.Error("lost pending identity")
			}
			return true, errors.New("still starting")
		},
		dispatch: func(f func()) { dispatch <- f },
	}}
	for range 3 {
		a.handleEnter()
		receiveRefresh(t, dispatch)()
	}
	if opens != 1 {
		t.Fatal("pending launch duplicated")
	}
}

func TestTerminalActionKeepsSeparateModeSessions(t *testing.T) {
	wt := git.Worktree{Path: "/repo", IsMain: true}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
	dispatch := make(chan func(), 1)
	regular, box := &terminal.Session{}, &terminal.Session{}
	opens := 0
	var activated *terminal.Session
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		find: noTerminal,
		open: func(_, command, _ string) (*terminal.Session, error) {
			opens++
			if command != "" {
				return box, errors.New("old sandbox failure")
			}
			return regular, nil
		},
		activate: func(s *terminal.Session) (bool, error) { activated = s; return true, nil },
		dispatch: func(f func()) { dispatch <- f },
	}}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", SandboxName: "box"}
	a.handleEnter()
	apply := receiveRefresh(t, dispatch)
	re.state.ActiveMode = nil
	apply()
	if re.state.StatusMessage != "" {
		t.Fatal("old mode error applied")
	}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if activated != regular {
		t.Fatal("regular session lost after mode switch")
	}
	re.state.ActiveMode = &config.ModeEntry{Type: "sandbox", SandboxName: "box"}
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if activated != box || opens != 2 {
		t.Fatal("sandbox session lost after mode switch")
	}
}

func TestTerminalActionDropsRemovedTargetResult(t *testing.T) {
	for _, removeRepo := range []bool{false, true} {
		wt := git.Worktree{Path: "/repo/work"}
		re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}}}
		dispatch := make(chan func(), 1)
		a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
			find: noTerminal,
			open: func(string, string, string) (*terminal.Session, error) {
				return &terminal.Session{}, errors.New("late failure")
			},
			dispatch: func(f func()) { dispatch <- f },
		}}
		a.handleEnter()
		apply := receiveRefresh(t, dispatch)
		if removeRepo {
			a.repos = nil
		} else {
			re.state.Worktrees = nil
		}
		apply()
		if re.state.StatusMessage != "" || len(a.terminalActions) != 0 || len(a.terminalSessions) != 0 {
			t.Fatal("removed target received a late result")
		}
	}
}

func TestTerminalRecoveryRequiresExplicitForget(t *testing.T) {
	for _, action := range []string{"Forget session", "Keep waiting", "Escape"} {
		t.Run(action, func(t *testing.T) {
			app := test.NewApp()
			defer app.Quit()
			win := app.NewWindow("test")
			defer win.Close()
			target := terminalTarget{path: "/repo"}
			session := &terminal.Session{}
			a := &App{window: win, terminalSessions: map[terminalTarget]*terminal.Session{target: session}}
			a.offerTerminalRecovery(target, session)
			if action == "Escape" {
				a.handleKeyName(fyne.KeyEscape)
			} else {
				popup := win.Canvas().Overlays().Top().(*widget.PopUp)
				var button *widget.Button
				walkSetupContent(popup.Content, func(obj fyne.CanvasObject) {
					if b, ok := obj.(*widget.Button); ok && b.Text == action {
						button = b
					}
				})
				if button == nil {
					t.Fatalf("missing %s button", action)
				}
				test.Tap(button)
			}
			if a.dialogOpen || a.activeDialog != nil {
				t.Fatal("recovery dialog left keyboard blocked")
			}
			retained := a.terminalSessions[target] != nil
			if retained == (action == "Forget session") {
				t.Fatal("recovery discarded session without explicit forget")
			}
			if len(a.terminalActions) != 0 {
				t.Fatal("recovery launched terminal automatically")
			}
		})
	}
}

func TestTerminalTargetRemovedMode(t *testing.T) {
	wt := git.Worktree{Path: "/repo"}
	mode := config.ModeEntry{Type: "sandbox", SandboxName: "box", Agent: "claude"}
	re := &repoEntry{group: &RepoGroup{Modes: []config.ModeEntry{{Type: "regular"}, mode}}, state: &RepoState{Worktrees: []git.Worktree{wt}, ActiveMode: &mode}}
	a := &App{repos: []*repoEntry{re}}
	target := targetForTerminal(re, wt.Path)
	re.state.ActiveMode = &config.ModeEntry{Type: "regular"}
	if !a.hasTerminalTarget(target) {
		t.Fatal("inactive valid mode discarded")
	}
	re.group.Modes = re.group.Modes[:1]
	if a.hasTerminalTarget(target) {
		t.Fatal("removed mode retained")
	}
}

func TestTerminalDiscoveryExcludesOtherMode(t *testing.T) {
	wt := git.Worktree{Path: "/repo", IsMain: true}
	mode := &config.ModeEntry{Type: "sandbox", SandboxName: "box"}
	re := &repoEntry{state: &RepoState{Worktrees: []git.Worktree{wt}, ActiveMode: mode}}
	dispatch := make(chan func(), 1)
	box := &terminal.Session{}
	opens := 0
	a := &App{repos: []*repoEntry{re}, terminalDeps: &terminalDependencies{
		open: func(string, string, string) (*terminal.Session, error) { opens++; return box, nil },
		find: func(_ []string, _ string, claimed []*terminal.Session) (*terminal.Session, error) {
			if len(claimed) != 1 || claimed[0] != box {
				t.Error("sandbox association not excluded")
			}
			return nil, nil
		},
		dispatch: func(f func()) { dispatch <- f },
	}}
	a.handleEnter()
	apply := receiveRefresh(t, dispatch)
	re.state.ActiveMode = nil
	a.handleEnter() // Same card's first launch has not completed its UI callback.
	if opens != 1 {
		t.Fatal("mode switch bypassed pending guard")
	}
	apply()
	a.handleEnter()
	receiveRefresh(t, dispatch)()
	if opens != 2 {
		t.Fatal("regular mode did not get its own session")
	}
}
