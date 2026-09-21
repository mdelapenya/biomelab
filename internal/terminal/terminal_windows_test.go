package terminal

import (
	"strings"
	"testing"
)

func TestWindowsLaunchRejectsPOSIXRecipeBeforeSpawn(t *testing.T) {
	for _, configured := range []string{"", "wt.exe", "powershell.exe", "missing-custom-terminal.exe"} {
		t.Run(configured, func(t *testing.T) {
			t.Setenv("BIOME_TERMINAL", configured)
			for _, open := range []func() error{
				func() error { return Open(`C:\work tree`, "") },
				func() error { return OpenWithTitle(`C:\work tree`, "", "branch") },
			} {
				if err := open(); err == nil || !strings.Contains(err.Error(), "not supported yet") {
					t.Fatalf("want pre-launch unsupported error, got %v", err)
				}
			}
		})
	}
}
