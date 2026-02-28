package tui

import (
	"context"
	"dpich/checkers"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
)

type page int

const (
	menuPage page = iota
	allPage
	whoamiPage
	cidrwhitelistPage
	dnsserverPage
	endpointPage
	tcp1620Page
)

func getPageName(p page) string {
	pageNames := map[page]string{
		menuPage:          "Menu",
		allPage:           "ALL",
		whoamiPage:        "Who am I?",
		cidrwhitelistPage: "Am I under the CIDR whitelist?",
		dnsserverPage:     "DNS",
		endpointPage:      "Popular Web Services",
		tcp1620Page:       "Infrastructure Providers",
	}

	if pn, ex := pageNames[p]; ex {
		return pn
	}
	return "Unknown"
}

type rootModel struct {
	quitting           bool
	page               page
	menuModel          menuModel
	whoamiModel        whoamiModel
	cidrwhitelistModel cidrwhitelistModel
	tcp1620Model       tcp1620Model
}

var menuOptions = []page{allPage, whoamiPage, cidrwhitelistPage, dnsserverPage, endpointPage, tcp1620Page}

type menuModel struct {
	optionIdx int
}

type whoamiModel struct {
	fetching bool
	spinner  spinner.Model
	result   checkers.WhoamiResult
	err      error
}

type cidrwhitelistModel struct {
	fetching bool
	spinner  spinner.Model
	err      error
}

type tcp1620Model struct {
	fetching bool
	rows     []table.Row
	ctx      context.Context
	cancel   context.CancelFunc
	ch       <-chan checkers.Tcp1620ResultItem
}
