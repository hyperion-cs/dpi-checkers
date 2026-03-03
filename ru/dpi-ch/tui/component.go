package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
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
