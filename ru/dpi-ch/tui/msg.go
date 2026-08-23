package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type exitMsg struct{}

type whoamiInitMsg struct{}
type whoamiResultMsg struct {
	result checkers.WhoamiResult
	err    error
}

type cidrwhitelistInitMsg struct{}
type cidrwhitelistResultMsg struct {
	err error
}

type webhostInitMsg struct {
	Targets []config.WebhostTarget
}
type webhostProducerStartedMsg struct {
	out checkers.WebhostGochanRunnerOut
}
type webhostProducerDoneMsg struct{}
type webhostItemMsg checkers.WebhostGochanOut[checkers.WebhostGochanBag]
type webhostProgressMsg string

type dnsInitMsg struct{}
type dnsProducerStartedMsg struct {
	out dnsChannelModel
}
type dnsProducerDoneMsg struct{}
type dnsLeakMsg checkers.DnsLeakWithIpinfoOut
type dnsProviderPlainMsg checkers.DnsVerdict
type dnsProviderDohMsg checkers.DnsVerdict
type dnsProgressMsg string

type allInitMsg struct{}
type allProducerStartedMsg struct {
	out <-chan checkers.FullCheckProgress
}
type allProgressMsg checkers.FullCheckProgress
type allProducerDoneMsg struct{}

type updaterInitMsg struct {
	selfTtu               bool
	inetlookupTtu         bool
	forceUpdate           bool
	forceInetlookupUpdate bool
	allFlag               bool
}

type updaterErrMsg struct{ err error }
type updaterStartInetlookupMsg struct{}
type updaterSelfDoneMsg struct{ version string }
type updaterDoneMsg struct{}

type sessionMsg struct {
	session *session
	msg     tea.Msg
}

// Wraps an internal msg, associating its result with a specific session.
func msgFor(session *session, msg tea.Msg) tea.Msg {
	if msg == nil {
		return nil
	}
	return sessionMsg{session: session, msg: msg}
}

// Check if the msg is from the current session (or a general msg).
// If so, return the unwrapped value.
func unwrapIfFor(session *session, msg tea.Msg) (tea.Msg, bool) {
	if sm, ok := msg.(sessionMsg); ok {
		if sm.session == session {
			return sm.msg, true
		}
		return nil, false
	}

	// general msg
	return msg, true
}
