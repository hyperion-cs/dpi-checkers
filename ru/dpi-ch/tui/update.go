package tui

import (
	"context"
	"dpich/checkers"

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

	rm.tcp1620Model, cmd = tcp1620Update(rm.tcp1620Model, msg)
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

func tcp1620Update(model tcp1620Model, msg tea.Msg) (tcp1620Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tcp1620InitMsg:
		// TODO: Should we move ctx/cancel to tcp1620ProducerStartedMsg?
		ctx, cancel := context.WithCancel(context.Background())
		model = tcp1620Model{ctx: ctx, cancel: cancel, fetching: true}
		return model, tcp1620ProducerStartCmd(ctx)
	case tcp1620ProducerStartedMsg:
		model.ch = msg.ch
		return model, tcp1620ConsumerCmd(model.ch)
	case tcp1620ItemMsg:
		// TODO: This shouldn't be here
		if msg.Attrs.Country == "XX" {
			msg.Attrs.Country = "N/A"
		}

		s := "Not detected"
		switch msg.Err {
		case checkers.ErrTcp1620Conn:
			s = "Conn Err"
		case checkers.ErrTcp1620Read:
			s = "Detected"
		}

		r := table.Row{
			msg.Attrs.Id,
			msg.Attrs.Subnet,
			msg.Attrs.Holder,
			"AS" + msg.Attrs.Asn,
			msg.Attrs.CountryEmoji + " " + msg.Attrs.Country,
			s,
		}
		model.rows = append(model.rows, r)
		return model, tcp1620ConsumerCmd(model.ch)
	case tcp1620ProducerDoneMsg:
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
