package tui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

const dotChar = " • "

var (
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	subtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	mainStyle     = lipgloss.NewStyle().MarginLeft(2)
	spinnerType   = spinner.Line
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

func checkbox(label string, checked bool) string {
	if checked {
		return selectedStyle.Render("[x] " + label)
	}
	return fmt.Sprintf("[ ] %s", label)
}

// https://github.com/charmbracelet/bubbletea/issues/1646
func tableOuterBorderStyle(workaround bool) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(!workaround).
		BorderRight(!workaround).
		BorderTop(true).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("240"))
}

func tableStyle(selectedActive bool) table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)

	s.Selected = s.Selected.
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240")).
		Bold(false)

	if selectedActive {
		s.Selected = s.Selected.
			Background(lipgloss.Color("57"))
	}
	return s
}
