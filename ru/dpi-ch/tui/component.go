package tui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

const dotChar = " • "

var (
	dangerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	warningStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	selectedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	subtleStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	mainStyle           = lipgloss.NewStyle().MarginLeft(2)
	spinnerType         = spinner.Line
	spinnerStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	tblOuterBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240"))
)

func checkbox(label string, checked bool, style *lipgloss.Style) string {
	s := &selectedStyle
	if style != nil {
		s = style
	}

	if checked {
		return s.Render("[x] " + label)
	}
	return fmt.Sprintf("[ ] %s", label)
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
