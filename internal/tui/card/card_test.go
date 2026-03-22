package card

import (
	"strings"
	"testing"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/github"
)

func TestRender_CleanNoAgent(t *testing.T) {
	wt := git.Worktree{
		Path:   "/home/user/project",
		Branch: "main",
		IsMain: true,
	}

	got := Render(wt, nil, nil, github.GHAvailable)

	if !strings.Contains(got, "main") {
		t.Error("expected branch name in output")
	}
	if !strings.Contains(got, "no agent") {
		t.Error("expected 'no agent' indicator")
	}
	if !strings.Contains(got, "clean") {
		t.Error("expected 'clean' indicator")
	}
}

func TestRender_DirtyWithAgent(t *testing.T) {
	wt := git.Worktree{
		Path:    "/home/user/feature",
		Branch:  "feature-auth",
		IsDirty: true,
	}
	agents := []agent.Info{
		{Kind: agent.Claude, PID: "12345"},
	}

	got := Render(wt, agents, nil, github.GHAvailable)

	if !strings.Contains(got, "feature-auth") {
		t.Error("expected branch name in output")
	}
	if !strings.Contains(got, "claude") {
		t.Error("expected agent name in output")
	}
	if !strings.Contains(got, "12345") {
		t.Error("expected PID in output")
	}
	if !strings.Contains(got, "dirty") {
		t.Error("expected 'dirty' indicator")
	}
}

func TestRender_MultipleAgents(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/work",
		Branch: "dev",
	}
	agents := []agent.Info{
		{Kind: agent.Claude, PID: "100"},
		{Kind: agent.Copilot, PID: "200"},
	}

	got := Render(wt, agents, nil, github.GHAvailable)

	if !strings.Contains(got, "claude") {
		t.Error("expected claude in output")
	}
	if !strings.Contains(got, "copilot") {
		t.Error("expected copilot in output")
	}
}

func TestRender_DetachedHead(t *testing.T) {
	wt := git.Worktree{
		Path:     "/tmp/detached",
		Branch:   "abc1234",
		Detached: true,
	}

	got := Render(wt, nil, nil, github.GHAvailable)

	if !strings.Contains(got, "detached") {
		t.Error("expected 'detached' in output")
	}
}

func TestRender_WithPR(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}
	pr := &github.PRInfo{
		Number:      42,
		Title:       "Add feature X",
		State:       "open",
		CheckStatus: "success",
	}

	got := Render(wt, nil, pr, github.GHAvailable)

	if !strings.Contains(got, "#42") {
		t.Error("expected PR number in output")
	}
	if !strings.Contains(got, "Add feature X") {
		t.Error("expected PR title in output")
	}
	if !strings.Contains(got, "open") {
		t.Error("expected PR state in output")
	}
}

func TestRender_WithDraftPR(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/wip",
		Branch: "wip-branch",
	}
	pr := &github.PRInfo{
		Number:      99,
		Title:       "WIP: something",
		State:       "open",
		Draft:       true,
		CheckStatus: "pending",
	}

	got := Render(wt, nil, pr, github.GHAvailable)

	if !strings.Contains(got, "draft") {
		t.Error("expected 'draft' state in output")
	}
	if !strings.Contains(got, "#99") {
		t.Error("expected PR number in output")
	}
}

func TestRender_GHNotFound(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, github.GHNotFound)

	if !strings.Contains(got, "gh not installed") {
		t.Error("expected 'gh not installed' message")
	}
	if !strings.Contains(got, "install gh CLI") {
		t.Error("expected install instruction")
	}
}

func TestRender_GHNotAuthenticated(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, github.GHNotAuthenticated)

	if !strings.Contains(got, "gh not authenticated") {
		t.Error("expected 'gh not authenticated' message")
	}
	if !strings.Contains(got, "gh auth login") {
		t.Error("expected auth login instruction")
	}
}

func TestRender_PRTakesPrecedenceOverGHStatus(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}
	pr := &github.PRInfo{
		Number: 7,
		Title:  "Fix bug",
		State:  "open",
	}

	// Even with GHNotAuthenticated, if we somehow have a PR, show it.
	got := Render(wt, nil, pr, github.GHNotAuthenticated)

	if !strings.Contains(got, "#7") {
		t.Error("expected PR number in output")
	}
	if strings.Contains(got, "gh not authenticated") {
		t.Error("should not show auth message when PR data is available")
	}
}
