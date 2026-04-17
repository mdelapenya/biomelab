package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ThemeVariant selects which palette biomeTheme applies.
type ThemeVariant string

const (
	VariantDark  ThemeVariant = "dark"
	VariantLight ThemeVariant = "light"
)

// Active palette. These package-level vars are read directly by card.go,
// dashboard.go, repo_panel.go, dialogs.go, and app.go. They are reassigned
// by applyDarkPalette / applyLightPalette whenever the theme variant changes.
// After swapping, a full rebuild (Dashboard.Rebuild / RepoPanel.RebuildFull)
// is required because canvas primitives capture colors at construction time.
var (
	colorBackground color.NRGBA
	colorPanelBg    color.NRGBA
	colorBorder     color.NRGBA
	colorSelected   color.NRGBA
	colorBranch     color.NRGBA
	colorGreen      color.NRGBA
	colorBlue       color.NRGBA
	colorYellow     color.NRGBA
	colorRed        color.NRGBA
	colorPurple     color.NRGBA
	colorGray       color.NRGBA
	colorDimGray    color.NRGBA
	colorForeground color.NRGBA
	colorSelection  color.NRGBA
	colorHover      color.NRGBA
	colorShadow     color.NRGBA
)

func init() {
	applyDarkPalette()
}

// applyDarkPalette installs the default GitHub-inspired dark palette.
func applyDarkPalette() {
	colorBackground = color.NRGBA{R: 28, G: 33, B: 40, A: 255}    // #1c2128
	colorPanelBg = color.NRGBA{R: 22, G: 27, B: 34, A: 255}       // #161b22
	colorBorder = color.NRGBA{R: 48, G: 54, B: 61, A: 255}        // #30363d
	colorSelected = color.NRGBA{R: 0, G: 212, B: 255, A: 255}     // #00d4ff cyan
	colorBranch = color.NRGBA{R: 255, G: 110, B: 199, A: 255}     // #ff6ec7 hot pink
	colorGreen = color.NRGBA{R: 46, G: 204, B: 113, A: 255}       // #2ecc71
	colorBlue = color.NRGBA{R: 88, G: 166, B: 255, A: 255}        // #58a6ff
	colorYellow = color.NRGBA{R: 243, G: 156, B: 18, A: 255}      // #f39c12
	colorRed = color.NRGBA{R: 231, G: 76, B: 60, A: 255}          // #e74c3c
	colorPurple = color.NRGBA{R: 135, G: 95, B: 215, A: 255}      // #8760d7
	colorGray = color.NRGBA{R: 139, G: 148, B: 158, A: 255}       // #8b949e
	colorDimGray = color.NRGBA{R: 72, G: 79, B: 88, A: 255}       // #484f58
	colorForeground = color.NRGBA{R: 230, G: 237, B: 243, A: 255} // #e6edf3
	colorSelection = color.NRGBA{R: 0, G: 212, B: 255, A: 60}
	colorHover = color.NRGBA{R: 48, G: 54, B: 61, A: 128}
	colorShadow = color.NRGBA{R: 0, G: 0, B: 0, A: 80}
}

// applyLightPalette installs a GitHub-inspired light palette.
func applyLightPalette() {
	colorBackground = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // #ffffff
	colorPanelBg = color.NRGBA{R: 246, G: 248, B: 250, A: 255}    // #f6f8fa
	colorBorder = color.NRGBA{R: 208, G: 215, B: 222, A: 255}     // #d0d7de
	colorSelected = color.NRGBA{R: 9, G: 105, B: 218, A: 255}     // #0969da blue
	colorBranch = color.NRGBA{R: 130, G: 80, B: 223, A: 255}      // #8250df purple
	colorGreen = color.NRGBA{R: 26, G: 127, B: 55, A: 255}        // #1a7f37
	colorBlue = color.NRGBA{R: 9, G: 105, B: 218, A: 255}         // #0969da
	colorYellow = color.NRGBA{R: 154, G: 103, B: 0, A: 255}       // #9a6700
	colorRed = color.NRGBA{R: 207, G: 34, B: 46, A: 255}          // #cf222e
	colorPurple = color.NRGBA{R: 130, G: 80, B: 223, A: 255}      // #8250df
	colorGray = color.NRGBA{R: 87, G: 96, B: 106, A: 255}         // #57606a
	colorDimGray = color.NRGBA{R: 140, G: 149, B: 159, A: 255}    // #8c959f
	colorForeground = color.NRGBA{R: 31, G: 35, B: 40, A: 255}    // #1f2328
	colorSelection = color.NRGBA{R: 9, G: 105, B: 218, A: 40}
	colorHover = color.NRGBA{R: 208, G: 215, B: 222, A: 120}
	colorShadow = color.NRGBA{R: 31, G: 35, B: 40, A: 40}
}

const (
	defaultTextSize float32 = 14
	minTextSize     float32 = 10
	maxTextSize     float32 = 24
	textSizeStep    float32 = 2
)

// biomeTheme implements fyne.Theme with a swappable light/dark palette.
// textSize can be adjusted at runtime via Ctrl+/Ctrl-; the variant is
// toggled via Ctrl+T and persisted through the Config.
type biomeTheme struct {
	textSize float32
	variant  ThemeVariant
}

func newBiomeTheme(v ThemeVariant) *biomeTheme {
	if v != VariantLight {
		v = VariantDark
	}
	t := &biomeTheme{textSize: defaultTextSize, variant: v}
	t.applyPalette()
	return t
}

// Variant returns the current theme variant.
func (t *biomeTheme) Variant() ThemeVariant { return t.variant }

// SetVariant switches the active palette.
func (t *biomeTheme) SetVariant(v ThemeVariant) {
	if v != VariantLight {
		v = VariantDark
	}
	t.variant = v
	t.applyPalette()
}

// Toggle flips between dark and light and returns the new variant.
func (t *biomeTheme) Toggle() ThemeVariant {
	if t.variant == VariantDark {
		t.SetVariant(VariantLight)
	} else {
		t.SetVariant(VariantDark)
	}
	return t.variant
}

func (t *biomeTheme) applyPalette() {
	if t.variant == VariantLight {
		applyLightPalette()
	} else {
		applyDarkPalette()
	}
}

// ZoomIn increases the font size.
func (t *biomeTheme) ZoomIn() {
	if t.textSize < maxTextSize {
		t.textSize += textSizeStep
	}
}

// ZoomOut decreases the font size.
func (t *biomeTheme) ZoomOut() {
	if t.textSize > minTextSize {
		t.textSize -= textSizeStep
	}
}

// ZoomReset restores the default font size.
func (t *biomeTheme) ZoomReset() {
	t.textSize = defaultTextSize
}

// fyneVariant maps our variant to Fyne's variant constant so default-theme
// fallbacks (colors we don't override) render correctly in light mode too.
func (t *biomeTheme) fyneVariant() fyne.ThemeVariant {
	if t.variant == VariantLight {
		return theme.VariantLight
	}
	return theme.VariantDark
}

func (t *biomeTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	v := t.fyneVariant()
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameForeground:
		return colorForeground
	case theme.ColorNamePrimary:
		return colorSelected
	case theme.ColorNameFocus:
		return colorSelected
	case theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameInputBackground:
		return colorPanelBg
	case theme.ColorNameInputBorder:
		return colorBorder
	case theme.ColorNameButton:
		return colorBorder
	case theme.ColorNameScrollBar:
		return colorBorder
	case theme.ColorNameShadow:
		return colorShadow
	case theme.ColorNameOverlayBackground:
		return colorPanelBg
	case theme.ColorNameHeaderBackground:
		return colorPanelBg
	case theme.ColorNameMenuBackground:
		return colorPanelBg
	case theme.ColorNamePlaceHolder:
		return colorDimGray
	case theme.ColorNameDisabled:
		return colorDimGray
	case theme.ColorNameSelection:
		return colorSelection
	case theme.ColorNameHover:
		return colorHover
	default:
		return theme.DefaultTheme().Color(name, v)
	}
}

func (t *biomeTheme) Font(style fyne.TextStyle) fyne.Resource {
	style.Monospace = true
	return theme.DefaultTheme().Font(style)
}

func (t *biomeTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *biomeTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return t.textSize
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 4
	default:
		return theme.DefaultTheme().Size(name)
	}
}
