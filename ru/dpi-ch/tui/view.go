package tui

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"

	"charm.land/bubbles/v2/table"
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
		return resultErrorText(model.err, "error when fetching ;(")
	}

	r := model.result
	location := terminalText(r.Location)
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s %s\nTTLB: %d ms",
		terminalText(r.Ip), terminalText(r.Subnet), terminalText(r.Org), terminalText(r.Asn),
		countryIsoToFlagEmoji(location), location, r.Ttlb.Milliseconds())
}

func allView(model allModel) string {
	var lines []string
	frame := model.spinner.View()
	for _, kind := range []fullcheck.Task{fullcheck.TaskWhoami, fullcheck.TaskCidrWhitelist} {
		for _, task := range model.tasks[kind] {
			lines = append(lines, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	var dns []string
	for _, kind := range []fullcheck.Task{fullcheck.TaskDnsLeak, fullcheck.TaskDnsPlain, fullcheck.TaskDnsDoh} {
		for _, task := range model.tasks[kind] {
			dns = append(dns, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	if len(dns) > 0 {
		lines = append(lines, "DNS: { "+strings.Join(dns, dotChar)+" }")
	}
	webhost := model.tasks[fullcheck.TaskWebhost]
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
		if model.fetching && model.progress.Task != fullcheck.TaskNone {
			state = fullcheck.StateRunning
		}
		lines = append(lines, allStatusView(state, frame, cmp.Or(model.progress.Msg, "fetching")))
	}
	return strings.Join(lines, "\n")
}

func allStatusView(state fullcheck.State, frame, msg string) string {
	switch state {
	case fullcheck.StateOk:
		return okStyle.Render("✅ " + cmp.Or(msg, "done"))
	case fullcheck.StateError:
		return dangerStyle.Render("❌ " + cmp.Or(msg, "error"))
	case fullcheck.StateTimeout:
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

	if model.err == cidrwhitelist.ErrDetected {
		return dangerStyle.Render("You're UNDER one :(")
	}

	if model.err == cidrwhitelist.ErrNoInetAccess {
		return warningStyle.Render("It seems that there is no Internet access (even to resources from the whitelist).")
	}

	return warningStyle.Render(resultErrorText(model.err, "Internal error ;("))
}

func resultErrorText(err error, fallback string) string {
	if err == fullcheck.ErrUnknownReason {
		return unknownResultText
	}
	return fallback
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
		r += "⏰ farming timeout exceeded"
		if model.farmLimit > 0 {
			r += " (" + model.farmLimit.String() + ")"
		}
		r += "\n"
	}
	r += fmt.Sprintf("count: %d pcs.", total)
	return r
}

func dnsView(model dnsModel) string {
	var r string
	providerTotal := len(model.providerTable.Rows())
	leakTotal := len(model.leakTable.Rows())

	var providerTbl, leakTbl string
	if model.showProviders && (providerTotal > 0 || !model.fetching) {
		providerTbl = "> DNS resolvers censorship:\n" + dnsTableView(model.providerTable)
	}
	if model.showLeak && (leakTotal > 0 || !model.fetching) {
		leakTbl = "> Current DNS resolver hijacking test. Actually used:\n"
		if leakTotal == 0 {
			text := "⚠️ No DNS resolver results"
			if model.leakErr != nil {
				text = resultErrorText(model.leakErr, "⚠️ "+model.leakErr.Error())
			}
			leakTbl += tableOuterBorderStyle().Render(" " + text + " ")
		} else {
			leakTbl += dnsTableView(model.leakTable)
			if model.leakErr == fullcheck.ErrUnknownReason {
				leakTbl += "\n" + warningStyle.Render(unknownResultText)
			}
		}
	}

	if providerTbl != "" || leakTbl != "" {
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

func dnsTableView(tbl table.Model) string {
	total := len(tbl.Rows())
	cursor := min(tbl.Cursor()+1, total)
	help := fmt.Sprintf("↑/↓ up/down; ←/→ left/right table; cursor: %d/%d", cursor, total)
	return tableOuterBorderStyle().Render(tbl.View() + "\n " + subtleStyle.Render(help))
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
