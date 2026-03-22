package main

import (
	"testing"
	"time"

	"github.com/mdelapenya/gwaim/internal/tui"
)

func TestResolveRefreshInterval_FlagTakesPrecedence(t *testing.T) {
	t.Setenv("GWAIM_REFRESH", "30s")
	got := resolveRefreshInterval(10 * time.Second)
	if got != 10*time.Second {
		t.Errorf("got %v, want 10s (flag should override env)", got)
	}
}

func TestResolveRefreshInterval_EnvFallback(t *testing.T) {
	t.Setenv("GWAIM_REFRESH", "15s")
	got := resolveRefreshInterval(0)
	if got != 15*time.Second {
		t.Errorf("got %v, want 15s (env var fallback)", got)
	}
}

func TestResolveRefreshInterval_Default(t *testing.T) {
	t.Setenv("GWAIM_REFRESH", "")
	got := resolveRefreshInterval(0)
	if got != tui.DefaultRefreshInterval {
		t.Errorf("got %v, want %v (default)", got, tui.DefaultRefreshInterval)
	}
}
