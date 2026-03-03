package tui

import (
	"cmp"
	"context"
	"dpich/checkers"
	"fmt"
	"slices"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Only root processing here
	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := msg.String()
		if k == "q" || k == "й" || k == "esc" || k == "ctrl+c" {
			rm.quitting = true
			return rm, tea.Quit
		}

		if k == "m" || k == "ь" || k == "backspace" {
			rm.page = menuPage
			cmds = append(cmds, func() tea.Msg { return returnedToMenuMsg{} })
		}
	case rootMsg:
		rm.page = msg.page
	}

	rm.menuModel, cmd = menuUpdate(rm.menuModel, msg)
	cmds = append(cmds, cmd)

	rm.whoamiModel, cmd = whoamiUpdate(rm.whoamiModel, msg)
	cmds = append(cmds, cmd)

	rm.cidrwhitelistModel, cmd = cidrwhitelistUpdate(rm.cidrwhitelistModel, msg)
	cmds = append(cmds, cmd)

	rm.webhostModel, cmd = webhostUpdate(rm.webhostModel, msg)
	cmds = append(cmds, cmd)

	return rm, tea.Batch(cmds...)
}

func menuUpdate(model menuModel, msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			case webhostInfraPage:
				// Сюда то мы и будем класть тип сервиска...
				initMsg = webhostInitMsg{}
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

func webhostUpdate(model webhostModel, msg tea.Msg) (webhostModel, tea.Cmd) {
	switch msg := msg.(type) {
	case webhostInitMsg:
		// TODO: Should we move ctx/cancel to webhostProducerStartedMsg?
		ctx, cancel := context.WithCancel(context.Background())
		model = webhostModel{ctx: ctx, cancel: cancel, fetching: true}
		return model, webhostProducerStartCmd(ctx)
	case webhostProducerStartedMsg:
		model.out = msg.out
		return model, webhostConsumerCmd(model.out)
	case webhostItemMsg:
		// 		type WebhostSingleResult struct {
		// 	IpInfo  inetlookup.IpInfo
		// 	Port    int
		// 	TlsV    uint16
		// 	Sni     string
		// 	Host    string
		// 	Alive   error
		// 	Tcp1620 error
		// }

		// type IpInfo struct {
		// 	Ip         netip.Addr
		// 	Asn        int32
		// 	Subnet     netip.Prefix
		// 	Org        string
		// 	CountryIso string
		// }

		model.progress = fmt.Sprintf(
			`webhost checker => for "%s" ready: %v`,
			msg.Bag.Name,
			msg.Out.IpInfo.Ip,
		)

		// {Title: "Group", Width: 20},
		// {Title: "AS", Width: 14},
		// {Title: "IP", Width: 15},
		// {Title: "Prefix", Width: 18},
		// {Title: "Location", Width: 5},
		// {Title: "Alive", Width: 5},
		// {Title: "Tcp 16-20", Width: 5},

		r := table.Row{
			msg.Bag.Name,
			fmt.Sprintf("AS%d", msg.Out.IpInfo.Asn),
			countryIsoToFlagEmoji(msg.Out.IpInfo.CountryIso) + " " + msg.Out.IpInfo.CountryIso,
			msg.Out.IpInfo.Ip.String(),
			msg.Out.IpInfo.Subnet.String(),
			prettyAlive(msg.Out.Alive),
			prettyTcp1620(msg.Out.Tcp1620),
		}
		model.rows = append(model.rows, r)
		slices.SortFunc(model.rows, func(a, b table.Row) int {
			return cmp.Compare(a[0], b[0]) // by group
		})

		return model, webhostConsumerCmd(model.out)
	case webhostProgressMsg:
		model.progress = string(msg)
		return model, webhostConsumerCmd(model.out)
	case webhostProducerDoneMsg:
		model.fetching = false
		return model, nil
	case returnedToMenuMsg:
		if model.cancel != nil {
			model.cancel()
		}
		return model, nil
	}

	return model, nil
}

// var (
// 	ErrWebhostTcpConnReset        = errors.New("tcp: connection reset")
// 	ErrWebhostTcpConnTimeout      = errors.New("tcp: connection timeout")
// 	ErrWebhostTcpWriteTimeout     = errors.New("tcp: write timeout")
// 	ErrWebhostTcpReadTimeout      = errors.New("tcp: read timeout")
// 	ErrWebhostTlsHandshakeTimeout = errors.New("tls: handshake timeout")
// 	ErrWebhostTlsHandshakeFail    = errors.New("tls: handshake failure")
// 	ErrWebhostTlsBadRecordMac     = errors.New("tls: bad record MAC")
// 	ErrWebhostTlsWriteBrokenPipe  = errors.New("tls/write: broken pipe")
// 	ErrWebhostInternal            = errors.New("check: internal error")
// 	ErrWebhostSkip                = errors.New("check: skip")
// )

func prettyAlive(err error) string {
	if err == nil {
		return "Yes 🟢"
	}

	return " No 🔴"
}

func prettyTcp1620(err error) string {
	switch err {
	case nil:
		return "No ✅"
	case checkers.ErrWebhostTcpWriteTimeout, checkers.ErrWebhostTcpReadTimeout:
		return "Detected❗️"
	case checkers.ErrWebhostSkip:
		return "Skip ⚠️"
	default:
		return "Possible ⚠️"
	}
}

func countryIsoToFlagEmoji(iso string) string {
	if len(iso) != 2 {
		return ""
	}

	runes := []rune(iso)
	for i := range 2 {
		c := runes[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if c < 'A' || c > 'Z' {
			return ""
		}
		runes[i] = rune(0x1F1E6 + (c - 'A'))
	}

	return string(runes)
}
