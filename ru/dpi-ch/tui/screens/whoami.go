package screens

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

type whoamiModel struct {
	fetching bool
	spinner  spinner.Model
	result   whoami.Result
	err      error
}

type whoamiResultMsg struct {
	result whoami.Result
	err    error
}

func openWhoami(ctx context.Context) (Screen, tea.Cmd) {
	model := &whoamiModel{fetching: true, spinner: primitives.NewSpinner()}
	return model, tea.Batch(model.spinner.Tick, func() tea.Msg {
		result, err := whoami.Check(ctx)
		return whoamiResultMsg{result, err}
	})
}

func (model *whoamiModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case whoamiResultMsg:
		model.fetching = false
		model.result = msg.result
		model.err = msg.err
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func (model *whoamiModel) View() string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}
	if model.err != nil {
		return resultErrorText(model.err, "error when fetching ;(")
	}

	r := model.result
	location := primitives.TerminalText(r.Location)
	return fmt.Sprintf("IP: %s\nSubnet: %s\nOrg: %s (%s)\nLocation: %s %s\nTTLB: %d ms",
		primitives.TerminalText(r.Ip), primitives.TerminalText(r.Subnet), primitives.TerminalText(r.Org), primitives.TerminalText(r.Asn),
		primitives.CountryIsoToFlagEmoji(location), location, r.Ttlb.Milliseconds())
}
