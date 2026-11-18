package screens

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

type Screen interface {
	Update(tea.Msg) tea.Cmd
	View() string
}

type OpenFunc func(context.Context) (Screen, tea.Cmd)

const (
	AllTitle           = "ALL"
	whoamiTitle        = "Who am I?"
	cidrwhitelistTitle = "Am I under the CIDR whitelist?"
	dnsTitle           = "DNS"
)

type Menu struct {
	pos     int
	items   []menuItem
	header  string
	spinner spinner.Model
	loading bool
}

type menuItem struct {
	name, desc string
	featured   bool
	open       OpenFunc
}

func NewMenu() *Menu {
	m := &Menu{header: "Select what you want to check.", spinner: primitives.NewSpinner(), loading: !inetlookup.Inited()}
	m.items = append(m.items,
		menuItem{AllTitle, "long exec warn: run all checks specified in the config", true, OpenAll},
		menuItem{whoamiTitle, "about your internet connection", true, openWhoami},
		menuItem{cidrwhitelistTitle, "checks if a censor restricts tcp/udp connections by ip subnets", true, openCidrWhitelist},
	)
	for _, section := range config.Get().Checkers.Webhost.Sections {
		m.items = append(m.items, menuItem{section.Name, section.Desc, section.Default, func(ctx context.Context) (Screen, tea.Cmd) {
			return openWebhost(ctx, section.Targets)
		}})
	}
	m.items = append(m.items, menuItem{dnsTitle, "checks if a censor is spoofing dns responses, hijacking servers, DoH blocking, etc", true, openDns})
	return m
}

func (m *Menu) Tick() tea.Cmd {
	if m.loading {
		return m.spinner.Tick
	}
	return nil
}

func (m *Menu) Update(msg tea.Msg) tea.Cmd {
	if m.loading && inetlookup.Inited() {
		m.loading = false
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if len(m.items) == 0 {
			return nil
		}
		switch primitives.NormKey(msg.String()) {
		case "up":
			m.pos = (m.pos - 1 + len(m.items)) % len(m.items)
		case "down":
			m.pos = (m.pos + 1) % len(m.items)
		}
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func (m *Menu) Selection() (string, OpenFunc) {
	if len(m.items) == 0 {
		return "", nil
	}
	item := m.items[m.pos]
	return primitives.TerminalText(item.name), item.open
}

func (m *Menu) Loading() bool {
	return m.loading
}

func (m *Menu) View() string {
	if m.loading {
		return m.spinner.View() + " initialization"
	}

	var out strings.Builder
	out.WriteString(m.header + "\n\n")
	for i, item := range m.items {
		name := primitives.TerminalText(item.name)
		if !item.featured && i != m.pos {
			name = primitives.InfoStyle.Render(name)
		}
		label := name + " " + primitives.SubtleStyle.Render(item.desc)
		if i == m.pos {
			out.WriteString(primitives.SelectedStyle.Render("[x] " + label))
		} else {
			out.WriteString("[ ] " + label)
		}
		out.WriteString("\n")
	}
	out.WriteString("\n\n")
	out.WriteString(primitives.SubtleStyle.Render(fmt.Sprintf("↑/↓: select%senter: choose%swasd: scroll%sq, esc: quit", primitives.DotChar, primitives.DotChar, primitives.DotChar)))
	return out.String()
}
