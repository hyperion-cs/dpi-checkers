package tui

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func Tui() {
	p := tea.NewProgram(rootModel{})
	if _, err := p.Run(); err != nil {
		log.Fatalf("could not start tui: %v", err)
	}
}
