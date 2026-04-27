package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

var ErrPending = errors.New("err: pending")

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Only root and updater processing here
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		k := msg.String()
		// this and other tea.ClearScreen; tmp workaround of https://github.com/charmbracelet/bubbletea/issues/1646
		cmds = append(cmds, tea.ClearScreen)

		if k == "q" || k == "й" || k == "esc" || k == "ctrl+c" {
			rm.quitting = true
			return rm, tea.Quit
		}

		if k == "m" || k == "ь" || k == "backspace" {
			if rm.page == updaterPage {
				cmds = append(cmds, func() tea.Msg { return updaterDoneMsg{} })
			}

			rm.page = menuPage
			cmds = append(cmds, func() tea.Msg { return returnedToMenuMsg{} })
		}
	case rootMsg:
		rm.page = msg.page
	case updaterInitMsg:
		rm.page = updaterPage
	case updaterDoneMsg:
		rm.page = menuPage
	}

	if rm.page == menuPage {
		rm.menuModel, cmd = menuUpdate(rm.menuModel, msg)
		cmds = append(cmds, cmd)
	}

	rm.whoamiModel, cmd = whoamiUpdate(rm.whoamiModel, msg)
	cmds = append(cmds, cmd)

	rm.cidrwhitelistModel, cmd = cidrwhitelistUpdate(rm.cidrwhitelistModel, msg)
	cmds = append(cmds, cmd)

	rm.webhostModel, cmd = webhostUpdate(rm.webhostModel, msg)
	cmds = append(cmds, cmd)

	rm.updaterModel, cmd = updaterUpdate(rm.updaterModel, msg)
	cmds = append(cmds, cmd)

	rm.dnsModel, cmd = dnsUpdate(rm.dnsModel, msg)
	cmds = append(cmds, cmd)

	return rm, tea.Batch(cmds...)
}

func menuUpdate(model menuModel, msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			model.optionIdx = (model.optionIdx - 1 + len(menuOptions)) % len(menuOptions)
		case "down":
			model.optionIdx = (model.optionIdx + 1) % len(menuOptions)
		case "enter":
			var initMsg tea.Msg
			p := menuOptions[model.optionIdx]
			switch p {
			case whoamiPage:
				initMsg = whoamiInitMsg{}
			case cidrwhitelistPage:
				initMsg = cidrwhitelistInitMsg{}
			case webhostPopularPage:
				initMsg = webhostInitMsg{Mode: checkers.WebHostModePopular}
			case webhostInfraPage:
				initMsg = webhostInitMsg{Mode: checkers.WebHostModeInfra}
			case dnsPage:
				initMsg = dnsInitMsg{}
			}

			return model, tea.Batch(
				func() tea.Msg { return rootMsg{page: p} },
				func() tea.Msg { return initMsg },
			)
		}
	}

	return model, nil
}

func whoamiUpdate(model whoamiModel, msg tea.Msg) (whoamiModel, tea.Cmd) {
	switch msg := msg.(type) {
	case whoamiInitMsg:
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = whoamiModel{spinner: s, fetching: true}
		return model, tea.Batch(model.spinner.Tick, whoamiFetchCmd)

	case whoamiResultMsg:
		model.fetching = false
		model.result = msg.result
		model.err = msg.err

	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmd
		}
	}

	return model, nil
}

func cidrwhitelistUpdate(model cidrwhitelistModel, msg tea.Msg) (cidrwhitelistModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cidrwhitelistInitMsg:
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = cidrwhitelistModel{spinner: s, fetching: true}
		return model, tea.Batch(model.spinner.Tick, cidrwhitelistCheckCmd)
	case cidrwhitelistResultMsg:
		model.fetching = false
		model.err = msg.err
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmd
		}
	}

	return model, nil
}

func updaterUpdate(model updaterModel, msg tea.Msg) (updaterModel, tea.Cmd) {
	switch msg := msg.(type) {
	case updaterInitMsg:
		ctx, cancel := context.WithCancel(context.Background())
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = updaterModel{fetching: true, spinner: s, ctx: ctx, cancel: cancel}
		if msg.forceInetlookupUpdate {
			return model, func() tea.Msg { return updaterSelfNoopMsg{} }
		}
		model.progress = "checking for updates to itself"
		return model, tea.Batch(updaterSelfCmd(model.ctx), model.spinner.Tick)
	case updaterSelfNoopMsg:
		model.progress = "checking for geoip updates"
		model.fetching = true
		return model, tea.Batch(updaterInetlookupCmd(model.ctx), model.spinner.Tick)
	case updaterSelfDoneMsg:
		model.fetching = false
		model.restartRequired = true
		model.progress = msg.version
		return model, nil
	case updaterErrMsg:
		model.fetching = false
		model.err = msg.err
		if isTimeoutErr(model.err) {
			model.err = fmt.Errorf("network timeout in the update mechanism; you can skip this step or restart the utility")
		}
		return model, nil
	case updaterDoneMsg:
		if model.cancel != nil {
			model.cancel()
		}
		model.fetching = false
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmd
		}
	}

	return model, nil
}

func webhostUpdate(model webhostModel, msg tea.Msg) (webhostModel, tea.Cmd) {
	if !model.inited {
		switch msg := msg.(type) {
		case webhostInitMsg:
			model := webhostInitModel()
			return model, tea.Batch(model.spinner.Tick, webhostProducerStartCmd(model.ctx, msg.Mode))
		}

		return model, nil
	}

	switch msg := msg.(type) {
	case webhostProducerStartedMsg:
		model.out = msg.out
		return model, webhostConsumerCmd(model.out)
	case webhostItemMsg:
		return webhostProcessItem(msg, model), tea.Batch(webhostConsumerCmd(model.out), tea.ClearScreen)
	case webhostProgressMsg:
		model.progress = string(msg)
		return model, webhostConsumerCmd(model.out)
	case webhostProducerDoneMsg:
		model.fetching = false
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmd
		}
	case returnedToMenuMsg:
		if model.cancel != nil {
			model.cancel()
		}
		model = webhostModel{}
		return model, nil
	}

	var cmd tea.Cmd
	model.table, cmd = model.table.Update(msg)
	return model, cmd
}

func webhostProcessItem(msg webhostItemMsg, model webhostModel) webhostModel {
	cfg := config.Get().Checkers.Webhost

	model.progress = fmt.Sprintf(
		`webhost checker => for "%s" host is ready: %v`,
		msg.Bag.Name,
		msg.Out.IpInfo.Ip,
	)

	var txMbps, rxMbps float64
	speed := "⚠️ skip"
	if msg.Out.Throughput.TxElapsed > 0 && msg.Out.Throughput.RxElapsed > 0 {
		thp := msg.Out.Throughput
		const bytesToMegabits = 8.0 / 1_000
		txMbps = float64(msg.Out.Throughput.TxBytes) / thp.TxElapsed.Seconds() * bytesToMegabits
		rxMbps = float64(msg.Out.Throughput.RxBytes) / thp.RxElapsed.Seconds() * bytesToMegabits
		speed = fmt.Sprintf("↑%.1f ↓%.1f", txMbps, rxMbps)
	}

	row := table.Row{
		msg.Bag.Name,
		msg.Out.IpInfo.Org,
		fmt.Sprintf("AS%d", msg.Out.IpInfo.Asn),
		countryIsoToFlagEmoji(msg.Out.IpInfo.CountryIso) + " " + msg.Out.IpInfo.CountryIso,
		msg.Out.IpInfo.Ip.String(),
		msg.Out.IpInfo.Subnet.String(),
		webhostPrettyAlive(msg.Out.Alive),
		webhostPrettyTcp1620(msg.Out.Tcp1620),
		speed,
	}

	rows := model.table.Rows()
	rows = append(rows, row)
	slices.SortFunc(rows, func(a, b table.Row) int {
		return cmp.Compare(a[0], b[0]) // by group
	})

	columns := []table.Column{
		{Title: "Group", Width: tableCellMaxLen(rows, 0, 5)},
		{Title: "Org", Width: tableCellMaxLen(rows, 1, 3)},
		{Title: "AS", Width: tableCellMaxLen(rows, 2, 7)},
		{Title: "Location", Width: 8},
		{Title: "IP", Width: tableCellMaxLen(rows, 4, 2)},
		{Title: "Prefix", Width: tableCellMaxLen(rows, 5, 6)},
		{Title: "Alive", Width: tableCellMaxLen(rows, 6, 6)},
		{Title: "Tcp 16-20", Width: tableCellMaxLen(rows, 7, 11)},
		{Title: "Burst kb/s", Width: tableCellMaxLen(rows, 8, 10)},
	}

	model.table.SetColumns(columns)
	model.table.SetRows(rows)
	model.table.SetHeight(tableHeight(model.table.Rows(), cfg.TableMaxVisibleRows))
	model.table.SetWidth(tableWidth(model.table.Columns()))

	return model
}

func webhostInitModel() webhostModel {
	ctx, cancel := context.WithCancel(context.Background())

	spin := spinner.New()
	spin.Spinner = spinnerType
	spin.Style = spinnerStyle

	t := table.New(
		table.WithFocused(true),
		table.WithStyles(tableStyle(true)),
	)

	return webhostModel{
		inited:   true,
		ctx:      ctx,
		cancel:   cancel,
		fetching: true,
		table:    t,
		spinner:  spin,
	}
}

func dnsUpdate(model dnsModel, msg tea.Msg) (dnsModel, tea.Cmd) {
	if !model.inited {
		switch msg.(type) {
		case dnsInitMsg:
			model = dnsInitModel()
			return model, tea.Batch(model.spinner.Tick, dnsProducerStartCmd(model.ctx))
		}

		return model, nil
	}

	switch msg := msg.(type) {
	case dnsProducerStartedMsg:
		model.out = msg.out
		return model, dnsConsumerCmd(model.out)
	case dnsProviderPlainMsg:
		return dnsProcessPlainProvider(msg, model), tea.Batch(dnsConsumerCmd(model.out), tea.ClearScreen)
	case dnsProviderDohMsg:
		return dnsProcessDohProvider(msg, model), tea.Batch(dnsConsumerCmd(model.out), tea.ClearScreen)
	case dnsLeakMsg:
		return dnsProcessLeak(msg, model), tea.Batch(dnsConsumerCmd(model.out), tea.ClearScreen)
	case dnsProgressMsg:
		model.progress = string(msg)
		return model, dnsConsumerCmd(model.out)
	case dnsProducerDoneMsg:
		model.fetching = false
		if model.out.progress != nil {
			close(model.out.progress)
		}
		return model, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left":
			model.providerTable.Focus()
			model.providerTable.SetStyles(tableStyle(true))
			model.leakTable.Blur()
			model.leakTable.SetStyles(tableStyle(false))
			return model, nil
		case "right":
			model.leakTable.Focus()
			model.leakTable.SetStyles(tableStyle(true))
			model.providerTable.Blur()
			model.providerTable.SetStyles(tableStyle(false))
			return model, nil
		}
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmd
		}
	case returnedToMenuMsg:
		if model.cancel != nil {
			model.cancel()
		}
		model = dnsModel{}
		return model, nil
	}

	var leakCmd, providerCmd tea.Cmd
	model.leakTable, leakCmd = model.leakTable.Update(msg)
	model.providerTable, providerCmd = model.providerTable.Update(msg)
	return model, tea.Batch(leakCmd, providerCmd)
}

func dnsProcessPlainProvider(msg dnsProviderPlainMsg, model dnsModel) dnsModel {
	model.out.progress <- fmt.Sprintf("[%s] plain: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict))
	v, ok := model.providerRows[msg.Provider]
	if !ok {
		v.dohVerdict = ErrPending
	}
	v.plainVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	return dnsUpdateProviderTable(model)
}

func dnsProcessDohProvider(msg dnsProviderDohMsg, model dnsModel) dnsModel {
	model.out.progress <- fmt.Sprintf("[%s] doh: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict))
	v, ok := model.providerRows[msg.Provider]
	if !ok {
		v.plainVerdict = ErrPending
	}
	v.dohVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	return dnsUpdateProviderTable(model)
}

func dnsProcessLeak(msg dnsLeakMsg, model dnsModel) dnsModel {
	if msg.Err != nil {
		model.out.progress <- "dns leak internal err"
		return model
	}

	model.out.progress <- "dns leak received"
	cfg := config.Get().Checkers.Dns

	newRows := []table.Row{}
	for _, item := range msg.Items {
		newRows = append(newRows, table.Row{
			item.Ip,
			item.Subnet,
			item.Asn,
			item.Org,
			fmt.Sprintf("%s %s", countryIsoToFlagEmoji(item.Location), item.Location),
		})
	}
	rows := append(model.leakTable.Rows(), newRows...)

	// by ip
	slices.SortFunc(rows, func(a, b table.Row) int {
		return netip.MustParseAddr(a[0]).Compare(netip.MustParseAddr(b[0]))
	})
	rows = slices.CompactFunc(rows, func(a, b table.Row) bool { return a[0] == b[0] })

	columns := []table.Column{
		{Title: "IP", Width: tableCellMaxLen(rows, 0, 2)},
		{Title: "Prefix", Width: tableCellMaxLen(rows, 1, 6)},
		{Title: "AS", Width: tableCellMaxLen(rows, 2, 7)},
		{Title: "Org", Width: tableCellMaxLen(rows, 3, 3)},
		{Title: "Location", Width: 8},
	}

	model.leakTable.SetColumns(columns)
	model.leakTable.SetRows(rows)

	model.tblHeight = max(model.tblHeight, tableHeight(model.leakTable.Rows(), cfg.TableMaxVisibleRows))
	model.leakTable.SetHeight(model.tblHeight)
	model.providerTable.SetHeight(model.tblHeight)
	model.leakTable.SetWidth(tableWidth(model.leakTable.Columns()))

	return model
}

func dnsUpdateProviderTable(model dnsModel) dnsModel {
	cfg := config.Get().Checkers.Dns
	rows := []table.Row{}

	for id, s := range model.providerRows {
		p := dnsPrettyProviderVerdict(s.plainVerdict)
		doh := dnsPrettyProviderVerdict(s.dohVerdict)
		row := table.Row{id, p, doh}
		rows = append(rows, row)
	}

	slices.SortFunc(rows, func(a, b table.Row) int {
		return strings.Compare(a[0], b[0]) // by provider name
	})
	columns := []table.Column{
		{Title: "Provider", Width: tableCellMaxLen(rows, 0, 14)},
		{Title: "Plain", Width: tableCellMaxLen(rows, 1, 14)},
		{Title: "DoH", Width: tableCellMaxLen(rows, 2, 14)},
	}

	model.providerTable.SetColumns(columns)
	model.providerTable.SetRows(rows)

	model.tblHeight = max(model.tblHeight, tableHeight(model.providerTable.Rows(), cfg.TableMaxVisibleRows))
	model.providerTable.SetHeight(model.tblHeight)
	model.leakTable.SetHeight(model.tblHeight)
	model.providerTable.SetWidth(tableWidth(model.providerTable.Columns()))

	return model
}

func dnsInitModel() dnsModel {
	ctx, cancel := context.WithCancel(context.Background())

	spin := spinner.New()
	spin.Spinner = spinnerType
	spin.Style = spinnerStyle

	providerTable := table.New(
		table.WithFocused(true),
		table.WithStyles(tableStyle(true)),
	)

	leakTable := table.New(
		table.WithFocused(false),
		table.WithStyles(tableStyle(false)),
	)

	return dnsModel{
		inited:        true,
		ctx:           ctx,
		cancel:        cancel,
		fetching:      true,
		spinner:       spin,
		providerRows:  map[string]dnsVerdictModel{},
		providerTable: providerTable,
		leakTable:     leakTable,
	}
}
