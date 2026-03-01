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
