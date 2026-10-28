package tui

import "github.com/charmbracelet/lipgloss"

const (
	colorText   = "#d7dee6"
	colorDim    = "#8b98a5"
	colorFaint  = "#5c6b78"
	colorAccent = "#8ec8c3"
	colorOK     = "#7dcea0"
	colorAmber  = "#e0b15a"
	colorRed    = "#e07a7a"
	colorBorder = "#3c4c59"
	colorTrack  = "#2a3844"
)

func styleText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
}

func styleDim() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorDim))
}

func styleFaint() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorFaint))
}

func styleAccent() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true)
}

func styleValue(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}
