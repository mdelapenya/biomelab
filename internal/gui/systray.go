package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
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

	a.trayMenu = fyne.NewMenu("biomelab",
		toggleItem,
		fyne.NewMenuItemSeparator(),
		themeItem,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			a.stopAllRefresh()
			if a.termPanel != nil {
				a.termPanel.CloseAll()
			}
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

func (a *App) stopAllRefresh() {
	for _, re := range a.repos {
		if re.refreshMgr != nil {
			re.refreshMgr.Stop()
		}
	}
}
