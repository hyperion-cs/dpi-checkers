package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
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

type router struct {
	Tab  tab
	Menu *MenuState
}

type MenuState struct {
	pos   int
	items []MenuItem
}

type MenuItem struct {
	Name    string
	Desc    string
	Tab     tab
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
	m := &MenuState{items: []MenuItem{}}

	m.Add("ALL", "warn: run all checks", allTab, allInitMsg{})
	m.Add("Who am I?", "about your internet connection", whoamiTab, whoamiInitMsg{})
	m.Add("Am I under the CIDR whitelist?",
		"checks if a censor restricts tcp/udp connections by ip subnets", cidrwhitelistTab, cidrwhitelistInitMsg{})

	// webhost checker is split into sections (separate tabs), which are defined in config
	for _, x := range webhostCfg.Sections {
		m.Add(x.Name, x.Desc, webhostTab, webhostInitMsg{Targets: x.Targets})
	}

	m.Add("DNS", "checks if a censor is spoofing dns responses, hijacking servers, DoH blocking, etc", dnsTab, dnsInitMsg{})
	return m
}

func (r *router) TabName() string {
	menuCurr := r.Menu.Curr()
	switch r.Tab {
	case menuCurr.Tab:
		return menuCurr.Name
	case updaterTab:
		return "Updater"
	case menuTab:
		return "Menu"
	}

	return "Unknown"
}

func (m *MenuState) View() string {
	var tpl strings.Builder
	tpl.WriteString("Select what you want to check.\n\n")
	for i, x := range m.items {
		tpl.WriteString(checkbox(x.Name+" "+subtleStyle.Render(x.Desc), i == m.pos) + "\n")
	}
	tpl.WriteString("\n\n" + subtleStyle.Render(fmt.Sprintf("up/down: select%senter: choose%sq, esc: quit\n%s", dotChar, dotChar, version.Value)))
	return tpl.String()
}

// Order of calls determines position in menu list.
func (r *MenuState) Add(name, desc string, tab tab, initMsg tea.Msg) {
	r.items = append(r.items, MenuItem{Name: name, Desc: desc, Tab: tab, InitMsg: initMsg})
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
