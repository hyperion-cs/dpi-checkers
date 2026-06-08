package tui

import (
	"fmt"
	"log"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (rm rootModel) View() tea.View {
	var v tea.View
	var s string

	if rm.router.Tab == menuTab && rm.quitting {
		s = fmt.Sprintf("See you later! Please star our repository on GitHub:\n%s",
			selectedStyle.Render("https://github.com/hyperion-cs/dpi-checkers/"))
		v.SetContent(mainStyle.Render("\n" + s + "\n\n"))
		return v
	}

	if rm.router.Tab != menuTab {
		s += fmt.Sprintf("Tab: %s\n\n", rm.router.TabName())
	}

	switch rm.router.Tab {
	case allTab:
		s += allView()
	case menuTab:
		s += rm.router.Menu.View()
	case whoamiTab:
		s += whoamiView(rm.whoamiModel)
	case cidrwhitelistTab:
		s += cidrwhitelistView(rm.cidrwhitelistModel)
	case webhostTab:
		s += webhostView(rm.webhostModel) // тут че то надо выдумать
	case dnsTab:
		s += dnsView(rm.dnsModel)
	case updaterTab:
		s += updaterView(rm.updaterModel)
	}

	if rm.router.Tab == updaterTab && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(fmt.Sprintf("m, backspace: skip updater%sq, esc: quit\n%s", dotChar, version.Value))
	}

	if rm.router.Tab != menuTab && rm.router.Tab != updaterTab && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(fmt.Sprintf("m, backspace: menu%sq, esc: quit\n%s", dotChar, version.Value))
	}

	v.SetContent(mainStyle.Render("\n" + s + "\n\n"))
	return v
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
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s %s\nTTLB: %d ms", r.Ip, r.Subnet, r.Org, r.Asn, emj, r.Location, r.Ttlb.Milliseconds())
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
		return dangerStyle.Render("You're UNDER one :(")
	}

	if model.err == checkers.ErrCidrWhitelistNoInetAccess {
		return warningStyle.Render("It seems that there is no Internet access (even to resources from the whitelist).")
	}

	return warningStyle.Render("Internal error ;(")
}

func webhostView(model webhostModel) string {
	var r string
	cfg := config.Get().Checkers.Webhost
	total := len(model.table.Rows())

	if total > 0 {
		cursor := model.table.Cursor() + 1
		over := ""
		if total > cfg.TableMaxVisibleRows {
			over = " 👀"
		}

		inner := model.table.View() +
			"\n " + model.table.HelpView() +
			subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d%s", cursor, total, over))

		r += tableOuterBorderStyle(true).Render(inner) + "\n\n"
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
			tableOuterBorderStyle(false).Render(tbl)
	}
	if leakTotal > 0 || !model.fetching {
		leakTbl = "> DNS servers hijacking test. Actually used:\n"
		if leakTotal == 0 {
			leakTbl += tableOuterBorderStyle(false).Render(" ⚠️ It seems that there is no Internet access  ")
		} else {
			cursor := model.leakTable.Cursor() + 1
			tbl := model.leakTable.View() +
				"\n " + dnsTableHelpView() +
				subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d", cursor, leakTotal))

			leakTbl += tableOuterBorderStyle(false).Render(tbl)
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
