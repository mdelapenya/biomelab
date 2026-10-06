package gui

import (
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
	"testing"
	"time"
)

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
