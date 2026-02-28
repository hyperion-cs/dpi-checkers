package tui

import (
	"dpich/checkers"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

func (rm rootModel) View() string {
	var s string

	if rm.page == menuPage && rm.quitting {
		s = fmt.Sprintf("See you later! Please star our repository on GitHub:\n%s",
			selectedStyle.Render("https://github.com/hyperion-cs/dpi-checkers/"))
		return mainStyle.Render("\n" + s + "\n\n")
	}

	if rm.page != menuPage {
		s += fmt.Sprintf("Tab: %s\n\n", getPageName(rm.page))
	}

	switch rm.page {
	case menuPage:
		s += menuView(rm.menuModel)
	case whoamiPage:
		s += whoamiView(rm.whoamiModel)
	case cidrwhitelistPage:
		s += cidrwhitelistView(rm.cidrwhitelistModel)
	case tcp1620Page:
		s += tcp1620View(rm.tcp1620Model)
	}

	if rm.page != menuPage && !rm.quitting {
		s += "\n\n" + subtleStyle.Render("m, backspace: menu"+dotChar+"q, esc: quit")
	}

	return mainStyle.Render("\n" + s + "\n\n")
}

func menuView(model menuModel) string {
	tpl := "Select what you want to check.\n\n%s\n\n" +
		subtleStyle.Render("up/down: select"+dotChar+"enter: choose"+dotChar+"q, esc: quit")

	p := menuOptions[model.optionIdx]
	var as *lipgloss.Style
	at := "ALL"
	if p == allPage {
		as = &warningStyle
		at = "ALL (warn: run all checks)"
	}

	choices := fmt.Sprintf(
		"%s\n%s\n%s\n%s\n%s\n%s",
		checkbox(at, p == allPage, as),
		checkbox("Who am I? "+subtleStyle.Render("about your internet connection"), p == whoamiPage, nil),
		checkbox("Am I under the CIDR whitelist? "+subtleStyle.Render("checks if a censor restricts tcp/udp connections by ip subnets"), p == cidrwhitelistPage, nil),
		checkbox("DNS "+subtleStyle.Render("checks availability and hijacking of some dns servers"), p == dnsserverPage, nil),
		checkbox("Popular Web Services "+subtleStyle.Render("like YouTube, Instagram, Discord, Telegram and others"), p == endpointPage, nil),
		checkbox("Infrastructure Providers "+subtleStyle.Render("like Cloudflare, Akamai, Hetzner, DigitalOcean and others"), p == tcp1620Page, nil),
	)

	return fmt.Sprintf(tpl, choices)
}

func whoamiView(model whoamiModel) string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}

	if model.err != nil {
		return "error when fetching"
	}

	r := model.result
	return fmt.Sprintf("IP: %s\nSubnet: %s\nHolder: %s (AS%s)\nLocation: %s", r.Ip, r.Subnet, r.Holder, r.Asn, r.Location)
}

func cidrwhitelistView(model cidrwhitelistModel) string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}

	if model.err == nil {
		return okStyle.Render("You're NOT under one ;)")
	}

	if model.err == checkers.ErrCidrWhitelistDetected {
		return dangerStyle.Render("You're UNDER one. :(")
	}

	if model.err == checkers.ErrCidrWhitelistNoInetAccess {
		return warningStyle.Render("It seems that there is no Internet access (even to resources from the whitelist).")
	}

	return warningStyle.Render("Internal error ;(")
}

func tcp1620View(model tcp1620Model) string {
	// type EndpointAttrs struct {
	// 	Id      string
	// 	Url     string
	// 	Host    string
	// 	IpAddr  string
	// 	Subnet  string
	// 	Asn     string
	// 	Holder  string
	// 	Country string
	// 	City    string
	// }

	columns := []table.Column{
		{Title: "Id", Width: 11},
		{Title: "Subnet", Width: 18},
		{Title: "Holder", Width: 20},
		{Title: "ASN", Width: 12},
		{Title: "Country", Width: 7},
		{Title: "Status", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(model.rows),
		table.WithFocused(true),
		//table.WithHeight(min(len(model.rows), 15)),
		table.WithHeight(len(model.rows)),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	var r string
	r += tblOuterBorderStyle.Render(t.View()) + "\n\n"
	r += fmt.Sprintf("Fetching: %t\nCount: %d\n", model.fetching, len(model.rows))
	return r
}
