package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type subtleTheme struct {
	fyne.Theme
}

func newSubtleTheme() fyne.Theme {
	return &subtleTheme{Theme: theme.LightTheme()}
}

func (t *subtleTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 160, G: 160, B: 160, A: 140}
	default:
		return t.Theme.Color(name, variant)
	}
}

func (t *subtleTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameScrollBar {
		return 7
	}
	return t.Theme.Size(name)
}
