package tui

import (
	"fmt"
	"log"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type TuiOpt struct {
	Report     *checkers.FullCheckDto
	ReportPath string
}

func newRootModel(opt TuiOpt) rootModel {
	var route *router
	if opt.Report != nil {
		route = &router{Tab: menuTab, Menu: reportMenu(opt.Report, opt.ReportPath), inited: true}
	} else {
		route = NewRouter()
	}

	vp := viewport.New(
		viewport.WithWidth(80),
		viewport.WithHeight(24),
	)
	vp.SoftWrap = false
	vp.FillHeight = false

	model := rootModel{
		router:   route,
		viewport: vp,
		report:   opt.Report,
	}
	model.syncViewport()
	return model
}

func Tui(opt TuiOpt) {
	fmt.Print(ansi.EraseEntireScreen, ansi.CursorHomePosition)

	p := tea.NewProgram(newRootModel(opt))
	if _, err := p.Run(); err != nil {
		log.Fatalf("could not start tui: %v", err)
	}
}
