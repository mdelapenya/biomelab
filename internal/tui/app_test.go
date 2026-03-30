package tui

import (
	"strings"
	"testing"
	"time"

	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
)

// testApp creates an App with n pre-populated repoTabs using fake data.
func testApp(n int) App {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repository"
	ti.CharLimit = 256
	ti.Width = 50

	app := App{
		textInput:       ti,
		refreshInterval: DefaultNetworkRefreshInterval,
		width:           120,
		height:          40,
	}

	for i := range n {
		name := string(rune('a' + i))
		path := "/tmp/repo-" + name
		m := testModel(2)
		m.embedded = true
		m.worktrees[0].Path = path
		app.repos = append(app.repos, &repoTab{
			path:  path,
			name:  "owner/repo-" + name,
			model: m,
		})
	}

	if n > 0 {
		app.focus = focusRight
	}

	return app
}

func TestAppFocusToggle(t *testing.T) {
	a := testApp(2)
	a.focus = focusRight

	// Tab switches to left.
	msg := tea.KeyMsg{Type: tea.KeyTab}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft", app.focus)
	}

	// Tab switches back to right.
	updated, _ = app.Update(msg)
	app = updated.(App)
	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight", app.focus)
	}
}

func TestAppFocusToggle_ShiftTab(t *testing.T) {
	a := testApp(2)
	a.focus = focusRight

	// Shift+Tab switches to left.
	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft after shift+tab", app.focus)
	}

	// Shift+Tab switches back to right.
	updated, _ = app.Update(msg)
	app = updated.(App)
	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight after second shift+tab", app.focus)
	}
}

func TestAppFocusToggle_EmptyRepos(t *testing.T) {
	a := testApp(0)

	// Tab should be a no-op when no repos are registered.
	msg := tea.KeyMsg{Type: tea.KeyTab}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft (no repos)", app.focus)
	}
}

func TestAppRepoNavigation(t *testing.T) {
	a := testApp(3)
	a.focus = focusLeft
	a.active = 0

	// Down moves to next repo.
	msg := tea.KeyMsg{Type: tea.KeyDown}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.active != 1 {
		t.Errorf("active = %d, want 1", app.active)
	}

	// Down again.
	updated, _ = app.Update(msg)
	app = updated.(App)
	if app.active != 2 {
		t.Errorf("active = %d, want 2", app.active)
	}

	// Down at last is no-op.
	updated, _ = app.Update(msg)
	app = updated.(App)
	if app.active != 2 {
		t.Errorf("active = %d, want 2 (should not go past last)", app.active)
	}

	// Up moves back.
	upMsg := tea.KeyMsg{Type: tea.KeyUp}
	updated, _ = app.Update(upMsg)
	app = updated.(App)
	if app.active != 1 {
		t.Errorf("active = %d, want 1", app.active)
	}
}

func TestAppRepoNavigation_UpAtFirst(t *testing.T) {
	a := testApp(3)
	a.focus = focusLeft
	a.active = 0

	msg := tea.KeyMsg{Type: tea.KeyUp}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.active != 0 {
		t.Errorf("active = %d, want 0 (up at first is no-op)", app.active)
	}
}

func TestAppAddRepoMode(t *testing.T) {
	a := testApp(1)
	a.focus = focusLeft

	// Press 'a' to enter add mode.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.mode != appModeAddRepo {
		t.Errorf("mode = %d, want appModeAddRepo", app.mode)
	}

	// Esc cancels.
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	updated, _ = app.Update(escMsg)
	app = updated.(App)
	if app.mode != appModeNormal {
		t.Errorf("mode = %d, want appModeNormal after Esc", app.mode)
	}
}

func TestAppAddRepoMode_EmptyEnterCancels(t *testing.T) {
	a := testApp(0)
	a.mode = appModeAddRepo

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.mode != appModeNormal {
		t.Errorf("mode = %d, want appModeNormal after empty Enter", app.mode)
	}
}

func TestAppRemoveRepo(t *testing.T) {
	a := testApp(2)
	a.focus = focusLeft
	a.active = 0

	// Press 'x' to start removal.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.mode != appModeConfirmRemove {
		t.Errorf("mode = %d, want appModeConfirmRemove", app.mode)
	}

	// Press 'y' to confirm.
	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	updated, _ = app.Update(yMsg)
	app = updated.(App)
	if len(app.repos) != 1 {
		t.Errorf("expected 1 repo after removal, got %d", len(app.repos))
	}
	if app.mode != appModeNormal {
		t.Errorf("mode = %d, want appModeNormal after removal", app.mode)
	}
}

func TestAppRemoveRepo_CancelledByN(t *testing.T) {
	a := testApp(2)
	a.focus = focusLeft
	a.mode = appModeConfirmRemove

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if len(app.repos) != 2 {
		t.Errorf("expected 2 repos (cancelled), got %d", len(app.repos))
	}
	if app.mode != appModeNormal {
		t.Errorf("mode = %d, want appModeNormal", app.mode)
	}
}

func TestAppRemoveLastRepo(t *testing.T) {
	a := testApp(1)
	a.focus = focusLeft
	a.mode = appModeConfirmRemove

	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	updated, _ := a.Update(yMsg)
	app := updated.(App)
	if len(app.repos) != 0 {
		t.Errorf("expected 0 repos after removing last, got %d", len(app.repos))
	}
	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft after removing last repo", app.focus)
	}
}

func TestAppForwardsKeysToModel(t *testing.T) {
	a := testApp(2)
	a.focus = focusRight
	a.repos[0].model.cursor = 0

	// Down key should go to child model (navigate from main to first linked worktree).
	msg := tea.KeyMsg{Type: tea.KeyDown}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.repos[0].model.cursor != 1 {
		t.Errorf("child cursor = %d, want 1 (down should move to first linked)", app.repos[0].model.cursor)
	}
}

func TestAppStaleMessageRouting(t *testing.T) {
	a := testApp(2)
	a.active = 0

	// Send a refreshMsg for the second repo.
	wts := []git.Worktree{{Path: "/new-path", Branch: "new", IsMain: true}}
	msg := refreshMsg{
		repoPath:  "/tmp/repo-b",
		worktrees: wts,
		agents:    agent.DetectionResult{},
	}
	updated, _ := a.Update(msg)
	app := updated.(App)

	// First repo should be unchanged.
	if len(app.repos[0].model.worktrees) != 2 {
		t.Errorf("repo[0] worktrees changed unexpectedly: got %d", len(app.repos[0].model.worktrees))
	}
	// Second repo should have the update (since testModel has nil repo, isStale returns false for matching path).
	// The message routes to repo[1] because repoPath matches.
	if len(app.repos[1].model.worktrees) != 1 {
		t.Errorf("repo[1] worktrees = %d, want 1", len(app.repos[1].model.worktrees))
	}
}

func TestAppEmptyStateView(t *testing.T) {
	a := testApp(0)
	a.width = 120
	a.height = 40

	view := a.View()
	if !strings.Contains(view, "No repositories registered") {
		t.Error("expected empty state message")
	}
	if !strings.Contains(view, "Press 'a'") {
		t.Error("expected 'press a' instruction")
	}
}

func TestAppLeftPanelRender(t *testing.T) {
	a := testApp(3)
	a.active = 1

	content := a.renderRepoList(30, 30)
	if !strings.Contains(content, "▸") {
		t.Error("expected ▸ marker for selected repo")
	}
	if !strings.Contains(content, "owner/repo-b") {
		t.Error("expected active repo name in list")
	}
}

func TestAppLayoutWidths(t *testing.T) {
	a := testApp(1)
	a.width = 200

	left := a.leftPanelWidth()
	right := a.dashWidth()

	if left != 30 { // 200 * 15 / 100 = 30
		t.Errorf("left width = %d, want 30", left)
	}
	if right != 170 { // 200 - 30 = 170
		t.Errorf("right width = %d, want 170", right)
	}
}

func TestAppLayoutWidths_MinLeft(t *testing.T) {
	a := testApp(1)
	a.width = 80

	left := a.leftPanelWidth()
	if left < 20 {
		t.Errorf("left width = %d, want >= 20 (minimum)", left)
	}
}

func TestAppEnterFromLeftPanel(t *testing.T) {
	a := testApp(2)
	a.focus = focusLeft

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := a.Update(msg)
	app := updated.(App)
	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight after Enter in left panel", app.focus)
	}
}

func TestAppQuitFromLeftPanel(t *testing.T) {
	a := testApp(1)
	a.focus = focusLeft

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := a.Update(msg)
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestAppQuitBlockedDuringTextInput(t *testing.T) {
	a := testApp(1)
	a.focus = focusRight
	a.repos[0].model.mode = modeCreate // text input active

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := a.Update(msg)
	// When child is not in normal mode, 'q' should be forwarded to child, not quit.
	// The child in modeCreate treats 'q' as text input, so no quit command.
	// We just verify we don't get a tea.Quit.
	if cmd != nil {
		// cmd could be textinput update, not quit.
		// Run the cmd to check it's not a quit.
		cmdMsg := cmd()
		if _, ok := cmdMsg.(tea.QuitMsg); ok {
			t.Error("'q' should not quit when child is in text input mode")
		}
	}
}

func TestNewApp(t *testing.T) {
	a := NewApp("/tmp/test-config.json", nil, 0)
	if a.refreshInterval != DefaultNetworkRefreshInterval {
		t.Errorf("refreshInterval = %v, want %v", a.refreshInterval, DefaultNetworkRefreshInterval)
	}
}

func TestNewApp_CustomInterval(t *testing.T) {
	a := NewApp("/tmp/test-config.json", nil, 10*time.Second)
	if a.refreshInterval != 10*time.Second {
		t.Errorf("refreshInterval = %v, want 10s", a.refreshInterval)
	}
}

func TestExtractRepoPath(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{"refreshMsg", refreshMsg{repoPath: "/a"}, "/a"},
		{"tickMsg", tickMsg{repoPath: "/b"}, "/b"},
		{"localTickMsg", localTickMsg{repoPath: "/c"}, "/c"},
		{"worktreeCreatedMsg", worktreeCreatedMsg{repoPath: "/d"}, "/d"},
		{"worktreeRemovedMsg", worktreeRemovedMsg{repoPath: "/e"}, "/e"},
		{"prFetchedMsg", prFetchedMsg{repoPath: "/f"}, "/f"},
		{"pullMsg", pullMsg{repoPath: "/g"}, "/g"},
		{"worktreeRepairedMsg", worktreeRepairedMsg{repoPath: "/h"}, "/h"},
		{"localFlashDoneMsg", localFlashDoneMsg{repoPath: "/i"}, "/i"},
		{"netFlashDoneMsg", netFlashDoneMsg{repoPath: "/j"}, "/j"},
		{"cliCheckMsg", cliCheckMsg{repoPath: "/k"}, "/k"},
		{"windowSizeMsg", tea.WindowSizeMsg{}, ""},
		{"keyMsg", tea.KeyMsg{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoPath(tt.msg)
			if got != tt.want {
				t.Errorf("extractRepoPath(%T) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestAppAddRepoMsg_Error(t *testing.T) {
	a := testApp(0)

	msg := addRepoMsg{err: fmt.Errorf("not a git repo")}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if len(app.repos) != 0 {
		t.Errorf("expected 0 repos after error, got %d", len(app.repos))
	}
	if !strings.Contains(app.statusMsg, "not a git repo") {
		t.Errorf("statusMsg = %q, want error message", app.statusMsg)
	}
}

func TestAppAddRepoMsg_Success(t *testing.T) {
	a := testApp(0)

	msg := addRepoMsg{path: "/tmp/new-repo", name: "owner/new-repo"}
	updated, _ := a.Update(msg)
	app := updated.(App)

	// Repo won't actually be added because msg.repo is nil and New(nil, ...) creates a model.
	// But the repoTab will be created.
	if len(app.repos) != 1 {
		t.Errorf("expected 1 repo after add, got %d", len(app.repos))
	}
	if app.repos[0].name != "owner/new-repo" {
		t.Errorf("repo name = %q, want owner/new-repo", app.repos[0].name)
	}
	if app.active != 0 {
		t.Errorf("active = %d, want 0", app.active)
	}
	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight", app.focus)
	}
}

func TestAppRemoveRepo_AdjustsActive(t *testing.T) {
	a := testApp(3)
	a.focus = focusLeft
	a.active = 2 // last repo
	a.mode = appModeConfirmRemove

	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	updated, _ := a.Update(yMsg)
	app := updated.(App)

	if len(app.repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(app.repos))
	}
	if app.active != 1 {
		t.Errorf("active = %d, want 1 (should adjust when last repo removed)", app.active)
	}
}

func TestAppView_WithRepos(t *testing.T) {
	a := testApp(2)
	a.width = 120
	a.height = 40

	view := a.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should contain repo list panel content.
	if !strings.Contains(view, "Repos") {
		t.Error("expected 'Repos' header in view")
	}
}

func TestAppInitMsg(t *testing.T) {
	a := testApp(0)
	// Simulate having already received a WindowSizeMsg.
	a.width = 120
	a.height = 40

	// Simulate what Init() produces.
	repos := []*repoTab{
		{path: "/tmp/repo-a", name: "owner/repo-a", model: testModel(2)},
		{path: "/tmp/repo-b", name: "owner/repo-b", model: testModel(3)},
	}
	msg := appInitMsg{repos: repos, cmds: nil}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if len(app.repos) != 2 {
		t.Errorf("expected 2 repos after init, got %d", len(app.repos))
	}
	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight after init with repos", app.focus)
	}
}

func TestAppInitMsg_ResizesChildren(t *testing.T) {
	a := testApp(0)
	a.width = 120
	a.height = 40

	repos := []*repoTab{
		{path: "/tmp/repo-a", name: "owner/repo-a", model: testModel(2)},
	}
	msg := appInitMsg{repos: repos, cmds: nil}
	updated, _ := a.Update(msg)
	app := updated.(App)

	// After appInitMsg with dimensions available, the child should have been resized
	// (ready == true means it received a WindowSizeMsg).
	if !app.repos[0].model.ready {
		t.Error("child model should be ready after appInitMsg resizes it")
	}
}

func TestAppInitMsg_Empty(t *testing.T) {
	a := testApp(0)

	msg := appInitMsg{repos: nil, cmds: nil}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if len(app.repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(app.repos))
	}
	// Focus should stay on left panel when no repos.
	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft with no repos", app.focus)
	}
}

func TestAppQuitBoundsCheck(t *testing.T) {
	// Regression test: quit handler must not panic when active >= len(repos).
	a := testApp(1)
	a.focus = focusRight
	a.active = 5 // deliberately out of bounds

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	// This should not panic.
	_, _ = a.Update(msg)
}

func TestAppAddRepoFromEmptyState(t *testing.T) {
	a := testApp(0) // no repos
	// Focus should be left by default.

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if app.mode != appModeAddRepo {
		t.Errorf("mode = %d, want appModeAddRepo", app.mode)
	}
}

func TestAppMouseClickLeftPanel(t *testing.T) {
	a := testApp(3)
	a.focus = focusRight

	// Repo rows start at Y = headerLines(1) + panel border(1) + section header(1) = 3.
	// First repo = Y=3, second = Y=4, etc.
	msg := tea.MouseMsg{X: 5, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if app.focus != focusLeft {
		t.Errorf("focus = %d, want focusLeft after clicking left panel", app.focus)
	}
	if app.active != 0 {
		t.Errorf("active = %d, want 0 (clicked first repo row)", app.active)
	}
}

func TestAppMouseClickLeftPanel_SelectsRepo(t *testing.T) {
	a := testApp(3)
	a.focus = focusRight
	a.active = 0

	// Second repo row = Y=4.
	msg := tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if app.active != 1 {
		t.Errorf("active = %d, want 1 (clicked second repo row)", app.active)
	}
}

func TestAppMouseClickRightPanel(t *testing.T) {
	a := testApp(2)
	a.focus = focusLeft

	// Click in the right panel area (X >= leftPanelWidth).
	leftWidth := a.leftPanelWidth()
	msg := tea.MouseMsg{X: leftWidth + 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	updated, _ := a.Update(msg)
	app := updated.(App)

	if app.focus != focusRight {
		t.Errorf("focus = %d, want focusRight after clicking right panel", app.focus)
	}
}

func TestAppViewFitsTerminalHeight(t *testing.T) {
	a := testApp(2)
	a.width = 120
	a.height = 40

	// Simulate WindowSizeMsg so children are ready.
	updated, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(App)

	view := app.View()
	lines := strings.Split(view, "\n")

	// Hard-clamp guarantees no overflow.
	if len(lines) > app.height+1 { // +1 for possible trailing empty from Split
		t.Errorf("View has %d split lines, but terminal height is %d",
			len(lines), app.height)
	}

	// Title must be present on first line.
	if !strings.Contains(lines[0], "gwaim") {
		t.Errorf("expected title on line 0, got %q", lines[0])
	}
	// Timestamps are now inside the right panel (line 1 is the top border).
	if len(lines) > 2 && !strings.Contains(lines[2], "Last Update") {
		t.Errorf("expected timestamps inside right panel (line 2), got %q", lines[2])
	}
}

func TestAppSwitchRepoPausesOldResumesNew(t *testing.T) {
	a := testApp(3)
	a.focus = focusLeft
	a.active = 0

	// Switch from repo 0 to repo 1.
	msg := tea.KeyMsg{Type: tea.KeyDown}
	updated, cmd := a.Update(msg)
	app := updated.(App)

	if app.active != 1 {
		t.Fatalf("active = %d, want 1", app.active)
	}

	// Old repo should be paused.
	if !app.repos[0].model.paused {
		t.Error("repo 0 should be paused after switching away")
	}
	// New repo should be unpaused.
	if app.repos[1].model.paused {
		t.Error("repo 1 should NOT be paused after switching to it")
	}
	// Resume returns commands (tick chains).
	if cmd == nil {
		t.Error("expected commands from resume")
	}
}

func TestAppPausedTickDoesNotReschedule(t *testing.T) {
	m := testModel(2)
	m.paused = true

	// A tick message to a paused model should return no commands.
	updated, cmd := m.Update(tickMsg{})
	model := updated.(Model)
	_ = model
	if cmd != nil {
		t.Error("paused model should not return commands from tickMsg")
	}

	// Same for localTickMsg.
	_, cmd = m.Update(localTickMsg{})
	if cmd != nil {
		t.Error("paused model should not return commands from localTickMsg")
	}
}

func TestAppRemoveActiveRepoResumesNewActive(t *testing.T) {
	a := testApp(3)
	a.focus = focusLeft
	a.active = 1

	// Pause repos 0 and 2 (not active).
	a.repos[0].model = a.repos[0].model.Pause()
	a.repos[2].model = a.repos[2].model.Pause()

	// Remove active repo (index 1). New active becomes index 1 (was repo 2).
	a.mode = appModeConfirmRemove
	yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	updated, cmd := a.Update(yMsg)
	app := updated.(App)

	if len(app.repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(app.repos))
	}
	if app.active != 1 {
		t.Fatalf("active = %d, want 1", app.active)
	}

	// The new active repo (was paused) should now be resumed.
	if app.repos[app.active].model.paused {
		t.Error("new active repo should be unpaused after removal of previous active")
	}
	// Resume should have returned commands.
	if cmd == nil {
		t.Error("expected commands from resuming new active repo")
	}
}

func TestAppXKeyNoRepos(t *testing.T) {
	a := testApp(0)
	a.focus = focusLeft

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	updated, _ := a.Update(msg)
	app := updated.(App)

	// Should stay in normal mode — nothing to remove.
	if app.mode != appModeNormal {
		t.Errorf("mode = %d, want appModeNormal", app.mode)
	}
}

func TestBuildPanels_ScrollbarRendered(t *testing.T) {
	a := testApp(1)
	a.focus = focusRight

	// No scrollbar when content fits (scrollTotal <= scrollVisible).
	out := a.buildPanels("left", "right", 10, 20, 5, 10, 10, 0)
	lines := strings.Split(out, "\n")
	// Content rows should use normal right border (no thumb character).
	for i := 1; i <= 5; i++ {
		if strings.Contains(lines[i], "┃") {
			t.Errorf("line %d has scrollbar thumb when content fits: %q", i, lines[i])
		}
	}

	// With scrollbar: total > visible.
	out = a.buildPanels("left", "right", 10, 20, 10, 40, 10, 0)
	lines = strings.Split(out, "\n")
	thumbCount := 0
	for i := 1; i <= 10; i++ {
		if strings.Contains(lines[i], "┃") {
			thumbCount++
		}
	}
	if thumbCount == 0 {
		t.Error("expected scrollbar thumb in output, got none")
	}
	if thumbCount >= 10 {
		t.Errorf("thumb should be smaller than track, got %d/%d", thumbCount, 10)
	}
}
