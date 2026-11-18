package screens

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

type cidrwhitelistModel struct {
	fetching bool
	spinner  spinner.Model
	err      error
}

type cidrwhitelistResultMsg struct{ err error }

func openCidrWhitelist(ctx context.Context) (Screen, tea.Cmd) {
	model := &cidrwhitelistModel{fetching: true, spinner: primitives.NewSpinner()}
	return model, tea.Batch(model.spinner.Tick, func() tea.Msg {
		return cidrwhitelistResultMsg{err: cidrwhitelist.Check(ctx)}
	})
}

func (model *cidrwhitelistModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case cidrwhitelistResultMsg:
		model.fetching = false
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

func (model *cidrwhitelistModel) View() string {
	if model.fetching {
		return fmt.Sprintf("%s fetching...", model.spinner.View())
	}
	switch model.err {
	case nil:
		return primitives.OkStyle.Render("You're NOT under one ;)")
	case cidrwhitelist.ErrDetected:
		return primitives.DangerStyle.Render("You're UNDER one :(")
	case cidrwhitelist.ErrNoInetAccess:
		return primitives.WarningStyle.Render("It seems that there is no Internet access (even to resources from the whitelist).")
	default:
		return primitives.WarningStyle.Render(resultErrorText(model.err, "Internal error ;("))
	}
}
