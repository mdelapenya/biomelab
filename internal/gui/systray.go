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

	toggleItem := fyne.NewMenuItem("Hide", nil)
	toggleItem.Action = func() {
		if a.window.Content().Visible() {
			a.window.Hide()
			toggleItem.Label = "Show"
		} else {
			a.window.Show()
			toggleItem.Label = "Hide"
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

	// Update label when window is hidden via close button.
	a.window.SetCloseIntercept(func() {
		a.window.Hide()
		toggleItem.Label = "Show"
		desk.SetSystemTrayMenu(a.trayMenu)
	})
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
// or "Dependencies: 1 missing" when something is wrong. Builds from a fresh
// cache read so the count reflects the latest probe (the cache itself
// memoizes for sysdepsCacheTTL, so this is cheap to call on menu refresh).
func (a *App) sysdepsSummaryLabel() string {
	cfg := a.loadConfigForSysDeps()
	reps := sysdeps.ApplyVisibility(
		sysdeps.ApplySuppression(a.sysdepsCache.Get(cfg)),
		cfg,
	)
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

// refreshSysdepsTray re-probes the dependency cache and updates the systray
// label and menu. Call after actions that could change the dep state
// (e.g. dialog Re-check, future install actions).
func (a *App) refreshSysdepsTray() {
	if a.trayDepsItem == nil {
		return
	}
	a.sysdepsCache.Invalidate()
	a.trayDepsItem.Label = a.sysdepsSummaryLabel()
	desk, ok := a.fyneApp.(desktop.App)
	if ok && a.trayMenu != nil {
		desk.SetSystemTrayMenu(a.trayMenu)
	}
}

func (a *App) stopAllRefresh() {
	for _, session := range a.terminalSessions {
		session.Cleanup()
	}
	for _, re := range a.repos {
		if re.refreshMgr != nil {
			re.refreshMgr.Stop()
		}
	}
}
