package tui

import (
	"log"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func Tui() {
	router := NewRouter()

	vp := viewport.New(
		viewport.WithWidth(80),
		viewport.WithHeight(24),
	)
	vp.SoftWrap = false
	vp.FillHeight = false

	model := rootModel{
		router:   router,
		viewport: vp,
	}
	model.syncViewport()

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		log.Fatalf("could not start tui: %v", err)
	}
}
