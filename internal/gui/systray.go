package gui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

// setupSystemTray creates a system tray icon with Show/Hide toggle, Theme
// submenu, and Quit.
func (a *App) setupSystemTray() {
	desk, ok := a.fyneApp.(desktop.App)
	if !ok {
		return
	}

	a.mainWindowVisible = true
	toggleItem := fyne.NewMenuItem("Hide", nil)
	toggleItem.Action = func() {
		if a.toggleMainWindowVisible() {
			toggleItem.Label = "Hide"
		} else {
			toggleItem.Label = "Show"
		}
		desk.SetSystemTrayMenu(a.trayMenu)
	}

	// Theme submenu — Light / Dark with a checkmark on the active variant.
	a.trayThemeLight = fyne.NewMenuItem("Light", func() {
		a.applyThemeVariant(VariantLight)
	})
	a.trayThemeDark = fyne.NewMenuItem("Dark", func() {
		a.applyThemeVariant(VariantDark)
	})
	a.trayThemeLight.Checked = a.theme.Variant() == VariantLight
	a.trayThemeDark.Checked = a.theme.Variant() == VariantDark
	themeItem := fyne.NewMenuItem("Theme", nil)
	themeItem.ChildMenu = fyne.NewMenu("", a.trayThemeLight, a.trayThemeDark)

	configItem := fyne.NewMenuItem("Show Config", func() {
		if err := a.openConfigFile(); err != nil {
			dialog.ShowError(err, a.window)
		}
	})

	sbxDocsItem := fyne.NewMenuItem("Docker Sandboxes docs", func() {
		if u, err := url.Parse(sbxInstallURL); err == nil {
			_ = a.fyneApp.OpenURL(u)
		}
	})

	a.trayDepsItem = fyne.NewMenuItem(a.sysdepsSummaryLabel(), func() {
		a.showSysDepsDialog()
	})

	a.trayMenu = fyne.NewMenu("biomelab",
		toggleItem,
		fyne.NewMenuItemSeparator(),
		themeItem,
		configItem,
		sbxDocsItem,
		a.trayDepsItem,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			a.stopAllRefresh()
			a.fyneApp.Quit()
		}),
	)
	desk.SetSystemTrayMenu(a.trayMenu)
	desk.SetSystemTrayIcon(AppIcon)
	a.requestSysdepsRefresh(false, nil)

	// Update label when window is hidden via close button.
	a.window.SetCloseIntercept(func() {
		a.setMainWindowVisible(false)
		toggleItem.Label = "Show"
		desk.SetSystemTrayMenu(a.trayMenu)
	})
}

func (a *App) toggleMainWindowVisible() bool {
	a.setMainWindowVisible(!a.mainWindowVisible)
	return a.mainWindowVisible
}

func (a *App) setMainWindowVisible(visible bool) {
	a.mainWindowVisible = visible
	if visible {
		a.window.Show()
	} else {
		a.window.Hide()
	}
}

// openConfigFile opens the config file with the system's default application.
// If the file does not exist yet (fresh install), an empty config is written
// first so the editor has something to open.
func (a *App) openConfigFile() error {
	if _, err := os.Stat(a.configPath); errors.Is(err, os.ErrNotExist) {
		if err := config.Save(a.configPath, &config.Config{}); err != nil {
			return err
		}
	}
	return openInSystem(a.configPath)
}

// openInSystem opens path with the OS default handler.
func openInSystem(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// sysdepsSummaryLabel returns the systray label like "Dependencies: 3/4 ✓"
// or "Dependencies: 1 missing" when something is wrong. This only reads the
// last snapshot; command probes always run off the UI thread.
func (a *App) sysdepsSummaryLabel() string {
	raw, ok := a.sysdepsCache.Peek()
	if !ok {
		return "Dependencies: checking…"
	}
	reps := a.visibleSysDeps(raw)
	primary, _ := sysdeps.Partition(reps)
	c := sysdeps.Summarize(primary)
	total := c.Total()
	if total == 0 {
		return "Dependencies"
	}
	if c.Missing == 0 && c.Degraded == 0 {
		return fmt.Sprintf("Dependencies: %d/%d ✓", c.OK, total)
	}
	missing := c.Missing + c.Degraded
	return fmt.Sprintf("Dependencies: %d/%d (%d need attention)", c.OK, total, missing)
}

// refreshSysdepsTray updates the menu from the current snapshot without
// invalidating it or running commands on the event loop.
func (a *App) refreshSysdepsTray() {
	if a.trayDepsItem == nil {
		return
	}
	a.trayDepsItem.Label = a.sysdepsSummaryLabel()
	desk, ok := a.fyneApp.(desktop.App)
	if ok && a.trayMenu != nil {
		desk.SetSystemTrayMenu(a.trayMenu)
	}
}

// requestSysdepsRefresh is called on the event loop. A generation check keeps
// late results from updating widgets after a newer request or app shutdown.
func (a *App) requestSysdepsRefresh(force bool, done func([]sysdeps.Reported)) {
	if force {
		a.sysdepsGeneration++
		a.sysdepsCache.Invalidate()
	}
	generation := a.sysdepsGeneration
	go func() {
		raw := a.sysdepsCache.Get(nil)
		fyne.Do(func() {
			if a.sysdepsClosed {
				return
			}
			if generation != a.sysdepsGeneration {
				// A forced refresh superseded this snapshot. Keep an open
				// dialog's callback alive; it will share the current probe.
				if done != nil {
					a.requestSysdepsRefresh(false, done)
				}
				return
			}
			reps := a.visibleSysDeps(raw)
			a.refreshSysdepsTray()
			a.updateSysdepsBanner(reps)
			if done != nil {
				done(reps)
			}
		})
	}()
}

func (a *App) stopAllRefresh() {
	a.sysdepsClosed = true
	a.sysdepsGeneration++
	for _, session := range a.terminalSessions {
		session.Cleanup()
	}
	for _, re := range a.repos {
		if re.refreshMgr != nil {
			re.refreshMgr.Stop()
		}
	}
}
