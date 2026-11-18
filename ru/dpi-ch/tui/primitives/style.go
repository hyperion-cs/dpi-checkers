package primitives

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

var (
	DangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	OkStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	WarningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	SelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	InfoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	SubtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	MainStyle     = lipgloss.NewStyle().MarginLeft(2)
)

func NewSpinner() spinner.Model {
	model := spinner.New()
	model.Spinner = spinner.Line
	model.Style = SelectedStyle
	return model
}
