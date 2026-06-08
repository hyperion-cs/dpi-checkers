package tui

import (
	"context"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
)

type rootModel struct {
	quitting bool
	router   *router

	whoamiModel        whoamiModel
	cidrwhitelistModel cidrwhitelistModel
	webhostModel       webhostModel
	dnsModel           dnsModel
	updaterModel       updaterModel
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
	inited   bool
	fetching bool
	spinner  spinner.Model
	progress string
	table    table.Model

	ctx    context.Context
	cancel context.CancelFunc
	out    checkers.WebhostGochanRunnerOut
}

type dnsChannelModel struct {
	providerPlain <-chan checkers.DnsVerdict
	providerDoh   <-chan checkers.DnsVerdict
	leak          <-chan checkers.DnsLeakWithIpinfoOut
	progress      chan string
}

type dnsVerdictModel struct {
	plainVerdict error
	dohVerdict   error
}

type dnsModel struct {
	inited   bool
	fetching bool
	spinner  spinner.Model
	progress string

	tblHeight     int
	providerRows  map[string]dnsVerdictModel
	providerTable table.Model
	leakTable     table.Model

	out    dnsChannelModel
	ctx    context.Context
	cancel context.CancelFunc
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
