package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// MatrixTheme è un tema personalizzato con log verdi stile Matrix
type MatrixTheme struct{}

var _ fyne.Theme = (*MatrixTheme)(nil)

func (m MatrixTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameDisabled:
		// Testo disabilitato (log area) -> verde Matrix
		return color.RGBA{0, 255, 65, 255} // Verde fosforescente #00FF41
	case theme.ColorNameDisabledButton:
		return color.RGBA{0, 180, 45, 255} // Verde più scuro per contrasto
	case theme.ColorNameInputBackground:
		return color.RGBA{10, 10, 10, 255} // Nero profondo per log
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (m MatrixTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m MatrixTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m MatrixTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
