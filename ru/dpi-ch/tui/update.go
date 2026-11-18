package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

const (
	horizontalScrollStep = 8
	verticalScrollStep   = 3
)

var ErrPending = errors.New("err: pending")

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case exitMsg:
		if rm.router.Tab == allTab && rm.allModel.cancel != nil {
			rm.allModel.cancel()
		}
		rm.quitting = true
		rm.syncViewport()
		return rm, tea.Quit

	case tea.WindowSizeMsg:
		rm.viewport.SetWidth(max(1, msg.Width))
		rm.viewport.SetHeight(max(1, msg.Height))

	case tea.KeyPressMsg:
		k := msg.String()
		switch k {
		case "a":
			rm.viewport.ScrollLeft(horizontalScrollStep)

		case "d":
			rm.viewport.ScrollRight(horizontalScrollStep)

		case "w":
			rm.viewport.ScrollUp(verticalScrollStep)

		case "s":
			rm.viewport.ScrollDown(verticalScrollStep)
		}

		if k == "q" || k == "й" || k == "esc" || k == "ctrl+c" || k == "ctrl+с" {
			return rm, exitCmd
		}

		if k == "m" || k == "ь" || k == "backspace" {
			rm.cleanupTab()

			if rm.router.Tab == updaterTab {
				cmds = append(cmds, func() tea.Msg { return updaterDoneMsg{} })
			}

			rm.router.Tab = menuTab
			rm.viewport.SetXOffset(0)
			rm.viewport.SetYOffset(0)
		}
	}

	if rm.report == nil {
		cmds = append(cmds, rm.updateLive(msg))
	}
	if rm.router.Tab == menuTab {
		cmds = append(cmds, rm.updateMenu(msg))
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		cmds = append(cmds, rm.updateControls(key))
	}

	rm.syncViewport()

	return rm, tea.Batch(cmds...)
}

func (rm *rootModel) updateLive(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	switch msg.(type) {
	case updaterInitMsg:
		rm.router.Tab = updaterTab

	case updaterDoneMsg:
		rm.router.Tab = menuTab
		rm.viewport.SetXOffset(0)
		rm.viewport.SetYOffset(0)
		cmds = append(cmds, func() tea.Msg { return inetlookup.Default() })

	case allInitMsg:
		rm.router.Tab = allTab
	}
	rm.allModel, cmd = allUpdate(rm.allModel, msg)
	cmds = append(cmds, cmd)

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

	return tea.Batch(cmds...)
}

func (rm *rootModel) updateMenu(msg tea.Msg) tea.Cmd {
	route := rm.router
	var cmds []tea.Cmd

	if !route.inited {
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle

		route.inited = true
		route.loading = true
		route.spinner = s

		cmds = append(cmds, route.spinner.Tick)
	}

	if route.loading && inetlookup.Inited() {
		route.loading = false
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch normKey(msg.String()) {
		case "up":
			route.Menu.Prev()
		case "down":
			route.Menu.Next()
		case "enter":
			item := route.Menu.Curr()
			route.Tab = item.Tab
			if rm.report != nil {
				rm.openReportTab(item)
			} else {
				cmds = append(cmds, func() tea.Msg { return item.InitMsg })
			}
		}
	case spinner.TickMsg:
		if route.loading {
			var cmd tea.Cmd
			route.spinner, cmd = route.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

func whoamiUpdate(model whoamiModel, msg tea.Msg) (whoamiModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	switch msg := msg.(type) {
	case whoamiInitMsg:
		ctx, cancel := context.WithCancel(context.Background())
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = whoamiModel{spinner: s, fetching: true, session: &session{}, ctx: ctx, cancel: cancel}
		return model, tea.Batch(
			cmdFor(model.session, model.spinner.Tick),
			whoamiFetchCmd(model.session, model.ctx),
		)
	case whoamiResultMsg:
		model.fetching = false
		model.result = msg.result
		model.err = msg.err

	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func cidrwhitelistUpdate(model cidrwhitelistModel, msg tea.Msg) (cidrwhitelistModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	switch msg := msg.(type) {
	case cidrwhitelistInitMsg:
		ctx, cancel := context.WithCancel(context.Background())
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = cidrwhitelistModel{spinner: s, fetching: true, session: &session{}, ctx: ctx, cancel: cancel}
		return model, tea.Batch(
			cmdFor(model.session, model.spinner.Tick),
			cidrwhitelistCheckCmd(model.session, model.ctx),
		)
	case cidrwhitelistResultMsg:
		model.fetching = false
		model.err = msg.err
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func updaterUpdate(model updaterModel, msg tea.Msg) (updaterModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	switch msg := msg.(type) {
	case updaterInitMsg:
		ctx, cancel := context.WithCancel(context.Background())
		s := spinner.New()
		s.Spinner = spinnerType
		s.Style = spinnerStyle
		model = updaterModel{fetching: true, spinner: s, ctx: ctx, cancel: cancel, allFlag: msg.allFlag, session: &session{}}

		allConfig := config.Get().All
		if model.allFlag && allConfig.AutoExit {
			// updaterStartInetlookupMsg => updaterDoneMsg (checks allFlag)
			if msg.inetlookupTtu || msg.forceInetlookupUpdate {
				return model, func() tea.Msg {
					return msgFor(model.session, updaterStartInetlookupMsg{})
				}
			}
			return model, func() tea.Msg { return updaterDoneMsg{} }
		}

		if msg.forceInetlookupUpdate {
			return model, func() tea.Msg {
				return msgFor(model.session, updaterStartInetlookupMsg{})
			}
		}

		model.progress = "checking for updates to itself"
		return model, tea.Batch(
			updaterSelfCmd(model.session, model.ctx, msg.forceUpdate, msg.inetlookupTtu),
			cmdFor(model.session, model.spinner.Tick),
		)

	case updaterStartInetlookupMsg:
		model.progress = "checking for geoip updates"
		model.fetching = true
		return model, tea.Batch(
			updaterInetlookupCmd(model.session, model.ctx),
			cmdFor(model.session, model.spinner.Tick),
		)
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
		if model.allFlag {
			return model, allCmd
		}
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func webhostUpdate(model webhostModel, msg tea.Msg) (webhostModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	if !model.inited {
		switch msg := msg.(type) {
		case webhostInitMsg:
			model := webhostInitModel()
			model.ctx, model.cancel = context.WithCancel(context.Background())
			model.fetching = true
			model.farmLimit = config.Get().Checkers.Webhost.FarmTimeout
			return model, tea.Batch(
				cmdFor(model.session, model.spinner.Tick),
				webhostProducerStartCmd(model.session, model.ctx, msg.Targets),
			)
		}

		return model, nil
	}

	switch msg := msg.(type) {
	case webhostProducerStartedMsg:
		model.out = msg.out
		return model, webhostConsumerCmd(model.session, model.out)
	case webhostItemMsg:
		return webhostProcessItem(msg, model), webhostConsumerCmd(model.session, model.out)
	case webhostProgressMsg:
		model.progress = string(msg)
		return model, webhostConsumerCmd(model.session, model.out)
	case webhostProducerDoneMsg:
		model.fetching = false
		model.farmTimeout = msg.FarmTimeout
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func webhostProcessItem(msg webhostItemMsg, model webhostModel) webhostModel {
	model.progress = fmt.Sprintf(
		`webhost checker => for "%s" host is ready: %v`,
		msg.Bag.Name,
		msg.Out.IpInfo.Ip,
	)

	item := webhostTableItem{
		group: msg.Bag.Name, info: inetlookup.IpInfoAsStrings(msg.Out.IpInfo),
		alive: msg.Out.Alive, tcp1620: msg.Out.Tcp1620, siberian: msg.Out.Siberian,
		tlsVersion: msg.Out.Tls.VersionString(),
	}
	if msg.Out.Tls != nil {
		item.san, item.cn = msg.Out.Tls.San, msg.Out.Tls.Cn
	}
	item.txKbps, item.rxKbps = msg.Out.Throughput.Kbps()
	return webhostApplyItems(model, []webhostTableItem{item})
}

func webhostInitModel() webhostModel {
	spin := spinner.New()
	spin.Spinner = spinnerType
	spin.Style = spinnerStyle

	t := table.New(
		table.WithFocused(true),
		table.WithStyles(tableStyle(true)),
		table.WithKeyMap(tableKeyMap()),
	)

	return webhostModel{
		session: &session{},
		inited:  true,
		table:   t,
		spinner: spin,
	}
}

func allUpdate(model allModel, msg tea.Msg) (allModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	cfg := config.Get()
	if !model.inited {
		switch msg.(type) {
		case allInitMsg:
			ctx, cancel := context.WithCancel(context.Background())

			spin := spinner.New()
			spin.Spinner = spinnerType
			spin.Style = spinnerStyle

			model = allModel{inited: true, spinner: spin, ctx: ctx, cancel: cancel, fetching: true, session: &session{}}
			model.tasks = make(map[checkers.FullCheckTask][]allTask)
			for _, name := range cfg.All.Checkers {
				switch name {
				case "whoami":
					model.tasks[checkers.FullCheckTaskWhoami] = []allTask{{name: "Whoami"}}
				case "cidrwhitelist":
					model.tasks[checkers.FullCheckTaskCidrWhitelist] = []allTask{{name: "CIDR whitelist"}}
				case "dns":
					model.tasks[checkers.FullCheckTaskDnsLeak] = []allTask{{name: "Leak"}}
					model.tasks[checkers.FullCheckTaskDnsPlain] = []allTask{{name: "Plain"}}
					model.tasks[checkers.FullCheckTaskDnsDoh] = []allTask{{name: "DoH"}}
				case "webhost":
					tasks := make([]allTask, len(cfg.Checkers.Webhost.Sections))
					for i, section := range cfg.Checkers.Webhost.Sections {
						tasks[i] = allTask{name: section.Name}
					}
					model.tasks[checkers.FullCheckTaskWebhost] = tasks
				}
			}
			return model, tea.Batch(
				cmdFor(model.session, model.spinner.Tick),
				allProducerStartCmd(model.session, model.ctx),
			)
		}

		return model, nil
	}

	switch msg := msg.(type) {
	case allProducerStartedMsg:
		model.out = msg.out
		return model, allConsumerCmd(model.session, model.out)
	case allProgressMsg:
		progress := checkers.FullCheckProgress(msg)
		if progress.Msg != "" {
			model.progress = progress
		}
		tasks := model.tasks[progress.Task]
		if progress.Index >= 0 && progress.Index < len(tasks) {
			tasks[progress.Index].state = progress.State
		}
		return model, allConsumerCmd(model.session, model.out)
	case allProducerDoneMsg:
		model.fetching = false
		for _, tasks := range model.tasks {
			for i := range tasks {
				if tasks[i].state == checkers.FullCheckStateRunning {
					tasks[i].state = checkers.FullCheckStateError
				}
			}
		}
		if model.progress.Task != checkers.FullCheckTaskNone || model.progress.State == checkers.FullCheckStateRunning {
			model.progress = checkers.FullCheckProgress{State: checkers.FullCheckStateError, Msg: "stopped before completion"}
		}
		if cfg.All.AutoExit {
			return model, exitCmd
		}
		return model, nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func dnsUpdate(model dnsModel, msg tea.Msg) (dnsModel, tea.Cmd) {
	var ok bool
	if msg, ok = unwrapIfFor(model.session, msg); !ok {
		return model, nil
	}

	if !model.inited {
		switch msg.(type) {
		case dnsInitMsg:
			model = dnsInitModel()
			model.ctx, model.cancel = context.WithCancel(context.Background())
			model.fetching = true
			return model, tea.Batch(
				cmdFor(model.session, model.spinner.Tick),
				dnsProducerStartCmd(model.session, model.ctx),
			)
		}

		return model, nil
	}

	switch msg := msg.(type) {
	case dnsProducerStartedMsg:
		model.out = msg.out
		return model, dnsConsumerCmd(model.session, model.out)
	case dnsProviderPlainMsg:
		return dnsProcessPlainProvider(msg, model), dnsConsumerCmd(model.session, model.out)
	case dnsProviderDohMsg:
		return dnsProcessDohProvider(msg, model), dnsConsumerCmd(model.session, model.out)
	case dnsLeakMsg:
		return dnsProcessLeak(msg, model), dnsConsumerCmd(model.session, model.out)
	case dnsProgressMsg:
		model.progress = string(msg)
		return model, dnsConsumerCmd(model.session, model.out)
	case dnsProducerDoneMsg:
		model.fetching = false
		if model.out.progress != nil {
			close(model.out.progress)
		}
		model.focusPopulatedTable()
		return dnsUpdateProviderTable(model), nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return model, cmdFor(model.session, cmd)
		}
	}

	return model, nil
}

func dnsProcessPlainProvider(msg dnsProviderPlainMsg, model dnsModel) dnsModel {
	v, ok := model.providerRows[msg.Provider]
	if !ok {
		v.dohVerdict = ErrPending
	}
	v.plainVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	dnsSendProgress(model.out.progress, fmt.Sprintf("[%s] plain: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict, model.fetching)))
	return dnsUpdateProviderTable(model)
}

func dnsProcessDohProvider(msg dnsProviderDohMsg, model dnsModel) dnsModel {
	v, ok := model.providerRows[msg.Provider]
	if !ok {
		v.plainVerdict = ErrPending
	}
	v.dohVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	dnsSendProgress(model.out.progress, fmt.Sprintf("[%s] doh: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict, model.fetching)))
	return dnsUpdateProviderTable(model)
}

func dnsProcessLeak(msg dnsLeakMsg, model dnsModel) dnsModel {
	if msg.Err != nil {
		model.leakErr = msg.Err
		dnsSendProgress(model.out.progress, "dns leak internal err")
		return model
	}
	model.leakErr = nil
	dnsSendProgress(model.out.progress, "dns leak received")
	return dnsApplyLeakItems(model, msg.Items)
}

func dnsSendProgress(ch chan<- string, p string) {
	select {
	case ch <- p:
	default:
	}
}

func dnsInitModel() dnsModel {
	spin := spinner.New()
	spin.Spinner = spinnerType
	spin.Style = spinnerStyle

	providerTable := table.New(
		table.WithFocused(true),
		table.WithStyles(tableStyle(true)),
		table.WithKeyMap(tableKeyMap()),
	)

	leakTable := table.New(
		table.WithFocused(false),
		table.WithStyles(tableStyle(false)),
		table.WithKeyMap(tableKeyMap()),
	)

	return dnsModel{
		session:       &session{},
		inited:        true,
		spinner:       spin,
		showProviders: true,
		showLeak:      true,
		providerRows:  map[string]dnsVerdictModel{},
		providerTable: providerTable,
		leakTable:     leakTable,
	}
}

func (rm *rootModel) updateControls(msg tea.KeyPressMsg) tea.Cmd {
	switch rm.router.Tab {
	case webhostTab:
		model := &rm.webhostModel
		if !model.inited {
			return nil
		}
		var cmd tea.Cmd
		model.table, cmd = model.table.Update(msg)
		return cmdFor(model.session, cmd)
	case dnsTab:
		model := &rm.dnsModel
		if !model.inited {
			return nil
		}
		switch normKey(msg.String()) {
		case "left":
			if model.fetching || len(model.providerTable.Rows()) > 0 {
				model.focusProviders(true)
			}
			return nil
		case "right":
			if model.fetching || len(model.leakTable.Rows()) > 0 {
				model.focusProviders(false)
			}
			return nil
		}
		var leakCmd, providerCmd tea.Cmd
		model.leakTable, leakCmd = model.leakTable.Update(msg)
		model.providerTable, providerCmd = model.providerTable.Update(msg)
		return tea.Batch(
			cmdFor(model.session, leakCmd),
			cmdFor(model.session, providerCmd),
		)
	}
	return nil
}

func (model *dnsModel) focusPopulatedTable() {
	hasProviders := len(model.providerTable.Rows()) > 0
	hasLeak := len(model.leakTable.Rows()) > 0
	if hasProviders != hasLeak {
		model.focusProviders(hasProviders)
	}
}

func (model *dnsModel) focusProviders(providers bool) {
	model.providerTable.Blur()
	model.leakTable.Blur()
	model.providerTable.SetStyles(tableStyle(providers))
	model.leakTable.SetStyles(tableStyle(!providers))
	if providers {
		model.providerTable.Focus()
	} else {
		model.leakTable.Focus()
	}
}
