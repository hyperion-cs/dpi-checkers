package tui

import (
	"fmt"
	"log"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (rm rootModel) View() tea.View {
	var v tea.View
	var s string

	if rm.page == menuPage && rm.quitting {
		s = fmt.Sprintf("See you later! Please star our repository on GitHub:\n%s",
			selectedStyle.Render("https://github.com/hyperion-cs/dpi-checkers/"))
		v.SetContent(mainStyle.Render("\n" + s + "\n\n"))
		return v
	}

	if rm.page != menuPage {
		s += fmt.Sprintf("Tab: %s\n\n", getPageName(rm.page))
	}

	switch rm.page {
	case allPage:
		s += allView()
	case menuPage:
		s += menuView(rm.menuModel)
	case whoamiPage:
		s += whoamiView(rm.whoamiModel)
	case cidrwhitelistPage:
		s += cidrwhitelistView(rm.cidrwhitelistModel)
	case webhostInfraPage, webhostPopularPage:
		s += webhostView(rm.webhostModel)
	case dnsPage:
		s += dnsView(rm.dnsModel)
	case updaterPage:
		s += updaterView(rm.updaterModel)
	}

	if rm.page == updaterPage && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(fmt.Sprintf("m, backspace: skip updater%sq, esc: quit\n%s", dotChar, version.Value))
	}

	if rm.page != menuPage && rm.page != updaterPage && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(fmt.Sprintf("m, backspace: menu%sq, esc: quit\n%s", dotChar, version.Value))
	}

	v.SetContent(mainStyle.Render("\n" + s + "\n\n"))
	return v
}

func menuView(model menuModel) string {
	tpl := "Select what you want to check.\n\n%s\n\n" +
		subtleStyle.Render(fmt.Sprintf("up/down: select%senter: choose%sq, esc: quit\n%s", dotChar, dotChar, version.Value))

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
		checkbox("Popular Web Services "+subtleStyle.Render("like YouTube, Instagram, Discord, Telegram and others"), p == webhostPopularPage, nil),
		checkbox("Infrastructure Providers "+subtleStyle.Render("like Cloudflare, Akamai, Hetzner, DigitalOcean and others"), p == webhostInfraPage, nil),
		checkbox("DNS "+subtleStyle.Render("checks if a censor is spoofing dns responses, hijacking servers, DoH blocking, etc"), p == dnsPage, nil),
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
	emj := countryIsoToFlagEmoji(r.Location)
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s %s", r.Ip, r.Subnet, r.Org, r.Asn, emj, r.Location)
}

func allView() string {
	return "Still in development. Will be ready soon ;)"
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
	var r string
	total := len(model.table.Rows())

	if total > 0 {
		cursor := model.table.Cursor() + 1

		inner := model.table.View() +
			"\n " + model.table.HelpView() +
			subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d", cursor, total))

		r += tblOuterBorderStyle.Render(inner) + "\n\n"
	}
	if model.fetching {
		r += fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}
	r += fmt.Sprintf("count: %d pcs.", total)
	return r
}

func dnsView(model dnsModel) string {
	var r string
	providerTotal := len(model.providerTable.Rows())
	leakTotal := len(model.leakTable.Rows())

	var providerTbl, leakTbl string
	if providerTotal > 0 || !model.fetching {
		cursor := model.providerTable.Cursor() + 1
		tbl := model.providerTable.View() +
			"\n " + dnsTableHelpView() +
			subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d", cursor, providerTotal))

		providerTbl = "> DNS resolves spoofing/blocking:\n" +
			tblOuterBorderStyle.Render(tbl)
	}
	if leakTotal > 0 || !model.fetching {
		leakTbl = "> DNS servers hijacking test. Actually used:\n"
		if leakTotal == 0 {
			leakTbl += tblOuterBorderStyle.Render(" ⚠️ It seems that there is no Internet access  ")
		} else {
			cursor := model.leakTable.Cursor() + 1
			tbl := model.leakTable.View() +
				"\n " + dnsTableHelpView() +
				subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d", cursor, leakTotal))

			leakTbl += tblOuterBorderStyle.Render(tbl)
		}
	}

	if providerTotal > 0 || leakTotal > 0 {
		r += lipgloss.JoinHorizontal(
			lipgloss.Top,
			providerTbl,
			leakTbl,
		)

		if model.fetching {
			r += "\n\n"
		}
	}

	if model.fetching {
		r += fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}

	return r
}

func dnsTableHelpView() string {
	return subtleStyle.Render("↑/↓ up/down; ←/→ left/right table")
}

func updaterView(model updaterModel) string {
	if model.err != nil {
		return fmt.Sprintf("⚠️ error: %v", model.err)
	}

	if model.fetching {
		return fmt.Sprintf("%s %s", model.spinner.View(), model.progress)
	}

	if model.restartRequired {
		return fmt.Sprintf("✅ dpi-ch utility has been updated (=> %s). Please restart.", model.progress)
	}

	return "noop"
}
