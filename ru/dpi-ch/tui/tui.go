package tui

import (
	"log"

	tea "charm.land/bubbletea/v2"
)

func Tui() {
	router := NewRouter()
	p := tea.NewProgram(rootModel{router: router})
	if _, err := p.Run(); err != nil {
		log.Fatalf("could not start tui: %v", err)
	}
}
