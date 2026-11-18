package tui

import (
	"context"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"

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
	report   *fullcheck.ReportDto

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

	progress fullcheck.Progress
	out      <-chan fullcheck.Progress
	tasks    map[fullcheck.Task][]allTask
}

type allTask struct {
	name  string
	state fullcheck.State
}

type whoamiModel struct {
	session  *session
	fetching bool
	spinner  spinner.Model
	result   whoami.Result
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
	farmLimit   time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	out    webhost.RunOut
}

type dnsChannelModel struct {
	providerPlain <-chan dns.Verdict
	providerDoh   <-chan dns.Verdict
	leak          <-chan dns.LeakResult
	progress      chan string
}

type dnsVerdictModel struct {
	plainVerdict error
	dohVerdict   error
}

type dnsModel struct {
	session       *session
	inited        bool
	fetching      bool
	spinner       spinner.Model
	progress      string
	showProviders bool
	showLeak      bool
	leakErr       error

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
