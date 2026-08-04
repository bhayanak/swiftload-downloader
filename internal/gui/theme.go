package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// accent is the brand colour used for primary buttons, progress bars, and
// selection highlights — a modern indigo/blue similar to Gopeed/AB.
var accent = color.NRGBA{R: 0x4F, G: 0x6B, B: 0xF6, A: 0xFF}

// swiftTheme is a custom Fyne theme that layers a brand accent colour and a
// slightly more generous layout on top of the built-in light/dark palettes.
type swiftTheme struct {
	variant fyne.ThemeVariant
	base    fyne.Theme
}

func newSwiftTheme(variant fyne.ThemeVariant) *swiftTheme {
	return &swiftTheme{variant: variant, base: theme.DefaultTheme()}
}

func (t *swiftTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	v := t.variant
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return accent
	case theme.ColorNameFocus:
		return nnrgba(accent)
	case theme.ColorNameBackground:
		if v == theme.VariantLight {
			return color.NRGBA{R: 0xF6, G: 0xF7, B: 0xFB, A: 0xFF}
		}
		return color.NRGBA{R: 0x1A, G: 0x1C, B: 0x22, A: 0xFF}
	case theme.ColorNameInputBackground:
		if v == theme.VariantLight {
			return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		}
		return color.NRGBA{R: 0x25, G: 0x28, B: 0x30, A: 0xFF}
	}
	return t.base.Color(name, v)
}

func (t *swiftTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return t.base.Icon(name) }

func (t *swiftTheme) Font(style fyne.TextStyle) fyne.Resource { return t.base.Font(style) }

func (t *swiftTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 8
	}
	return t.base.Size(name)
}

// nnrgba returns a semi-opaque focus tint derived from the accent colour.
func nnrgba(c color.NRGBA) color.NRGBA {
	c.A = 0x66
	return c
}
