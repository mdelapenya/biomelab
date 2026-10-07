package gui

import (
	"image/color"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

// uiText creates proportional interface text at the current theme text size.
// Like other canvas primitives, rebuild it after changing theme or zoom.
// Keep monoText for technical values that benefit from fixed-width alignment.
func uiText(text string, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextStyle.Bold = bold
	t.TextSize = theme.TextSize()
	return t
}

// secondaryText creates readable supporting text, scaled with font zoom.
func secondaryText(text string) *canvas.Text {
	t := uiText(text, colorDimGray, false)
	t.TextSize = scaledSize(textSecondarySize)
	return t
}
