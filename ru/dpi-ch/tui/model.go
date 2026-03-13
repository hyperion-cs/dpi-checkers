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
	webhostPopularPage
	webhostInfraPage
	updaterPage
)

func getPageName(p page) string {
	pageNames := map[page]string{
		menuPage:           "Menu",
		allPage:            "ALL",
		whoamiPage:         "Who am I?",
		cidrwhitelistPage:  "Am I under the CIDR whitelist?",
		webhostPopularPage: "Popular Web Services",
		webhostInfraPage:   "Infrastructure Providers",
		updaterPage:        "Updater",
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
	webhostModel       webhostModel
	updaterModel       updaterModel
}

var menuOptions = []page{allPage, whoamiPage, cidrwhitelistPage, webhostPopularPage, webhostInfraPage}

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

type webhostModel struct {
	fetching bool
	spinner  spinner.Model
	progress string
	rows     []table.Row
	table    table.Model

	ctx    context.Context
	cancel context.CancelFunc
	out    checkers.WebhostGochanRunnerOut
}

type updaterModel struct {
	ctx    context.Context
	cancel context.CancelFunc

	err             error
	restartRequired bool
	fetching        bool
	spinner         spinner.Model
	progress        string
}
