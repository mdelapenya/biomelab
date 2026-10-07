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
	colorBackground  color.NRGBA // App canvas behind cards and panels.
	colorPanelBg     color.NRGBA // Sidebar and persistent panel surface.
	colorCardBg      color.NRGBA // Elevated cards, dialogs, menus, and inputs.
	colorSecondaryBg color.NRGBA // Subtle inset sections within a card.
	colorBorder      color.NRGBA
	colorActionBg    color.NRGBA // Filled primary actions, with colorOnAction text.
	colorOnAction    color.NRGBA
	colorSelected    color.NRGBA
	colorBranch      color.NRGBA
	colorGreen       color.NRGBA
	colorBlue        color.NRGBA
	colorYellow      color.NRGBA
	colorRed         color.NRGBA
	colorPurple      color.NRGBA
	colorGray        color.NRGBA
	colorDimGray     color.NRGBA
	colorForeground  color.NRGBA
	colorSelection   color.NRGBA
	colorHover       color.NRGBA
	colorShadow      color.NRGBA
)

func init() {
	applyDarkPalette()
}

// applyDarkPalette installs calm charcoal surfaces with restrained accents.
func applyDarkPalette() {
	colorBackground = color.NRGBA{R: 43, G: 43, B: 46, A: 255}
	colorPanelBg = color.NRGBA{R: 36, G: 36, B: 39, A: 255}
	colorCardBg = color.NRGBA{R: 40, G: 40, B: 43, A: 255}
	colorSecondaryBg = color.NRGBA{R: 51, G: 51, B: 55, A: 255}
	colorBorder = color.NRGBA{R: 65, G: 65, B: 70, A: 255}
	colorBlue = color.NRGBA{R: 120, G: 163, B: 228, A: 255}
	colorSelected = colorBlue
	colorActionBg = color.NRGBA{R: 57, G: 113, B: 200, A: 255}
	colorOnAction = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorPurple = color.NRGBA{R: 180, G: 160, B: 221, A: 255}
	colorBranch = colorPurple
	colorGreen = color.NRGBA{R: 129, G: 195, B: 154, A: 255}
	colorYellow = color.NRGBA{R: 217, G: 181, B: 108, A: 255}
	colorRed = color.NRGBA{R: 235, G: 146, B: 152, A: 255}
	colorGray = color.NRGBA{R: 189, G: 195, B: 205, A: 255}
	colorDimGray = color.NRGBA{R: 170, G: 170, B: 180, A: 255}
	colorForeground = color.NRGBA{R: 236, G: 236, B: 241, A: 255}
	colorSelection = color.NRGBA{R: colorBlue.R, G: colorBlue.G, B: colorBlue.B, A: 38}
	colorHover = color.NRGBA{R: 189, G: 195, B: 205, A: 20}
	colorShadow = color.NRGBA{A: 70}
}

// applyLightPalette installs cool-gray chrome and white card surfaces.
func applyLightPalette() {
	colorBackground = color.NRGBA{R: 250, G: 250, B: 251, A: 255}
	colorPanelBg = color.NRGBA{R: 240, G: 240, B: 242, A: 255}
	colorCardBg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorSecondaryBg = color.NRGBA{R: 245, G: 245, B: 247, A: 255}
	colorBorder = color.NRGBA{R: 224, G: 224, B: 229, A: 255}
	colorBlue = color.NRGBA{R: 57, G: 113, B: 200, A: 255}
	colorSelected = colorBlue
	colorActionBg = color.NRGBA{R: 57, G: 113, B: 200, A: 255}
	colorOnAction = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorPurple = color.NRGBA{R: 117, G: 83, B: 176, A: 255}
	colorBranch = colorPurple
	colorGreen = color.NRGBA{R: 39, G: 117, B: 72, A: 255}
	colorYellow = color.NRGBA{R: 149, G: 105, B: 0, A: 255}
	colorRed = color.NRGBA{R: 190, G: 63, B: 72, A: 255}
	colorGray = color.NRGBA{R: 86, G: 97, B: 112, A: 255}
	colorDimGray = color.NRGBA{R: 101, G: 101, B: 111, A: 255}
	colorForeground = color.NRGBA{R: 48, G: 48, B: 56, A: 255}
	colorSelection = color.NRGBA{R: colorBlue.R, G: colorBlue.G, B: colorBlue.B, A: 26}
	colorHover = color.NRGBA{R: 86, G: 97, B: 112, A: 16}
	colorShadow = color.NRGBA{R: 48, G: 48, B: 56, A: 24}
}

const (
	defaultTextSize float32 = textBodySize
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

// NewTheme creates the application theme for standalone GUI renderers.
// It also installs the widget palette, so call it before constructing widgets.
// Like runtime theme changes, it must not run concurrently with GUI rendering.
func NewTheme(v ThemeVariant) fyne.Theme {
	return newBiomeTheme(v)
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
		return colorActionBg
	case theme.ColorNameHyperlink:
		return colorBlue
	case theme.ColorNameSuccess:
		return colorGreen
	case theme.ColorNameWarning:
		return colorYellow
	case theme.ColorNameError:
		return colorRed
	case theme.ColorNameForegroundOnPrimary:
		return colorOnAction
	case theme.ColorNameForegroundOnSuccess,
		theme.ColorNameForegroundOnWarning, theme.ColorNameForegroundOnError:
		if t.variant == VariantLight {
			return colorCardBg
		}
		return colorBackground
	case theme.ColorNameFocus:
		// Fyne overlays focus on ordinary buttons without changing their text.
		// A translucent action accent also preserves filled-action contrast.
		focus := colorActionBg
		focus.A = colorSelection.A
		return focus
	case theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameInputBackground:
		return colorCardBg
	case theme.ColorNameInputBorder:
		return colorBorder
	case theme.ColorNameButton:
		return colorSecondaryBg
	case theme.ColorNameScrollBar:
		return colorBorder
	case theme.ColorNameShadow:
		return colorShadow
	case theme.ColorNameOverlayBackground:
		return colorCardBg
	case theme.ColorNameHeaderBackground:
		return colorPanelBg
	case theme.ColorNameMenuBackground:
		return colorCardBg
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
	return theme.DefaultTheme().Font(style)
}

func (t *biomeTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *biomeTheme) Size(name fyne.ThemeSizeName) float32 {
	// Use this receiver's zoom; scaledSize reads the currently installed theme.
	scale := t.textSize / defaultTextSize
	switch name {
	case theme.SizeNameText:
		return t.textSize
	case theme.SizeNameHeadingText:
		return textTitleSize * scale
	case theme.SizeNameSubHeadingText:
		return textHeadingSize * scale
	case theme.SizeNameCaptionText:
		return textSecondarySize * scale
	case theme.SizeNamePadding:
		return spaceXS * scale
	case theme.SizeNameInnerPadding:
		return spaceXS * scale
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return radiusControl * scale
	default:
		return theme.DefaultTheme().Size(name)
	}
}
