package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type exitMsg struct{}

type whoamiInitMsg struct{}
type whoamiResultMsg struct {
	result whoami.Result
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
	out webhost.RunOut
}
type webhostProducerDoneMsg webhost.Summary
type webhostItemMsg webhost.Item
type webhostProgressMsg string

type dnsInitMsg struct{}
type dnsProducerStartedMsg struct {
	out dnsChannelModel
}
type dnsProducerDoneMsg struct{}
type dnsLeakMsg dns.LeakResult
type dnsProviderPlainMsg dns.Verdict
type dnsProviderDohMsg dns.Verdict
type dnsProgressMsg string

type allInitMsg struct{}
type allProducerStartedMsg struct {
	out <-chan fullcheck.Progress
}
type allProgressMsg fullcheck.Progress
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
