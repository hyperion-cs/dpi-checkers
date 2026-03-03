package tui

import (
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
