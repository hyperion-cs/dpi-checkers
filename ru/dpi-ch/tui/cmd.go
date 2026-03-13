package tui

import (
	"context"
	"dpich/checkers"
	"dpich/config"
	"dpich/inetlookup"
	"dpich/updater"

	tea "github.com/charmbracelet/bubbletea"
)

func (rm rootModel) Init() tea.Cmd {
	updaterCfg := config.Get().Updater
	ttu, _ := updater.TimeToUpdate()
	if updaterCfg.ForceInetlookupUpdate || (updaterCfg.Enabled && ttu) {
		return func() tea.Msg {
			return updaterInitMsg{forceInetlookupUpdate: updaterCfg.ForceInetlookupUpdate}
		}
	}

	// if updates are disabled, then user is responsible for maintaining current state
	return func() tea.Msg {
		inetlookup.Default()
		return nil
	}
}

func whoamiFetchCmd() tea.Msg {
	res, err := checkers.Whoami()
	return whoamiResultMsg{res, err}
}

func cidrwhitelistCheckCmd() tea.Msg {
	err := checkers.CidrWhitelist()
	return cidrwhitelistResultMsg{err: err}
}

func webhostProducerStartCmd(ctx context.Context, mode checkers.WebHostMode) tea.Cmd {
	return func() tea.Msg {
		opt := checkers.WebhostGochanRunnerOpt{Ctx: ctx, Mode: mode}
		out := checkers.WebhostGochanRunner(opt)
		return webhostProducerStartedMsg{out}
	}
}

func webhostConsumerCmd(out checkers.WebhostGochanRunnerOut) tea.Cmd {
	return func() tea.Msg {
		for out.Out != nil || out.Progress != nil {
			select {
			case v, ok := <-out.Out:
				if !ok {
					out.Out = nil
					continue
				}
				return webhostItemMsg(v)
			case v, ok := <-out.Progress:
				if !ok {
					out.Progress = nil
					continue
				}
				return webhostProgressMsg(v)
			}
		}

		return webhostProducerDoneMsg{}
	}
}

func updaterSelfCmd() tea.Msg {
	upd, err := updater.SelfCheckUpdates()
	if err == updater.ErrUnsupportedOsOrArch {
		// TODO: the user should be warned about this.
		return updaterSelfNoopMsg{}
	}

	if err != nil {
		return updaterErrMsg{err: err}
	}

	if !upd.Required {
		return updaterSelfNoopMsg{}
	}

	if err = updater.SelfUpdate(upd.Name, upd.Url); err != nil {
		return updaterErrMsg{err: err}
	}

	return updaterSelfDoneMsg{name: upd.Name}
}

func updaterInetlookupCmd() tea.Msg {
	updater.GeoliteUpdate()
	inetlookup.Default()
	return updaterInetlookupDoneMsg{}
}
