package gui

import (
	"bytes"
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestThemePreservesProportionalAndMonospaceFonts(t *testing.T) {
	themeUnderTest := newBiomeTheme(VariantDark)
	sans := themeUnderTest.Font(fyne.TextStyle{})
	mono := themeUnderTest.Font(fyne.TextStyle{Monospace: true})
	if bytes.Equal(sans.Content(), mono.Content()) {
		t.Fatal("interface and explicit monospace text use the same font")
	}
	for _, style := range []fyne.TextStyle{
		{}, {Bold: true}, {Italic: true}, {Monospace: true}, {Monospace: true, Bold: true},
	} {
		if !bytes.Equal(themeUnderTest.Font(style).Content(), theme.DefaultTheme().Font(style).Content()) {
			t.Fatalf("theme did not preserve requested font style %+v", style)
		}
	}
}

func TestThemePalettesKeepSurfacesAndLabelsReadable(t *testing.T) {
	themeUnderTest := newBiomeTheme(VariantDark)
	t.Cleanup(applyDarkPalette)
	darkCard := colorCardBg
	darkSecondary := colorSecondaryBg
	for _, variant := range []ThemeVariant{VariantLight, VariantDark, VariantLight} {
		themeUnderTest.SetVariant(variant)
		if colorCardBg == colorBackground || colorPanelBg == colorBackground || colorSecondaryBg == colorCardBg {
			t.Fatalf("%s palette lost differentiated surfaces", variant)
		}
		if variant == VariantLight && (colorCardBg == darkCard || colorSecondaryBg == darkSecondary) {
			t.Fatal("new surfaces did not regenerate on theme switch")
		}
		for name, c := range map[string]color.NRGBA{
			"background": colorBackground, "panel": colorPanelBg, "card": colorCardBg,
			"secondary": colorSecondaryBg, "border": colorBorder, "accent": colorSelected,
			"branch": colorBranch, "success": colorGreen, "blue": colorBlue,
			"warning": colorYellow, "error": colorRed, "purple": colorPurple,
			"gray": colorGray, "muted": colorDimGray, "foreground": colorForeground,
			"action": colorActionBg, "onAction": colorOnAction,
		} {
			if c.A != 255 {
				t.Errorf("%s %s color is not an initialized opaque color", variant, name)
			}
		}
		for name, c := range map[string]color.NRGBA{
			"selection": colorSelection, "hover": colorHover, "shadow": colorShadow,
		} {
			if c.A == 0 || c.A == 255 {
				t.Errorf("%s %s overlay is not translucent", variant, name)
			}
		}
		for _, bg := range []color.NRGBA{colorBackground, colorPanelBg, colorCardBg, colorSecondaryBg} {
			for _, fg := range []color.NRGBA{colorForeground, colorGray, colorDimGray} {
				if ratio := paletteContrast(fg, bg); ratio < 4.5 {
					t.Errorf("%s label contrast %.2f is below 4.5 on surface %v", variant, ratio, bg)
				}
			}
		}
		if ratio := paletteContrast(colorOnAction, colorActionBg); ratio < 4.5 {
			t.Errorf("%s primary action contrast %.2f is below 4.5", variant, ratio)
		}
		focusColor := colorActionBg
		focusColor.A = colorSelection.A
		for name, want := range map[fyne.ThemeColorName]color.NRGBA{
			theme.ColorNamePrimary: colorActionBg, theme.ColorNameForegroundOnPrimary: colorOnAction,
			theme.ColorNameFocus: focusColor, theme.ColorNameSelection: colorSelection,
			theme.ColorNameSuccess: colorGreen, theme.ColorNameWarning: colorYellow,
			theme.ColorNameError: colorRed, theme.ColorNameOverlayBackground: colorCardBg,
		} {
			if got := themeUnderTest.Color(name, theme.VariantDark); got != want {
				t.Errorf("%s %s lookup disagrees with canvas palette: %v != %v", variant, name, got, want)
			}
		}
	}
}

func TestThemeTypographyAndGeometryFollowBoundedZoom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	t.Cleanup(applyDarkPalette)
	themeUnderTest := newBiomeTheme(VariantLight)
	app.Settings().SetTheme(themeUnderTest)
	baseline := map[fyne.ThemeSizeName]float32{}
	for _, name := range []fyne.ThemeSizeName{
		theme.SizeNameText, theme.SizeNameCaptionText, theme.SizeNameHeadingText,
		theme.SizeNameSubHeadingText, theme.SizeNamePadding, theme.SizeNameInnerPadding,
		theme.SizeNameInputRadius, theme.SizeNameSelectionRadius,
	} {
		baseline[name] = themeUnderTest.Size(name)
	}
	for i := 0; i < 30; i++ {
		themeUnderTest.ZoomIn()
	}
	if got := themeUnderTest.Size(theme.SizeNameText); got != 24 {
		t.Fatalf("maximum text zoom = %v, want 24", got)
	}
	for i := 0; i < 30; i++ {
		themeUnderTest.ZoomOut()
	}
	if got := themeUnderTest.Size(theme.SizeNameText); got != 10 {
		t.Fatalf("minimum text zoom = %v, want 10", got)
	}
	themeUnderTest.ZoomReset()
	if themeUnderTest.Size(theme.SizeNameText) != 14 {
		t.Fatal("reset did not restore default zoom")
	}
	for _, zoomIn := range []bool{false, true} {
		if zoomIn {
			themeUnderTest.ZoomIn()
		}
		scale := themeUnderTest.Size(theme.SizeNameText) / 14
		for name, base := range baseline {
			if got := themeUnderTest.Size(name); math.Abs(float64(got-base*scale)) > 0.001 {
				t.Errorf("%s does not follow font zoom: %v", name, got)
			}
		}
		body := uiText("Repository", colorForeground, true)
		secondary := secondaryText("Updated just now")
		if body.TextStyle.Monospace || !body.TextStyle.Bold || body.TextSize != theme.TextSize() {
			t.Fatal("uiText did not use proportional styled text at current theme size")
		}
		if secondary.TextStyle.Monospace || secondary.Color != colorDimGray || secondary.TextSize < 11*scale {
			t.Fatal("secondaryText did not retain a readable proportional label")
		}
		if got := scaledSize(radiusCard); math.Abs(float64(got-radiusCard*scale)) > 0.001 {
			t.Fatal("shared canvas geometry did not follow font zoom")
		}
	}
}

func paletteContrast(a, b color.NRGBA) float64 {
	luminance := func(c color.NRGBA) float64 {
		linear := func(v uint8) float64 {
			f := float64(v) / 255
			if f <= 0.04045 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	lighter, darker := luminance(a), luminance(b)
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

func TestThemeFocusedButtonsRetainReadableText(t *testing.T) {
	t.Cleanup(applyDarkPalette)
	for _, variant := range []ThemeVariant{VariantLight, VariantDark} {
		th := newBiomeTheme(variant)
		overlay := th.Color(theme.ColorNameFocus, theme.VariantLight).(color.NRGBA)
		blend := func(fg, bg color.NRGBA) color.NRGBA {
			alpha := float64(fg.A) / 255
			return color.NRGBA{R: uint8(float64(fg.R)*alpha + float64(bg.R)*(1-alpha)), G: uint8(float64(fg.G)*alpha + float64(bg.G)*(1-alpha)), B: uint8(float64(fg.B)*alpha + float64(bg.B)*(1-alpha)), A: 255}
		}
		if ratio := paletteContrast(colorOnAction, blend(overlay, colorActionBg)); ratio < 4.5 {
			t.Errorf("%s focused primary button contrast %.2f", variant, ratio)
		}
		if ratio := paletteContrast(colorForeground, blend(overlay, colorSecondaryBg)); ratio < 4.5 {
			t.Errorf("%s focused secondary button contrast %.2f", variant, ratio)
		}
	}
}
