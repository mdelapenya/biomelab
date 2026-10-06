package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

func TestVisibleSysDepsKeepsMissingCLIForConfiguredProvider(t *testing.T) {
	gh := sysdeps.Reported{Check: sysdeps.Check{Name: "gh", SuppressIfAny: []string{"glab"}}, Result: sysdeps.Result{Status: sysdeps.StatusMissing}}
	glab := sysdeps.Reported{Check: sysdeps.Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: sysdeps.Result{Status: sysdeps.StatusOK}}
	a := &App{repos: []*repoEntry{
		{group: &RepoGroup{Modes: []config.ModeEntry{{Type: "regular"}}}, state: &RepoState{Provider: provider.ProviderGitHub}},
		{group: &RepoGroup{Modes: []config.ModeEntry{{Type: "regular"}}}, state: &RepoState{Provider: provider.ProviderGitLab}},
	}}
	got := a.visibleSysDeps([]sysdeps.Reported{gh, glab})
	if len(got) != 2 || got[0].Check.Name != "gh" {
		t.Fatalf("configured GitHub repo lost missing gh warning: %+v", got)
	}
	a.repos = a.repos[1:]
	got = a.visibleSysDeps([]sysdeps.Reported{gh, glab})
	if len(got) != 1 || got[0].Check.Name != "glab" {
		t.Fatalf("unneeded gh warning should be suppressed: %+v", got)
	}
}

func TestConcurrentSysdepsDialogsBothReceiveProbe(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	entered := make(chan struct{})
	release := make(chan struct{})
	cache := sysdeps.NewCache(time.Hour)
	cache.SetChecks([]sysdeps.Check{{Name: "gh", Probe: func() sysdeps.Result {
		close(entered)
		<-release
		return sysdeps.Result{Status: sysdeps.StatusOK}
	}}})
	a := &App{sysdepsCache: cache}
	first := make(chan struct{}, 1)
	second := make(chan struct{}, 1)
	a.requestSysdepsRefresh(false, func([]sysdeps.Reported) { first <- struct{}{} })
	<-entered
	a.requestSysdepsRefresh(false, func([]sysdeps.Reported) { second <- struct{}{} })
	close(release)
	for _, ch := range []chan struct{}{first, second} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("dependency dialog did not receive the shared probe")
		}
	}
}

func TestSysdepsSummaryDoesNotRunProbeOnEventLoop(t *testing.T) {
	cache := sysdeps.NewCache(time.Hour)
	cache.SetChecks([]sysdeps.Check{{Name: "slow", Probe: func() sysdeps.Result {
		t.Fatal("summary ran a probe synchronously")
		return sysdeps.Result{}
	}}})
	a := &App{sysdepsCache: cache}
	if got := a.sysdepsSummaryLabel(); got != "Dependencies: checking…" {
		t.Fatalf("empty snapshot label = %q", got)
	}
}
