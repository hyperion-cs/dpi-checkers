package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type session struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type sessionMsg struct {
	session *session
	msg     tea.Msg
}

func newSession() *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{ctx: ctx, cancel: cancel}
}

func (s *session) bind(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		if s.ctx.Err() != nil {
			return nil
		}
		switch msg := cmd().(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			// Bubble Tea schedules batches itself. Tag the results of their commands.
			cmds := make(tea.BatchMsg, len(msg))
			for i, cmd := range msg {
				cmds[i] = s.bind(cmd)
			}
			return cmds
		default:
			return sessionMsg{s, msg}
		}
	}
}
