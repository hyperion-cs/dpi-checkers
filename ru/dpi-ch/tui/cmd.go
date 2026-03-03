package tui

import (
	"context"
	"dpich/checkers"
	"dpich/inetlookup"

	tea "github.com/charmbracelet/bubbletea"
)

func (rm rootModel) Init() tea.Cmd {
	// Immediately warm up inetlookup
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
