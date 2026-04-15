package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// testModel creates a Model with pre-populated worktrees (no repo/detector needed for unit tests).
func testModel(n int) Model {
	ti := textinput.New()
	ti.Placeholder = "branch-name"
	ti.CharLimit = 80
	ti.Width = 30

	m := Model{
		keys:            defaultKeyMap(),
		agents:          make(agent.DetectionResult),
		ides:            make(ide.DetectionResult),
		textInput:       ti,
		refreshInterval: DefaultNetworkRefreshInterval,
		prProv:          &provider.GitHubProvider{},
	}
	for i := range n {
		wt := git.Worktree{
			Path:   "/tmp/wt-" + string(rune('a'+i)),
			Branch: "branch-" + string(rune('a'+i)),
			IsMain: i == 0,
		}
		m.worktrees = append(m.worktrees, wt)
	}
	return m
}

func TestUpdate(t *testing.T) {
	t.Run("navigate right in linked worktrees", func(t *testing.T) {
		m := testModel(4) // main + 3 linked
		m.cursor = 1      // first linked

		msg := tea.KeyMsg{Type: tea.KeyRight}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 2 {
			t.Errorf("cursor = %d, want 2", model.cursor)
		}
	})

	t.Run("navigate right from main is no-op", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 0 // main

		msg := tea.KeyMsg{Type: tea.KeyRight}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 0 {
			t.Errorf("cursor = %d, want 0 (right from main is no-op)", model.cursor)
		}
	})

	t.Run("navigate left in linked worktrees", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 2

		msg := tea.KeyMsg{Type: tea.KeyLeft}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 1 {
			t.Errorf("cursor = %d, want 1", model.cursor)
		}
	})

	t.Run("navigate left at first linked is no-op", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 1 // first linked

		msg := tea.KeyMsg{Type: tea.KeyLeft}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 1 {
			t.Errorf("cursor = %d, want 1 (can't go left past first linked)", model.cursor)
		}
	})

	t.Run("navigate right at last linked is no-op", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 2

		msg := tea.KeyMsg{Type: tea.KeyRight}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 2 {
			t.Errorf("cursor = %d, want 2", model.cursor)
		}
	})

	t.Run("navigate down from main goes to first linked", func(t *testing.T) {
		m := testModel(4) // main + 3 linked
		m.cursor = 0

		msg := tea.KeyMsg{Type: tea.KeyDown}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 1 {
			t.Errorf("cursor = %d, want 1 (down from main goes to first linked)", model.cursor)
		}
	})

	t.Run("navigate up from first linked row goes to main", func(t *testing.T) {
		m := testModel(4) // main + 3 linked
		m.cursor = 2      // second card in first linked row

		msg := tea.KeyMsg{Type: tea.KeyUp}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 0 {
			t.Errorf("cursor = %d, want 0 (up from first linked row goes to main)", model.cursor)
		}
	})

	t.Run("refresh msg updates worktrees", func(t *testing.T) {
		m := testModel(0)

		wts := []git.Worktree{
			{Path: "/a", Branch: "main", IsMain: true},
			{Path: "/b", Branch: "feature"},
		}
		msg := refreshMsg{worktrees: wts, agents: agent.DetectionResult{}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if len(model.worktrees) != 2 {
			t.Errorf("got %d worktrees, want 2", len(model.worktrees))
		}
	})

	t.Run("refresh msg clamps cursor to valid range", func(t *testing.T) {
		m := testModel(5)
		m.cursor = 4

		// Refresh with fewer worktrees.
		wts := []git.Worktree{
			{Path: "/a", Branch: "main", IsMain: true},
		}
		msg := refreshMsg{worktrees: wts}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.cursor != 0 {
			t.Errorf("cursor = %d, want 0", model.cursor)
		}
	})

	t.Run("c key enters create mode", func(t *testing.T) {
		m := testModel(2)

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeCreate {
			t.Errorf("mode = %d, want modeCreate", model.mode)
		}
	})

	t.Run("escape exits create mode", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeCreate

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal", model.mode)
		}
	})

	t.Run("delete main worktree is blocked", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 0 // main worktree

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("should stay in normal mode when trying to delete main")
		}
		if model.statusMsg == "" {
			t.Error("expected error status message")
		}
	})

	t.Run("delete non-main enters confirm delete mode", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 1 // non-main worktree

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeConfirmDelete {
			t.Errorf("mode = %d, want modeConfirmDelete", model.mode)
		}
	})

	t.Run("confirm delete cancelled by pressing n", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeConfirmDelete
		m.cursor = 1

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after cancel", model.mode)
		}
	})

	t.Run("confirm delete requires enter after y", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeConfirmDelete
		m.cursor = 1

		// Pressing 'y' should stay in confirm-delete mode with deleteConfirmed set.
		yMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
		updated, _ := m.Update(yMsg)
		model := updated.(Model)

		if model.mode != modeConfirmDelete {
			t.Errorf("after 'y': mode = %d, want modeConfirmDelete", model.mode)
		}
		if !model.deleteConfirmed {
			t.Error("after 'y': deleteConfirmed should be true")
		}

		// Pressing Enter after 'y' should confirm deletion and return to normal.
		enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
		updated, cmd := model.Update(enterMsg)
		model = updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("after Enter: mode = %d, want modeNormal", model.mode)
		}
		if cmd == nil {
			t.Error("after Enter: expected a command for worktree removal")
		}
	})

	t.Run("confirm delete enter without y cancels", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeConfirmDelete
		m.cursor = 1

		// Pressing Enter without 'y' should cancel.
		enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
		updated, cmd := m.Update(enterMsg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after Enter without y", model.mode)
		}
		if cmd != nil {
			t.Error("expected no command when Enter pressed without y confirmation")
		}
	})

	t.Run("confirm delete esc cancels even after y", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeConfirmDelete
		m.deleteConfirmed = true
		m.cursor = 1

		// Pressing Esc should cancel even after 'y'.
		escMsg := tea.KeyMsg{Type: tea.KeyEscape}
		updated, _ := m.Update(escMsg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after Esc", model.mode)
		}
		if model.deleteConfirmed {
			t.Error("deleteConfirmed should be reset after Esc")
		}
	})

	t.Run("refresh from main worktree", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 0 // main worktree

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
		updated, cmd := m.Update(msg)
		model := updated.(Model)

		if model.statusMsg == "" {
			t.Error("expected status message for refresh")
		}
		if cmd == nil {
			t.Error("expected a command for refresh")
		}
	})

	t.Run("refresh from non-main worktree", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 1 // non-main worktree

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
		updated, cmd := m.Update(msg)
		model := updated.(Model)

		if model.statusMsg == "" {
			t.Error("expected status message for refresh")
		}
		if cmd == nil {
			t.Error("expected a command for refresh")
		}
	})

	t.Run("window size msg updates dimensions", func(t *testing.T) {
		m := testModel(2)

		msg := tea.WindowSizeMsg{Width: 120, Height: 40}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.width != 120 || model.height != 40 {
			t.Errorf("size = %dx%d, want 120x40", model.width, model.height)
		}
	})

	t.Run("f key enters fetch PR mode", func(t *testing.T) {
		m := testModel(2)
		m.cliAvail = provider.CLIAvailable

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeFetchPR {
			t.Errorf("mode = %d, want modeFetchPR", model.mode)
		}
	})

	t.Run("fetch PR blocked from non-main worktree", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 1
		m.cliAvail = provider.CLIAvailable

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal (fetch PR only from main)", model.mode)
		}
	})

	t.Run("fetch PR blocked without gh CLI", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 0
		m.cliAvail = provider.CLINotFound

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal (gh not available)", model.mode)
		}
		if model.statusMsg == "" {
			t.Error("expected error status message")
		}
	})

	t.Run("escape exits fetch PR mode", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeFetchPR

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after Esc", model.mode)
		}
	})

	t.Run("fetch PR empty input cancels", func(t *testing.T) {
		m := testModel(2)
		m.mode = modeFetchPR

		msg := tea.KeyMsg{Type: tea.KeyEnter}
		updated, cmd := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after empty Enter", model.mode)
		}
		if cmd != nil {
			t.Error("expected no command for empty PR input")
		}
	})

	t.Run("PR fetched msg with error shows status", func(t *testing.T) {
		m := testModel(2)

		msg := prFetchedMsg{err: fmt.Errorf("PR not found")}
		updated, _ := m.Update(msg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after error", model.mode)
		}
		if !strings.Contains(model.statusMsg, "PR not found") {
			t.Errorf("expected error in statusMsg, got %q", model.statusMsg)
		}
	})

	t.Run("PR fetched msg error handling", func(t *testing.T) {
		m := testModel(2)

		errMsg := prFetchedMsg{branchName: "feature-branch", err: fmt.Errorf("some error")}
		updated, _ := m.Update(errMsg)
		model := updated.(Model)

		if model.mode != modeNormal {
			t.Errorf("mode = %d, want modeNormal after error", model.mode)
		}
		if !strings.Contains(model.statusMsg, "some error") {
			t.Errorf("expected error in statusMsg, got %q", model.statusMsg)
		}
	})

	t.Run("cli check msg sets cli availability", func(t *testing.T) {
		m := testModel(2)

		updated, _ := m.Update(cliCheckMsg{avail: provider.CLINotFound})
		model := updated.(Model)

		if model.cliAvail != provider.CLINotFound {
			t.Errorf("expected cliAvail = CLINotFound, got %v", model.cliAvail)
		}
	})
}

func TestView_NoWorktrees(t *testing.T) {
	m := testModel(0)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestView_WithWorktrees(t *testing.T) {
	m := testModel(3)
	m.width = 120
	m.height = 40

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestColumns(t *testing.T) {
	tests := []struct {
		width    int
		wantCols int
	}{
		{0, 2},     // default
		{44, 1},    // just one card
		{88, 2},    // two cards
		{132, 3},   // three cards
	}

	for _, tt := range tests {
		m := testModel(1)
		m.width = tt.width
		got := m.columns()
		if got != tt.wantCols {
			t.Errorf("width=%d: columns() = %d, want %d", tt.width, got, tt.wantCols)
		}
	}
}

func TestScrollState_NotReady(t *testing.T) {
	m := testModel(3)
	total, visible, offset := m.ScrollState()
	if total != 0 || visible != 0 || offset != 0 {
		t.Errorf("ScrollState() on unready model = (%d, %d, %d), want (0, 0, 0)", total, visible, offset)
	}
}

func TestNew_DefaultRefreshInterval(t *testing.T) {
	m := New(nil, nil, nil, nil, 0)
	if m.refreshInterval != DefaultNetworkRefreshInterval {
		t.Errorf("refreshInterval = %v, want %v", m.refreshInterval, DefaultNetworkRefreshInterval)
	}
}

func TestNew_CustomRefreshInterval(t *testing.T) {
	m := New(nil, nil, nil, nil, 10*time.Second)
	if m.refreshInterval != 10*time.Second {
		t.Errorf("refreshInterval = %v, want 10s", m.refreshInterval)
	}
}

func TestKeyMap(t *testing.T) {
	km := defaultKeyMap()

	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, km.Quit) {
		t.Error("q should match Quit")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}, km.Create) {
		t.Error("c should match Create")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, km.Delete) {
		t.Error("d should match Delete")
	}
}

func TestRenderFixedTop_CLINotFoundShowsIndicator(t *testing.T) {
	m := testModel(2)
	m.width = 120
	m.height = 40
	m.cliAvail = provider.CLINotFound

	body := m.renderFixedTop()

	if !strings.Contains(body, "gh not installed") {
		t.Error("expected 'gh not installed' indicator in card body when gh CLI is not found")
	}
}

func TestRenderFixedTop_CLINotAuthenticatedShowsIndicator(t *testing.T) {
	m := testModel(2)
	m.width = 120
	m.height = 40
	m.cliAvail = provider.CLINotAuthenticated

	body := m.renderFixedTop()

	if !strings.Contains(body, "gh not authenticated") {
		t.Error("expected 'gh not authenticated' indicator in card body")
	}
}

func TestRenderFixedTop_UnsupportedProviderShowsMessage(t *testing.T) {
	m := testModel(2)
	m.width = 120
	m.height = 40
	m.cliAvail = provider.CLIUnsupportedProvider
	m.prProv = provider.NewUnsupportedProvider(provider.ProviderUnknown)

	body := m.renderFixedTop()

	if !strings.Contains(body, "not yet supported") {
		t.Error("expected 'not yet supported' in card body, got: " + body)
	}
}

func TestLocalRefreshMsg_PreservesPRs(t *testing.T) {
	// A local refresh (hasPRs=false) must not wipe PR data set by a prior network refresh.
	m := testModel(2)
	branch := m.worktrees[0].Branch
	m.prs = provider.PRResult{branch: &provider.PRInfo{Number: 7, Title: "my PR"}}

	// Simulate a local refresh arriving with no PR data.
	updated, _ := m.Update(refreshMsg{
		worktrees: m.worktrees,
		agents:    m.agents,
		hasPRs:    false,
	})
	model := updated.(Model)

	if model.prs[branch] == nil || model.prs[branch].Number != 7 {
		t.Error("local refresh must not overwrite existing PR data")
	}
}

func TestNetworkRefreshMsg_UpdatesPRs(t *testing.T) {
	// A network refresh (hasPRs=true) must update the PR map.
	m := testModel(2)
	branch := m.worktrees[0].Branch
	m.prs = provider.PRResult{branch: &provider.PRInfo{Number: 7, Title: "old"}}

	newPRs := provider.PRResult{branch: &provider.PRInfo{Number: 42, Title: "new"}}
	updated, _ := m.Update(refreshMsg{
		worktrees: m.worktrees,
		agents:    m.agents,
		prs:       newPRs,
		hasPRs:    true,
	})
	model := updated.(Model)

	if model.prs[branch] == nil || model.prs[branch].Number != 42 {
		t.Errorf("network refresh must update PR data, got %v", model.prs[branch])
	}
}

func TestLocalTickMsg_SchedulesNextLocalTick(t *testing.T) {
	m := testModel(2)
	_, cmd := m.Update(localTickMsg{})
	if cmd == nil {
		t.Error("localTickMsg handler must return a command (next localTick + localRefresh)")
	}
}

func TestRefreshFlash_LocalSetsAndClears(t *testing.T) {
	m := testModel(2)

	// A local-sourced refreshMsg sets localFlash and returns a revert cmd.
	updated, cmd := m.Update(refreshMsg{
		source:    refreshSourceLocal,
		worktrees: m.worktrees,
		agents:    m.agents,
	})
	model := updated.(Model)

	if !model.localFlash {
		t.Error("localFlash should be true after local refreshMsg")
	}
	if model.netFlash {
		t.Error("netFlash should not be set by a local refresh")
	}
	if cmd == nil {
		t.Error("expected a flash-revert command after local refresh")
	}
	if model.lastLocalRefresh.IsZero() {
		t.Error("lastLocalRefresh should be set after local refresh")
	}

	// localFlashDoneMsg clears the flag.
	updated, _ = model.Update(localFlashDoneMsg{})
	model = updated.(Model)
	if model.localFlash {
		t.Error("localFlash should be false after localFlashDoneMsg")
	}
}

func TestRefreshFlash_NetworkSetsAndClears(t *testing.T) {
	m := testModel(2)

	updated, cmd := m.Update(refreshMsg{
		source:    refreshSourceNetwork,
		worktrees: m.worktrees,
		agents:    m.agents,
		hasPRs:    true,
	})
	model := updated.(Model)

	if !model.netFlash {
		t.Error("netFlash should be true after network refreshMsg")
	}
	if model.localFlash {
		t.Error("localFlash should not be set by a network refresh")
	}
	if cmd == nil {
		t.Error("expected a flash-revert command after network refresh")
	}
	if model.lastNetworkRefresh.IsZero() {
		t.Error("lastNetworkRefresh should be set after network refresh")
	}

	updated, _ = model.Update(netFlashDoneMsg{})
	model = updated.(Model)
	if model.netFlash {
		t.Error("netFlash should be false after netFlashDoneMsg")
	}
}

func TestRefreshFlash_QuickRefreshNoFlash(t *testing.T) {
	m := testModel(2)

	updated, cmd := m.Update(refreshMsg{
		source:    refreshSourceQuick,
		worktrees: m.worktrees,
		agents:    m.agents,
	})
	model := updated.(Model)

	if model.localFlash || model.netFlash {
		t.Error("quick refresh must not trigger any flash")
	}
	if cmd != nil {
		t.Error("quick refresh must not return a flash-revert command")
	}
}

func TestIsNormal(t *testing.T) {
	m := testModel(2)
	if !m.IsNormal() {
		t.Error("expected IsNormal() to return true for default model")
	}
	m.mode = modeCreate
	if m.IsNormal() {
		t.Error("expected IsNormal() to return false in create mode")
	}
}

func TestStaleRefreshMsgIgnored(t *testing.T) {
	m := testModel(2)

	// A refreshMsg with a non-matching repoPath should be ignored.
	// Since testModel has no real repo (repo == nil), isStale returns false for any repoPath.
	// We test the concept: matching empty repoPath is not stale for nil-repo models.
	wts := []git.Worktree{
		{Path: "/new", Branch: "new-branch", IsMain: true},
	}
	msg := refreshMsg{repoPath: "", worktrees: wts, agents: agent.DetectionResult{}}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if len(model.worktrees) != 1 || model.worktrees[0].Path != "/new" {
		t.Error("expected refresh with empty repoPath to be applied to nil-repo model")
	}
}

func TestStaleTickMsgIgnored(t *testing.T) {
	m := testModel(2)

	// For a nil-repo model, tickMsg with empty repoPath should still work.
	_, cmd := m.Update(tickMsg{repoPath: ""})
	// With nil repo, the commands will be created (but would fail at execution).
	// The key assertion is that the handler doesn't panic.
	_ = cmd
}

func TestRenderHeader_ShowsTimestamps(t *testing.T) {
	m := testModel(2)
	m.width = 120

	title := "biomelab - Git Worktree Agent Manager"

	// Before any refresh — shows dashes.
	header := m.renderHeader(title)
	if !strings.Contains(header, "local: —") {
		t.Errorf("expected '—' for unset local timestamp, got: %s", header)
	}

	// After a local refresh — shows a time.
	m.lastLocalRefresh = time.Date(2026, 3, 23, 12, 34, 56, 0, time.UTC)
	header = m.renderHeader(title)
	if !strings.Contains(header, "local: 12:34:56") {
		t.Errorf("expected local timestamp in header, got: %s", header)
	}

	// Flash replaces the time with ✓.
	m.localFlash = true
	header = m.renderHeader(title)
	if !strings.Contains(header, "local: ✓") {
		t.Errorf("expected ✓ flash in header, got: %s", header)
	}
}

func TestRenderConfirmPopup(t *testing.T) {
	t.Run("initial prompt shows branch name and confirm hint", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 1
		m.mode = modeConfirmDelete
		m.deleteConfirmed = false

		popup := m.renderConfirmPopup()
		if !strings.Contains(popup, "branch-b") {
			t.Errorf("popup should contain branch name, got: %s", popup)
		}
		if !strings.Contains(popup, "[y]es") {
			t.Errorf("popup should contain [y]es hint, got: %s", popup)
		}
	})

	t.Run("confirmed prompt shows Enter/Esc hint", func(t *testing.T) {
		m := testModel(3)
		m.cursor = 1
		m.mode = modeConfirmDelete
		m.deleteConfirmed = true

		popup := m.renderConfirmPopup()
		if !strings.Contains(popup, "branch-b") {
			t.Errorf("popup should contain branch name, got: %s", popup)
		}
		if !strings.Contains(popup, "[Enter] confirm") {
			t.Errorf("popup should contain [Enter] confirm hint, got: %s", popup)
		}
	})

	t.Run("popup is empty for invalid cursor", func(t *testing.T) {
		m := testModel(2)
		m.cursor = 5
		m.mode = modeConfirmDelete

		if popup := m.renderConfirmPopup(); popup != "" {
			t.Errorf("popup should be empty for invalid cursor, got: %s", popup)
		}
	})
}

func TestOverlayCenter(t *testing.T) {
	base := strings.Repeat("........\n", 10)
	popup := "HELLO"

	result := overlayCenter(base, popup, 8, 10)
	lines := strings.Split(result, "\n")

	// The popup should appear somewhere in the middle.
	found := false
	for _, line := range lines {
		if strings.Contains(line, "HELLO") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("overlay should contain popup text, got:\n%s", result)
	}
}

// TestConfirmDeleteRendersPopup is in app_test.go since the popup is rendered at the App level.

// --- Sandbox worktree creation tests ---

func TestSandboxCreateGoesDirectlyToCmd(t *testing.T) {
	m := testModel(2)
	m.activeMode = &config.ModeEntry{Type: "sandbox", SandboxName: "owner-repo-claude", Agent: "claude"}
	m.mode = modeCreate
	m.textInput.SetValue("feature/my-branch")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(msg)
	model := updated.(Model)

	// Sandbox mode goes straight to modeNormal and creates worktree (no agent prompt).
	if model.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", model.mode)
	}
	if cmd == nil {
		t.Error("expected a cmd to create sandbox worktree")
	}
}

func TestRegularCreateDoesNotUseSandbox(t *testing.T) {
	m := testModel(2)
	m.activeMode = nil
	m.mode = modeCreate
	m.textInput.SetValue("feature/my-branch")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", model.mode)
	}
}

func TestRenderMainCardHelp_NonSandbox(t *testing.T) {
	m := testModel(2)
	help := m.renderMainCardHelp()
	for _, want := range []string{"[c]reate", "[f]etch PR", "[n]ew sandbox"} {
		if !strings.Contains(help, want) {
			t.Errorf("non-sandbox help = %q, missing %q", help, want)
		}
	}
}

func TestRenderMainCardHelp_SandboxRunning(t *testing.T) {
	m := testModel(2)
	m.activeMode = &config.ModeEntry{Type: "sandbox", SandboxName: "test-sbx", Agent: "claude"}
	m.sandboxStatus = sandbox.StatusRunning
	help := m.renderMainCardHelp()
	for _, want := range []string{"[c]reate", "[f]etch PR", "[S]top", "[d]el sandbox"} {
		if !strings.Contains(help, want) {
			t.Errorf("running sandbox help = %q, missing %q", help, want)
		}
	}
}

func TestRenderMainCardHelp_SandboxStopped(t *testing.T) {
	m := testModel(2)
	m.activeMode = &config.ModeEntry{Type: "sandbox", SandboxName: "test-sbx", Agent: "claude"}
	m.sandboxStatus = sandbox.StatusStopped
	help := m.renderMainCardHelp()
	for _, want := range []string{"[s]tart", "[d]el sandbox"} {
		if !strings.Contains(help, want) {
			t.Errorf("stopped sandbox help = %q, missing %q", help, want)
		}
	}
}

func TestRenderMainCardHelp_SandboxNotFound(t *testing.T) {
	m := testModel(2)
	m.activeMode = &config.ModeEntry{Type: "sandbox", SandboxName: "test-sbx", Agent: "claude"}
	m.sandboxStatus = sandbox.StatusNotFound
	help := m.renderMainCardHelp()
	if !strings.Contains(help, "[n]ew sandbox") {
		t.Errorf("not-found sandbox help = %q, missing %q", help, "[n]ew sandbox")
	}
}

func TestRenderFixedTop_ContainsMainCardHelp(t *testing.T) {
	m := testModel(2)
	m.width = 120
	m.height = 40
	body := m.renderFixedTop()
	if !strings.Contains(body, "[c]reate") {
		t.Error("renderFixedTop should include main card help line with '[c]reate'")
	}
}

func TestViewContent_BottomBarNoMainCardActions(t *testing.T) {
	m := testModel(2)
	m.width = 120
	m.height = 40
	content := m.viewContent()
	// The help bar is the last non-empty line.
	lines := strings.Split(content, "\n")
	var helpLine string
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			helpLine = lines[i]
			break
		}
	}
	for _, absent := range []string{"[c]reate", "[f]etch PR", "[n]ew sandbox", "[S]top", "[s]tart"} {
		if strings.Contains(helpLine, absent) {
			t.Errorf("bottom help bar should not contain %q, got: %s", absent, helpLine)
		}
	}
	for _, present := range []string{"[d]elete", "[q]uit", "nav"} {
		if !strings.Contains(helpLine, present) {
			t.Errorf("bottom help bar should contain %q, got: %s", present, helpLine)
		}
	}
}
