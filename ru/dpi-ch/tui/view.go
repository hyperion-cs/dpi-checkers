package tui

import (
	"dpich/checkers"
	"fmt"
	"log"

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
	case webhostInfraPage, webhostPopularPage:
		s += webhostView(rm.webhostModel)
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
		"%s\n%s\n%s\n%s\n%s",
		checkbox(at, p == allPage, as),
		checkbox("Who am I? "+subtleStyle.Render("about your internet connection"), p == whoamiPage, nil),
		checkbox("Am I under the CIDR whitelist? "+subtleStyle.Render("checks if a censor restricts tcp/udp connections by ip subnets"), p == cidrwhitelistPage, nil),
		checkbox("Popular Web Services "+subtleStyle.Render("like YouTube, Instagram, Discord, Telegram and others"), p == webhostPopularPage, nil),
		checkbox("Infrastructure Providers "+subtleStyle.Render("like Cloudflare, Akamai, Hetzner, DigitalOcean and others"), p == webhostInfraPage, nil),
	)

	return fmt.Sprintf(tpl, choices)
}

func whoamiView(model whoamiModel) string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}

	if model.err != nil {
		log.Println(model.err)
		return "error when fetching ;("
	}

	r := model.result
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s", r.Ip, r.Subnet, r.Org, r.Asn, r.Location)
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

func webhostView(model webhostModel) string {
	columns := []table.Column{
		{Title: "Group", Width: getMaxLen(model.rows, 0, 5)},
		{Title: "AS", Width: getMaxLen(model.rows, 1, 7)},
		{Title: "Location", Width: 8},
		{Title: "IP", Width: getMaxLen(model.rows, 3, 2)},
		{Title: "Prefix", Width: getMaxLen(model.rows, 4, 6)},
		{Title: "Alive", Width: 6},
		{Title: "Tcp 16-20", Width: 11},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(model.rows),
		table.WithFocused(true),
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
	if len(model.rows) > 0 {
		r += tblOuterBorderStyle.Render(t.View()) + "\n\n"
	}
	r += fmt.Sprintf("Status: %s\nCount: %d\n", model.progress, len(model.rows))
	return r
}

func getMaxLen(rows []table.Row, pos, min int) int {
	max := min
	for _, v := range rows {
		if len(v[pos]) > max {
			max = len(v[pos])
		}
	}
	return max
}
