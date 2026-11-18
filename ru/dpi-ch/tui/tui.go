package tui

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/screens"
)

const (
	horizontalScrollStep = 8
	verticalScrollStep   = 3
)

type TuiOpt struct {
	Report     *fullcheck.ReportDto
	ReportPath string
}

type rootModel struct {
	menu               *screens.Menu
	screen             screens.Screen
	title              string
	session            *session
	viewport           viewport.Model
	configPath         string
	dataPath           string
	configCreateFailed bool
	showDataPath       bool
	lookupErr          error
	offline            bool
	quitting           bool
}

type startupMsg struct{}
type lookupReadyMsg struct {
	all bool
	err error
}

func newRootModel(opt TuiOpt) rootModel {
	var menu *screens.Menu
	if opt.Report != nil {
		menu = screens.NewReportMenu(opt.Report, opt.ReportPath)
	} else {
		menu = screens.NewMenu()
	}
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.SoftWrap = false
	vp.FillHeight = false

	cfgPath, _ := config.ConfigPath()
	model := rootModel{
		menu:               menu,
		viewport:           vp,
		configPath:         cfgPath,
		dataPath:           config.DataDir(),
		configCreateFailed: config.ConfigCreateError() != nil,
		offline:            opt.Report != nil,
	}
	model.syncViewport()
	return model
}

func Tui(opt TuiOpt) {
	fmt.Print(ansi.EraseEntireScreen, ansi.CursorHomePosition)

	p := tea.NewProgram(newRootModel(opt))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "could not start tui: %v\n", err)
		os.Exit(1)
	}
}

func (rm rootModel) Init() tea.Cmd {
	if rm.offline {
		return nil
	}
	return func() tea.Msg { return startupMsg{} }
}

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if rm.quitting {
		return rm, nil
	}
	if scoped, ok := msg.(sessionMsg); ok {
		if scoped.session != rm.session {
			return rm, nil
		}
		msg = scoped.msg
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case startupMsg:
		if rm.offline || rm.screen != nil {
			return rm, nil
		}
		cmd = rm.open(screens.UpdaterTitle, screens.OpenUpdater)
		if rm.screen == nil {
			cmd = rm.loadLookup(config.Get().All.Flag)
		}
	case lookupReadyMsg:
		if rm.offline {
			return rm, nil
		}
		if msg.err != nil {
			rm.cancel()
			rm.lookupErr = msg.err
		} else if msg.all {
			cmd = rm.open(screens.AllTitle, screens.OpenAll)
		}
	case screens.UpdaterDoneMsg:
		if _, updating := rm.screen.(*screens.Updater); updating {
			rm.leave()
			cmd = rm.loadLookup(config.Get().All.Flag)
		}
	case screens.AllExitMsg:
		if _, running := rm.screen.(*screens.All); running {
			rm.quit()
			cmd = tea.Quit
		}
	case tea.WindowSizeMsg:
		rm.viewport.SetWidth(max(1, msg.Width))
		rm.viewport.SetHeight(max(1, msg.Height))
	case tea.KeyPressMsg:
		switch msg.String() {
		case "a":
			rm.viewport.ScrollLeft(horizontalScrollStep)
		case "d":
			rm.viewport.ScrollRight(horizontalScrollStep)
		case "w":
			rm.viewport.ScrollUp(verticalScrollStep)
		case "s":
			rm.viewport.ScrollDown(verticalScrollStep)
		case "p", "з":
			rm.showDataPath = !rm.showDataPath
		case "q", "й", "esc", "ctrl+c", "ctrl+с":
			rm.quit()
			cmd = tea.Quit
		case "m", "ь", "backspace":
			if rm.screen != nil {
				_, updating := rm.screen.(*screens.Updater)
				rm.leave()
				if updating {
					cmd = rm.loadLookup(false)
				} else {
					cmd = rm.menu.Tick()
				}
			} else {
				rm.resetScroll()
			}
		case "enter":
			if rm.screen == nil && rm.session == nil && rm.lookupErr == nil && !rm.menu.Loading() {
				title, open := rm.menu.Selection()
				if open != nil {
					cmd = rm.open(title, open)
				}
			}
		}
	}

	if !rm.quitting {
		if rm.screen != nil {
			cmd = tea.Batch(cmd, rm.session.bind(rm.screen.Update(msg)))
		} else if rm.lookupErr == nil {
			cmd = tea.Batch(cmd, rm.menu.Update(msg))
		}
	}
	rm.syncViewport()
	return rm, cmd
}

func (rm *rootModel) open(title string, open screens.OpenFunc) tea.Cmd {
	run := newSession()
	screen, cmd := open(run.ctx)
	if screen == nil {
		run.cancel()
		return nil
	}
	rm.leave()
	rm.screen, rm.title, rm.session = screen, title, run
	return run.bind(cmd)
}

func (rm *rootModel) loadLookup(all bool) tea.Cmd {
	cmd := func() tea.Msg {
		return lookupReadyMsg{all: all, err: inetlookup.InitDefault()}
	}
	if all {
		rm.session = newSession()
		cmd = rm.session.bind(cmd)
	}
	return tea.Batch(cmd, rm.menu.Tick())
}

func (rm *rootModel) leave() {
	rm.cancel()
	rm.screen, rm.title = nil, ""
	rm.resetScroll()
}

func (rm *rootModel) resetScroll() {
	rm.viewport.SetXOffset(0)
	rm.viewport.SetYOffset(0)
}

func (rm *rootModel) quit() {
	rm.cancel()
	rm.quitting = true
}

func (rm *rootModel) cancel() {
	if rm.session != nil {
		rm.session.cancel()
		rm.session = nil
	}
}

func (rm rootModel) View() tea.View {
	return tea.NewView(rm.viewport.View())
}

func (rm *rootModel) syncViewport() {
	rm.viewport.SetContent(rm.render())
	rm.viewport.SetXOffset(rm.viewport.XOffset())
	rm.viewport.SetYOffset(rm.viewport.YOffset())
}

func (rm rootModel) renderHeader() string {
	const separator = " · "
	header := version.Value + separator + "p: config/data"
	path, style := rm.configPath, primitives.SubtleStyle
	if rm.showDataPath {
		path = rm.dataPath
	} else if rm.configCreateFailed {
		style = primitives.DangerStyle
	}
	if path == "" {
		return primitives.SubtleStyle.Render(header)
	}
	return primitives.SubtleStyle.Render(header+separator+"path: ") + style.Render(primitives.TerminalText(path))
}

func (rm rootModel) render() string {
	var text string
	switch {
	case rm.screen == nil && rm.quitting:
		text = fmt.Sprintf(
			"See you later! Please star our repository on GitHub:\n%s\n\n🤝 Join our community\nTelegram group: %s",
			primitives.SelectedStyle.Render("https://github.com/hyperion-cs/dpi-checkers/"),
			primitives.SelectedStyle.Render("https://t.me/dpi_checkers"),
		)
		return primitives.MainStyle.Render("\n" + text + "\n\n")
	case rm.screen == nil && rm.lookupErr != nil:
		text = primitives.DangerStyle.Render("GeoLite initialization failed: " + primitives.TerminalText(rm.lookupErr.Error()))
		text += "\n\n" + primitives.SubtleStyle.Render("q, esc: quit")
	case rm.screen == nil:
		text = rm.menu.View()
	default:
		text = "Tab: " + rm.title + "\n\n" + rm.screen.View()
		if !rm.quitting {
			back := "menu"
			if _, updating := rm.screen.(*screens.Updater); updating {
				back = "skip updater"
			}
			text += "\n\n" + primitives.SubtleStyle.Render(
				"m, backspace: "+back+primitives.DotChar+"wasd: scroll"+primitives.DotChar+"q, esc: quit",
			)
		} else if _, all := rm.screen.(*screens.All); all {
			text += "\n"
		}
	}

	return primitives.MainStyle.Render("\n" + rm.renderHeader() + "\n" + text + "\n\n")
}
