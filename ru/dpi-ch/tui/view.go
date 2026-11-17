package tui

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (rm rootModel) View() tea.View {
	var v tea.View
	v.SetContent(rm.viewport.View())
	return v
}

func (rm *rootModel) syncViewport() {
	content := rm.render()
	rm.viewport.SetContent(content)
	rm.viewport.SetXOffset(rm.viewport.XOffset())
	rm.viewport.SetYOffset(rm.viewport.YOffset())
}

func (rm rootModel) render() string {
	var s string

	if rm.router.Tab == menuTab && rm.quitting {
		s = fmt.Sprintf(
			"See you later! Please star our repository on GitHub:\n%s\n\n🤝 Join our community\nTelegram group: %s",
			selectedStyle.Render("https://github.com/hyperion-cs/dpi-checkers/"),
			selectedStyle.Render("https://t.me/dpi_checkers"),
		)

		return mainStyle.Render("\n" + s + "\n\n")
	}

	if rm.router.Tab == menuTab && rm.router.inited && rm.router.loading {
		return mainStyle.Render(
			fmt.Sprintf("\n%s initialization\n\n", rm.router.spinner.View()),
		)
	}

	if rm.router.Tab != menuTab {
		s += fmt.Sprintf("Tab: %s\n\n", rm.router.TabName())
	}

	switch rm.router.Tab {
	case allTab:
		s += allView(rm.allModel)
	case menuTab:
		s += rm.router.Menu.View()
	case whoamiTab:
		s += whoamiView(rm.whoamiModel)
	case cidrwhitelistTab:
		s += cidrwhitelistView(rm.cidrwhitelistModel)
	case webhostTab:
		s += webhostView(rm.webhostModel)
	case dnsTab:
		s += dnsView(rm.dnsModel)
	case updaterTab:
		s += updaterView(rm.updaterModel)
	}

	if rm.router.Tab == updaterTab && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(
			fmt.Sprintf(
				"m, backspace: skip updater%swasd: scroll%sq, esc: quit",
				dotChar,
				dotChar,
			),
		)
	}

	if rm.router.Tab != menuTab && rm.router.Tab != updaterTab && !rm.quitting {
		s += "\n\n" + subtleStyle.Render(
			fmt.Sprintf(
				"m, backspace: menu%swasd: scroll%sq, esc: quit",
				dotChar,
				dotChar,
			),
		)
	}

	if rm.router.Tab == allTab && rm.quitting {
		s += "\n"
	}
	cfgPath, err := config.ConfigPath()
	if err == nil {
		s += subtleStyle.Render(
			fmt.Sprintf(
				"\n%s%sconfig path: %s",
				version.Value,
				dotChar,
				cfgPath,
			),
		)
	} else {
		s += subtleStyle.Render("\n" + version.Value)
	}

	return mainStyle.Render("\n" + s + "\n\n")
}

func whoamiView(model whoamiModel) string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}

	if model.err != nil {
		return "error when fetching ;("
	}

	r := model.result
	emj := countryIsoToFlagEmoji(r.Location)
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s %s\nTTLB: %d ms", r.Ip, r.Subnet, r.Org, r.Asn, emj, r.Location, r.Ttlb.Milliseconds())
}

func allView(model allModel) string {
	var lines []string
	frame := model.spinner.View()
	for _, kind := range []checkers.FullCheckTask{checkers.FullCheckTaskWhoami, checkers.FullCheckTaskCidrWhitelist} {
		for _, task := range model.tasks[kind] {
			lines = append(lines, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	var dns []string
	for _, kind := range []checkers.FullCheckTask{checkers.FullCheckTaskDnsLeak, checkers.FullCheckTaskDnsPlain, checkers.FullCheckTaskDnsDoh} {
		for _, task := range model.tasks[kind] {
			dns = append(dns, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	if len(dns) > 0 {
		lines = append(lines, "DNS: { "+strings.Join(dns, dotChar)+" }")
	}
	webhost := model.tasks[checkers.FullCheckTaskWebhost]
	width := 0
	for _, task := range webhost {
		width = max(width, lipgloss.Width(task.name))
	}
	for i, task := range webhost {
		prefix := "         "
		if i == 0 {
			prefix = "Webhost: "
		}
		padding := strings.Repeat(" ", width-lipgloss.Width(task.name)+2)
		lines = append(lines, prefix+task.name+padding+allStatusView(task.state, frame, ""))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	if model.progress.Msg == "" && !model.fetching {
		lines = append(lines, "idle")
	} else {
		state := model.progress.State
		if model.fetching && model.progress.Task != checkers.FullCheckTaskNone {
			state = checkers.FullCheckStateRunning
		}
		lines = append(lines, allStatusView(state, frame, cmp.Or(model.progress.Msg, "fetching")))
	}
	return strings.Join(lines, "\n")
}

func allStatusView(state checkers.FullCheckState, frame, msg string) string {
	switch state {
	case checkers.FullCheckStateOk:
		return okStyle.Render("✅ " + cmp.Or(msg, "done"))
	case checkers.FullCheckStateError:
		return dangerStyle.Render("❌ " + cmp.Or(msg, "error"))
	case checkers.FullCheckStateTimeout:
		return warningStyle.Render("⏰ " + cmp.Or(msg, "timeout"))
	default:
		if msg == "" {
			return frame
		}
		return frame + " " + msg
	}
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

		r += tableOuterBorderStyle().Render(inner) + "\n\n"
	}
	if model.fetching {
		r += fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}
	if model.farmTimeout {
		r += fmt.Sprintf("⏰ farming timeout exceeded (%s)\n", cfg.FarmTimeout.String())
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

		providerTbl = "> DNS resolvers censorship:\n" +
			tableOuterBorderStyle().Render(tbl)
	}
	if leakTotal > 0 || !model.fetching {
		leakTbl = "> Current DNS resolver hijacking test. Actually used:\n"
		if leakTotal == 0 {
			leakTbl += tableOuterBorderStyle().Render(" ⚠️ It seems that there is no Internet access  ")
		} else {
			cursor := model.leakTable.Cursor() + 1
			tbl := model.leakTable.View() +
				"\n " + dnsTableHelpView() +
				subtleStyle.Render(fmt.Sprintf("; cursor: %d/%d", cursor, leakTotal))

			leakTbl += tableOuterBorderStyle().Render(tbl)
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
