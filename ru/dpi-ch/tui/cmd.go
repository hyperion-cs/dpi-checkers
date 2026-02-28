package tui

import (
	"context"
	"dpich/checkers"

	tea "github.com/charmbracelet/bubbletea"
)

func (rm rootModel) Init() tea.Cmd {
	// Do nothing when starting tui
	return nil
}

func whoamiFetchCmd() tea.Msg {
	res, err := checkers.Whoami()
	return whoamiResultMsg{res, err}
}

func cidrwhitelistCheckCmd() tea.Msg {
	err := checkers.CidrWhitelist()
	return cidrwhitelistResultMsg{err: err}
}

func tcp1620ProducerStartCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ch := checkers.Tcp1620Start(ctx)
		return tcp1620ProducerStartedMsg{ch}
	}
}

func tcp1620ConsumerCmd(ch <-chan checkers.Tcp1620ResultItem) tea.Cmd {
	return func() tea.Msg {
		if x, ok := <-ch; ok {
			return tcp1620ItemMsg(x)
		}
		return tcp1620ProducerDoneMsg{}
	}
}
