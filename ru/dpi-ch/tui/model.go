package tui

import (
	"context"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
)

// isolate messages from the bubble tea shared queue
type session struct {
	_ byte
}

type rootModel struct {
	quitting bool
	router   *router
	viewport viewport.Model

	allModel           allModel
	whoamiModel        whoamiModel
	cidrwhitelistModel cidrwhitelistModel
	webhostModel       webhostModel
	dnsModel           dnsModel
	updaterModel       updaterModel
}

type allModel struct {
	session  *session
	inited   bool
	fetching bool
	spinner  spinner.Model

	ctx    context.Context
	cancel context.CancelFunc

	progress checkers.FullCheckProgress
	out      <-chan checkers.FullCheckProgress
}

type whoamiModel struct {
	session  *session
	fetching bool
	spinner  spinner.Model
	result   checkers.WhoamiResult
	err      error
	ctx      context.Context
	cancel   context.CancelFunc
}

type cidrwhitelistModel struct {
	session  *session
	fetching bool
	spinner  spinner.Model
	err      error
	ctx      context.Context
	cancel   context.CancelFunc
}

type webhostModel struct {
	session     *session
	inited      bool
	fetching    bool
	spinner     spinner.Model
	progress    string
	table       table.Model
	farmTimeout bool

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
	session  *session
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
	session *session
	ctx     context.Context
	cancel  context.CancelFunc

	err             error
	restartRequired bool
	fetching        bool
	spinner         spinner.Model
	progress        string
	allFlag         bool
}

func (rm *rootModel) cleanupTab() {
	switch rm.router.Tab {
	case allTab:
		if rm.allModel.cancel != nil {
			rm.allModel.cancel()
		}
		rm.allModel = allModel{}

	case whoamiTab:
		if rm.whoamiModel.cancel != nil {
			rm.whoamiModel.cancel()
		}
		rm.whoamiModel = whoamiModel{}

	case cidrwhitelistTab:
		if rm.cidrwhitelistModel.cancel != nil {
			rm.cidrwhitelistModel.cancel()
		}
		rm.cidrwhitelistModel = cidrwhitelistModel{}

	case webhostTab:
		if rm.webhostModel.cancel != nil {
			rm.webhostModel.cancel()
		}
		rm.webhostModel = webhostModel{}

	case dnsTab:
		if rm.dnsModel.cancel != nil {
			rm.dnsModel.cancel()
		}
		rm.dnsModel = dnsModel{}

	case updaterTab:
		if rm.updaterModel.cancel != nil {
			rm.updaterModel.cancel()
		}
		rm.updaterModel = updaterModel{}
	}
}
