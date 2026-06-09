package tui

import (
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type returnedToMenuMsg struct{}
type allInitMsg struct{}

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

type updaterInitMsg struct {
	forceUpdate           bool
	forceInetlookupUpdate bool
}

type updaterErrMsg struct{ err error }
type updaterStartInetlookupMsg struct{}
type updaterSelfDoneMsg struct{ version string }
type updaterDoneMsg struct{}
