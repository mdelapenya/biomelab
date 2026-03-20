package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
)

// testModel creates a Model with pre-populated worktrees (no repo/detector needed for unit tests).
func testModel(n int) Model {
	ti := textinput.New()
	ti.Placeholder = "branch-name"
	ti.CharLimit = 80
	ti.Width = 30

	m := Model{
		keys:      defaultKeyMap(),
		agents:    make(agent.DetectionResult),
		textInput: ti,
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

func TestUpdate_NavigateRightInLinked(t *testing.T) {
	m := testModel(4) // main + 3 linked
	m.cursor = 1      // first linked

	msg := tea.KeyMsg{Type: tea.KeyRight}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 2 {
		t.Errorf("cursor = %d, want 2", model.cursor)
	}
}

func TestUpdate_NavigateRightFromMainNoOp(t *testing.T) {
	m := testModel(3)
	m.cursor = 0 // main

	msg := tea.KeyMsg{Type: tea.KeyRight}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (right from main is no-op)", model.cursor)
	}
}

func TestUpdate_NavigateLeftInLinked(t *testing.T) {
	m := testModel(3)
	m.cursor = 2

	msg := tea.KeyMsg{Type: tea.KeyLeft}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 1 {
		t.Errorf("cursor = %d, want 1", model.cursor)
	}
}

func TestUpdate_NavigateLeftAtFirstLinkedNoOp(t *testing.T) {
	m := testModel(3)
	m.cursor = 1 // first linked

	msg := tea.KeyMsg{Type: tea.KeyLeft}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (can't go left past first linked)", model.cursor)
	}
}

func TestUpdate_NavigateRightAtEnd(t *testing.T) {
	m := testModel(3)
	m.cursor = 2

	msg := tea.KeyMsg{Type: tea.KeyRight}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 2 {
		t.Errorf("cursor = %d, want 2", model.cursor)
	}
}

func TestUpdate_NavigateDownFromMain(t *testing.T) {
	m := testModel(4) // main + 3 linked
	m.cursor = 0

	msg := tea.KeyMsg{Type: tea.KeyDown}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (down from main goes to first linked)", model.cursor)
	}
}

func TestUpdate_NavigateUpFromFirstLinkedRow(t *testing.T) {
	m := testModel(4) // main + 3 linked
	m.cursor = 2      // second card in first linked row

	msg := tea.KeyMsg{Type: tea.KeyUp}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (up from first linked row goes to main)", model.cursor)
	}
}

func TestUpdate_RefreshMsg(t *testing.T) {
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
}

func TestUpdate_RefreshMsgClampsCursor(t *testing.T) {
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
}

func TestUpdate_EnterCreateMode(t *testing.T) {
	m := testModel(2)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.mode != modeCreate {
		t.Errorf("mode = %d, want modeCreate", model.mode)
	}
}

func TestUpdate_EscapeCreateMode(t *testing.T) {
	m := testModel(2)
	m.mode = modeCreate

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", model.mode)
	}
}

func TestUpdate_DeleteMainBlocked(t *testing.T) {
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
}

func TestUpdate_DeleteNonMain(t *testing.T) {
	m := testModel(2)
	m.cursor = 1 // non-main worktree

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.mode != modeConfirmDelete {
		t.Errorf("mode = %d, want modeConfirmDelete", model.mode)
	}
}

func TestUpdate_ConfirmDeleteCancel(t *testing.T) {
	m := testModel(2)
	m.mode = modeConfirmDelete
	m.cursor = 1

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal after cancel", model.mode)
	}
}

func TestUpdate_ConfirmDeleteRequiresEnterAfterY(t *testing.T) {
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
}

func TestUpdate_ConfirmDeleteEnterWithoutYCancels(t *testing.T) {
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
}

func TestUpdate_ConfirmDeleteEscCancels(t *testing.T) {
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
}

func TestUpdate_WindowSize(t *testing.T) {
	m := testModel(2)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.width != 120 || model.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", model.width, model.height)
	}
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
