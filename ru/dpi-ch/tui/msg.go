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

type tcp1620InitMsg struct{}
type tcp1620ProducerStartedMsg struct {
	ch <-chan checkers.Tcp1620ResultItem
}
type tcp1620ProducerDoneMsg struct{}
type tcp1620ItemMsg checkers.Tcp1620ResultItem
