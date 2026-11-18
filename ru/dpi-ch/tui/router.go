package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type tab int

const (
	menuTab tab = iota
	allTab
	whoamiTab
	cidrwhitelistTab
	webhostTab
	dnsTab
	updaterTab
)

const (
	whoamiTitle        = "Who am I?"
	cidrwhitelistTitle = "Am I under the CIDR whitelist?"
	dnsTitle           = "DNS"
)

type router struct {
	Tab  tab
	Menu *MenuState

	inited  bool
	loading bool
	spinner spinner.Model
}

type MenuState struct {
	pos    int
	items  []MenuItem
	header string
}

type MenuItem struct {
	Name    string
	Desc    string
	Tab     tab
	Default bool
	InitMsg tea.Msg
}

func NewRouter() *router {
	return &router{
		Tab:  menuTab,
		Menu: NewMenu(),
	}
}

func NewMenu() *MenuState {
	webhostCfg := config.Get().Checkers.Webhost
	m := &MenuState{header: "Select what you want to check."}

	m.Add("ALL", "long exec warn: run all checks specified in the config", allTab, true, allInitMsg{})
	m.Add(whoamiTitle, "about your internet connection", whoamiTab, true, whoamiInitMsg{})
	m.Add(cidrwhitelistTitle,
		"checks if a censor restricts tcp/udp connections by ip subnets", cidrwhitelistTab, true, cidrwhitelistInitMsg{})

	// webhost checker is split into sections (separate tabs), which are defined in config
	for _, x := range webhostCfg.Sections {
		m.Add(x.Name, x.Desc, webhostTab, x.Default, webhostInitMsg{Targets: x.Targets})
	}

	m.Add(dnsTitle, "checks if a censor is spoofing dns responses, hijacking servers, DoH blocking, etc", dnsTab, true, dnsInitMsg{})
	return m
}

func (r *router) TabName() string {
	menuCurr := r.Menu.Curr()
	switch r.Tab {
	case menuCurr.Tab:
		return terminalText(menuCurr.Name)
	case updaterTab:
		return "Updater"
	case menuTab:
		return "Menu"
	}

	return "Unknown"
}

func (m *MenuState) View() string {
	var tpl strings.Builder
	tpl.WriteString(m.header + "\n\n")
	for i, x := range m.items {
		name := terminalText(x.Name)
		if !x.Default && i != m.pos {
			name = infoStyle.Render(name)
		}
		tpl.WriteString(checkbox(name+" "+subtleStyle.Render(x.Desc), i == m.pos))
		tpl.WriteString("\n")
	}
	tpl.WriteString("\n\n")
	tpl.WriteString(subtleStyle.Render(fmt.Sprintf("↑/↓: select%senter: choose%swasd: scroll%sq, esc: quit", dotChar, dotChar, dotChar)))
	return tpl.String()
}

// Order of calls determines position in menu list.
func (r *MenuState) Add(name, desc string, tab tab, def bool, initMsg tea.Msg) {
	r.items = append(r.items, MenuItem{Name: name, Desc: desc, Tab: tab, Default: def, InitMsg: initMsg})
}

func (m *MenuState) Curr() MenuItem {
	return m.items[m.pos]
}

func (m *MenuState) Prev() {
	m.pos = (m.pos - 1 + len(m.items)) % len(m.items)
}

func (m *MenuState) Next() {
	m.pos = (m.pos + 1) % len(m.items)
}
