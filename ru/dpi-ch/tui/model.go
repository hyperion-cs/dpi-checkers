package tui

import (
	"dpich/checkers"

	"github.com/charmbracelet/bubbles/spinner"
)

type page int

const (
	menuPage page = iota
	allPage
	whoamiPage
	cidrwhitelistPage
	webhostPopularPage
	webhostInfraPage
)

func getPageName(p page) string {
	pageNames := map[page]string{
		menuPage:           "Menu",
		allPage:            "ALL",
		whoamiPage:         "Who am I?",
		cidrwhitelistPage:  "Am I under the CIDR whitelist?",
		webhostPopularPage: "Popular Web Services",
		webhostInfraPage:   "Infrastructure Providers",
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
