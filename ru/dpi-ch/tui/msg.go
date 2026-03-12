package tui

import "dpich/checkers"

type rootMsg struct {
	page page
}

type returnedToMenuMsg struct{}

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
	Mode checkers.WebHostMode
}
type webhostProducerStartedMsg struct {
	out checkers.WebhostGochanRunnerOut
}
type webhostProducerDoneMsg struct{}
type webhostItemMsg checkers.WebhostGochanOut[checkers.WebhostGochanBag]
type webhostProgressMsg string

type updaterInitMsg struct{}
type updaterDoneMsg struct{}
