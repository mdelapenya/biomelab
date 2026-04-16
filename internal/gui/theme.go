package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Dark-mode color palette matching the hero-dashboard reference design.
var (
	colorBackground = color.NRGBA{R: 28, G: 33, B: 40, A: 255}    // #1c2128
	colorPanelBg    = color.NRGBA{R: 22, G: 27, B: 34, A: 255}    // #161b22
	colorBorder     = color.NRGBA{R: 48, G: 54, B: 61, A: 255}    // #30363d
	colorSelected   = color.NRGBA{R: 0, G: 212, B: 255, A: 255}   // #00d4ff (cyan)
	colorBranch     = color.NRGBA{R: 255, G: 110, B: 199, A: 255} // #ff6ec7 (hot pink)
	colorGreen      = color.NRGBA{R: 46, G: 204, B: 113, A: 255}  // #2ecc71
	colorBlue       = color.NRGBA{R: 88, G: 166, B: 255, A: 255}  // #58a6ff
	colorYellow     = color.NRGBA{R: 243, G: 156, B: 18, A: 255}  // #f39c12
	colorRed        = color.NRGBA{R: 231, G: 76, B: 60, A: 255}   // #e74c3c
	colorPurple     = color.NRGBA{R: 135, G: 95, B: 215, A: 255}  // #8760d7 (merged PR)
	colorGray       = color.NRGBA{R: 139, G: 148, B: 158, A: 255} // #8b949e
	colorDimGray    = color.NRGBA{R: 72, G: 79, B: 88, A: 255}    // #484f58
	colorForeground = color.NRGBA{R: 230, G: 237, B: 243, A: 255} // #e6edf3
)

const (
	defaultTextSize float32 = 14
	minTextSize     float32 = 10
	maxTextSize     float32 = 24
	textSizeStep    float32 = 2
)

// biomeTheme implements fyne.Theme with a dark developer dashboard look.
// textSize can be adjusted at runtime via Ctrl+/Ctrl-.
type biomeTheme struct {
	textSize float32
}

func newBiomeTheme() *biomeTheme {
	return &biomeTheme{textSize: defaultTextSize}
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

func (t *biomeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
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
		return color.NRGBA{R: 0, G: 0, B: 0, A: 80}
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
		return color.NRGBA{R: 0, G: 212, B: 255, A: 60}
	case theme.ColorNameHover:
		return color.NRGBA{R: 48, G: 54, B: 61, A: 128}
	default:
		return theme.DefaultTheme().Color(name, variant)
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
