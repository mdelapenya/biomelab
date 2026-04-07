package card

import (
	"strings"
	"testing"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/ide"
	"github.com/mdelapenya/gwaim/internal/provider"
)

func TestRender_CleanNoAgent(t *testing.T) {
	wt := git.Worktree{
		Path:   "/home/user/project",
		Branch: "main",
		IsMain: true,
	}

	got := Render(wt, nil, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

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

	got := Render(wt, agents, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

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

	got := Render(wt, agents, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "claude") {
		t.Error("expected claude in output")
	}
	if !strings.Contains(got, "copilot") {
		t.Error("expected copilot in output")
	}
}

func TestRender_SubAgent(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/work",
		Branch: "feature",
	}
	agents := []agent.Info{
		{Kind: agent.Claude, PID: "100"},
		{Kind: agent.Claude, PID: "200", IsSubAgent: true},
	}

	got := Render(wt, agents, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "↳") {
		t.Error("expected subagent indent marker '↳' in output")
	}
	if strings.Count(got, "●") != 1 {
		t.Errorf("expected exactly 1 top-level bullet, got %d", strings.Count(got, "●"))
	}
}

func TestRender_DetachedHead(t *testing.T) {
	wt := git.Worktree{
		Path:     "/tmp/detached",
		Branch:   "abc1234",
		Detached: true,
	}

	got := Render(wt, nil, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "detached") {
		t.Error("expected 'detached' in output")
	}
}

func TestRender_WithPR(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}
	pr := &provider.PRInfo{
		Number:      42,
		Title:       "Add feature X",
		State:       "open",
		CheckStatus: "success",
	}

	got := Render(wt, nil, nil, pr, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "#42") {
		t.Error("expected PR number in output")
	}
	if !strings.Contains(got, "Add feature X") {
		t.Error("expected PR title in output")
	}
	if !strings.Contains(got, "open") {
		t.Error("expected PR state in output")
	}
	if !strings.Contains(got, "PR") {
		t.Error("expected PR label in output")
	}
}

func TestRender_WithDraftPR(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/wip",
		Branch: "wip-branch",
	}
	pr := &provider.PRInfo{
		Number:      99,
		Title:       "WIP: something",
		State:       "open",
		Draft:       true,
		CheckStatus: "pending",
	}

	got := Render(wt, nil, nil, pr, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "draft") {
		t.Error("expected 'draft' state in output")
	}
	if !strings.Contains(got, "#99") {
		t.Error("expected PR number in output")
	}
}

func TestRender_CLINotFound(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, nil, provider.CLINotFound, provider.ProviderGitHub)

	if !strings.Contains(got, "gh not installed") {
		t.Error("expected 'gh not installed' message")
	}
	if !strings.Contains(got, "install gh CLI") {
		t.Error("expected install instruction")
	}
}

func TestRender_CLINotAuthenticated(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, nil, provider.CLINotAuthenticated, provider.ProviderGitHub)

	if !strings.Contains(got, "gh not authenticated") {
		t.Error("expected 'gh not authenticated' message")
	}
	if !strings.Contains(got, "gh auth login") {
		t.Error("expected auth login instruction")
	}
}

func TestRender_PRTakesPrecedenceOverCLIStatus(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}
	pr := &provider.PRInfo{
		Number: 7,
		Title:  "Fix bug",
		State:  "open",
	}

	// Even with CLINotAuthenticated, if we somehow have a PR, show it.
	got := Render(wt, nil, nil, pr, provider.CLINotAuthenticated, provider.ProviderGitHub)

	if !strings.Contains(got, "#7") {
		t.Error("expected PR number in output")
	}
	if strings.Contains(got, "gh not authenticated") {
		t.Error("should not show auth message when PR data is available")
	}
}

func TestRender_UnsupportedProvider_ShowsMessage(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, nil, provider.CLIUnsupportedProvider, provider.ProviderUnknown)

	if !strings.Contains(got, "not yet supported") {
		t.Error("expected 'not yet supported' message, got: " + got)
	}
}

func TestRender_GitLabProvider_UsesMRLabel(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}
	mr := &provider.PRInfo{
		Number: 42,
		Title:  "Add feature X",
		State:  "open",
	}

	got := Render(wt, nil, nil, mr, provider.CLIAvailable, provider.ProviderGitLab)

	if !strings.Contains(got, "MR") {
		t.Error("expected MR label for GitLab provider")
	}
	if !strings.Contains(got, "#42") {
		t.Error("expected MR number in output")
	}
}

func TestRender_GitLabNotFound(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, nil, provider.CLINotFound, provider.ProviderGitLab)

	if !strings.Contains(got, "glab not installed") {
		t.Error("expected 'glab not installed' message, got: " + got)
	}
	if !strings.Contains(got, "install glab CLI") {
		t.Error("expected install instruction for glab")
	}
}

func TestRender_GitLabNotAuthenticated(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/feature",
		Branch: "feature-x",
	}

	got := Render(wt, nil, nil, nil, provider.CLINotAuthenticated, provider.ProviderGitLab)

	if !strings.Contains(got, "glab not authenticated") {
		t.Error("expected 'glab not authenticated' message, got: " + got)
	}
	if !strings.Contains(got, "glab auth login") {
		t.Error("expected auth login instruction for glab")
	}
}

func TestRender_WithIDE(t *testing.T) {
	wt := git.Worktree{
		Path:   "/home/user/project",
		Branch: "feature",
	}
	ides := []ide.Info{
		{Kind: ide.VSCode, PID: 42},
	}

	got := Render(wt, nil, ides, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "vscode") {
		t.Error("expected IDE kind in output")
	}
	if !strings.Contains(got, "42") {
		t.Error("expected IDE PID in output")
	}
}

func TestRender_NoIDE(t *testing.T) {
	wt := git.Worktree{
		Path:   "/home/user/project",
		Branch: "main",
		IsMain: true,
	}

	got := Render(wt, nil, nil, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "no IDE") {
		t.Error("expected 'no IDE' indicator")
	}
}

func TestRender_MultipleIDEs(t *testing.T) {
	wt := git.Worktree{
		Path:   "/tmp/work",
		Branch: "dev",
	}
	ides := []ide.Info{
		{Kind: ide.VSCode, PID: 100},
		{Kind: ide.Neovim, PID: 200},
	}

	got := Render(wt, nil, ides, nil, provider.CLIAvailable, provider.ProviderGitHub)

	if !strings.Contains(got, "vscode") {
		t.Error("expected vscode in output")
	}
	if !strings.Contains(got, "neovim") {
		t.Error("expected neovim in output")
	}
}
