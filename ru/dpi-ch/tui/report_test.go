package tui

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func importTestConfig(t *testing.T) {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.Checkers.Webhost.TableMaxVisibleRows = 5
	cfg.Checkers.Webhost.FarmTimeout = 47 * time.Second
	cfg.Checkers.Dns.TableMaxVisibleRows = 5
	cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "local config only"}}
	cfg.Updater.Enabled = true
	cfg.Updater.ForceUpdate = true
	cfg.Updater.ForceInetlookupUpdate = true
	cfg.All.Flag = true
	cfg.All.AutoExit = true
}

func openReportTab(t *testing.T, model rootModel, name string) rootModel {
	t.Helper()
	for i, item := range model.router.Menu.items {
		if item.Name != name {
			continue
		}
		model.router.Menu.pos = i
		model.router.Tab = menuTab
		next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("opening saved results scheduled a command")
		}
		opened := next.(rootModel)
		if opened.router.Tab != item.Tab {
			t.Fatal("menu did not open the selected tab")
		}
		return opened
	}
	t.Fatalf("missing menu entry %q", name)
	return model
}

func TestImportMenuAndOfflineNavigation(t *testing.T) {
	importTestConfig(t)
	report := &fullcheck.ReportDto{
		Version:       1,
		Whoami:        &fullcheck.WhoamiDto{Status: fullcheck.StatusDto{Code: "ERR", Reason: "NONE"}, Ip: "192.0.2.1", TtlbMs: 42},
		CidrWhitelist: &fullcheck.CidrWhitelistDto{Status: fullcheck.StatusDto{Code: "OK", Reason: "CHECKERS_CIDRWHITELIST_ERR_DETECTED"}},
		Webhost: map[string]fullcheck.WebhostDto{
			"saved section": {FarmTimeout: true},
		},
	}
	model := newRootModel(TuiOpt{Report: report, ReportPath: "/reports/saved.yml"})
	if model.Init() != nil || model.router.loading || !model.router.inited {
		t.Fatal("viewer entered live initialization")
	}
	var names []string
	for _, item := range model.router.Menu.items {
		names = append(names, item.Name)
	}
	if !reflect.DeepEqual(names, []string{"Who am I?", "Am I under the CIDR whitelist?", "saved section"}) {
		t.Fatalf("menu depends on local configuration: %v", names)
	}
	if !strings.Contains(ansi.Strip(model.render()), "Results from: /reports/saved.yml") {
		t.Fatal("menu lost report path")
	}
	model = openReportTab(t, model, "Who am I?")
	if model.whoamiModel.fetching || model.whoamiModel.ctx != nil || !strings.Contains(whoamiView(model.whoamiModel), "192.0.2.1") {
		t.Fatal("Whoami did not display saved results")
	}
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd != nil || next.(rootModel).router.Tab != menuTab {
		t.Fatal("return to imported menu scheduled work")
	}
	model = openReportTab(t, next.(rootModel), "Am I under the CIDR whitelist?")
	if model.cidrwhitelistModel.err != cidrwhitelist.ErrDetected || !strings.Contains(ansi.Strip(model.View().Content), "You're UNDER one") || model.cidrwhitelistModel.ctx != nil {
		t.Fatal("CIDR did not display the saved status in the first frame")
	}
	model = openReportTab(t, model, "saved section")
	view := ansi.Strip(webhostView(model.webhostModel))
	if !model.webhostModel.farmTimeout || model.webhostModel.fetching || model.webhostModel.ctx != nil || !strings.Contains(view, "farming timeout exceeded") || strings.Contains(view, "(47s)") {
		t.Fatalf("invalid imported timeout view: %s", view)
	}
	for _, msg := range []tea.Msg{allInitMsg{}, updaterInitMsg{}, updaterDoneMsg{}} {
		next, cmd = model.Update(msg)
		model = next.(rootModel)
		if cmd != nil || model.router.Tab != webhostTab || model.allModel.inited || model.updaterModel.fetching {
			t.Fatal("live lifecycle message started work in viewer")
		}
	}
	if report.Whoami.Ip != "192.0.2.1" || !report.Webhost["saved section"].FarmTimeout {
		t.Fatal("viewer mutated the report")
	}
}

func TestImportUsesLiveWebhostTable(t *testing.T) {
	importTestConfig(t)
	item := webhost.Item{
		Group: "example",
		Result: webhost.SingleResult{
			IpInfo:     inetlookup.IpInfo{Ip: netip.MustParseAddr("192.0.2.1"), Subnet: netip.MustParsePrefix("192.0.2.0/24"), Asn: 64496, Org: "Example", CountryIso: "ZZ"},
			Tls:        &webhost.Tls{V: 0x0304, Cn: "example.invalid", San: []string{"example.invalid"}},
			Tcp1620:    inetutil.ErrTcpReadTimeout,
			Throughput: webhost.Throughput{TxBytes: 1250, RxBytes: 2500, TxElapsed: time.Second, RxElapsed: time.Second},
		},
	}
	live := webhostProcessItem(webhostItemMsg(item), webhostInitModel())
	txKbps, rxKbps := 10.0, 20.0
	dto := fullcheck.WebhostItemDto{
		Group: "example", Org: "Example", AS: "AS64496", Location: "ZZ", IP: "192.0.2.1", Prefix: "192.0.2.0/24",
		Alive: fullcheck.StatusDto{Reason: "NONE"}, Tls: &fullcheck.WebhostTlsDto{V: "1.3", Cn: "example.invalid", San: []string{"example.invalid"}},
		Tcp1620: fullcheck.StatusDto{Reason: "INETUTIL_ERR_TCP_READ_TIMEOUT"}, Siberian: fullcheck.StatusDto{Reason: "NONE"},
		BurstTxKbps: &txKbps, BurstRxKbps: &rxKbps,
	}
	report := &fullcheck.ReportDto{Version: 1, Webhost: map[string]fullcheck.WebhostDto{"saved": {Items: []fullcheck.WebhostItemDto{dto, dto}}}}
	model := openReportTab(t, newRootModel(TuiOpt{Report: report}), "saved")
	if !reflect.DeepEqual(model.webhostModel.table.Rows(), append(live.table.Rows(), live.table.Rows()...)) {
		t.Fatal("live and imported webhost rows differ")
	}
	next, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if next.(rootModel).webhostModel.table.Cursor() != 1 {
		t.Fatal("imported table lost keyboard navigation")
	}
}

func TestImportUsesLiveDnsTables(t *testing.T) {
	importTestConfig(t)
	leakItems := []inetlookup.IpInfoStrings{{Ip: "192.0.2.2"}, {Ip: "192.0.2.1"}}
	report := &fullcheck.ReportDto{Version: 1, Dns: &fullcheck.DnsDto{
		Plain: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{"example resolver": {{Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}}}},
		Doh: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{
			"non2xx resolver": {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}, {Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}},
		}},
		Leak: &fullcheck.DnsLeakDto{Status: fullcheck.StatusDto{Reason: "NONE"}, Items: leakItems},
	}}
	model := openReportTab(t, newRootModel(TuiOpt{Report: report}), "DNS")
	live := dnsProcessPlainProvider(dnsProviderPlainMsg{Provider: "example resolver", Verdict: dns.ErrProviderTimeout}, dnsInitModel())
	live = dnsProcessDohProvider(dnsProviderDohMsg{
		Provider: "non2xx resolver",
		Verdict:  errors.Join(dns.ErrDohNon2xxResp, dns.ErrProviderTimeout),
	}, live)
	live = dnsProcessLeak(dnsLeakMsg{Items: leakItems}, live)
	if got, want := model.dnsModel.providerTable.Rows(), live.providerTable.Rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("imported provider rows %v, want live rows %v", got, want)
	}
	if got, want := model.dnsModel.leakTable.Rows(), live.leakTable.Rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("imported leak rows %v, want live rows %v", got, want)
	}
	row := model.dnsModel.providerTable.Rows()[0]
	if row[1] != "❗️timeout" || row[2] != "—" || model.dnsModel.leakTable.Rows()[0][0] != "192.0.2.1" {
		t.Fatalf("invalid imported DNS data: %v", row)
	}
	liveRoot := newRootModel(TuiOpt{})
	liveRoot.router.Tab = dnsTab
	liveRoot.dnsModel = live
	for _, current := range []rootModel{model, liveRoot} {
		next, _ := current.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		current = next.(rootModel)
		if !current.dnsModel.leakTable.Focused() || current.dnsModel.providerTable.Focused() {
			t.Fatal("DNS tables lost focus switching")
		}
		next, _ = current.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if next.(rootModel).dnsModel.leakTable.Cursor() != 1 {
			t.Fatal("DNS table lost cursor navigation")
		}
	}
}

func TestImportMissingDnsAndUnknownStatus(t *testing.T) {
	importTestConfig(t)
	report := &fullcheck.ReportDto{Version: 1, Dns: &fullcheck.DnsDto{
		Doh: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{
			"future reason":  {{Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}, {Reason: "FUTURE_CODE"}},
			"missing reason": {{Code: "OK", Msg: "ignored"}},
			"non2xx only":    {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}},
			"non2xx unknown": {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}, {Reason: "UNKNOWN"}},
			"network error":  {{Reason: "INETUTIL_ERR_TCP_CONN_TIMEOUT"}, {Reason: "UNKNOWN"}},
		}},
	}}
	model := openReportTab(t, newRootModel(TuiOpt{Report: report}), "DNS")
	view := ansi.Strip(dnsView(model.dnsModel))
	if model.dnsModel.showLeak || strings.Contains(view, "Actually used") || strings.Contains(view, "checking") || strings.Contains(view, "ignored") {
		t.Fatalf("missing data was replaced with invented results: %s", view)
	}
	want := map[string]string{
		"future reason":  "❗️timeout, ⚠️ Unknown",
		"missing reason": "⚠️ Unknown",
		"non2xx only":    "✅ not detected",
		"non2xx unknown": "⚠️ non-2xx response, ⚠️ Unknown",
		"network error":  "❗️ tcp: connection timeout, ⚠️ Unknown",
	}
	for _, row := range model.dnsModel.providerTable.Rows() {
		if row[1] != "—" || row[2] != want[row[0]] {
			t.Errorf("row %q got %q and %q, want — and %q", row[0], row[1], row[2], want[row[0]])
		}
	}
}

func TestImportShowsUnknownForMissingCheckerReason(t *testing.T) {
	importTestConfig(t)
	status := fullcheck.StatusDto{Code: "OK", Msg: "ignored"}
	report := &fullcheck.ReportDto{
		Version:       1,
		Whoami:        &fullcheck.WhoamiDto{Status: status},
		CidrWhitelist: &fullcheck.CidrWhitelistDto{Status: status},
		Webhost: map[string]fullcheck.WebhostDto{"saved": {Items: []fullcheck.WebhostItemDto{{
			Alive: status, Tcp1620: status, Siberian: status,
		}}}},
		Dns: &fullcheck.DnsDto{Leak: &fullcheck.DnsLeakDto{
			Status: status, Items: []inetlookup.IpInfoStrings{{Ip: "192.0.2.1"}},
		}},
	}
	for _, tab := range []string{whoamiTitle, cidrwhitelistTitle, "saved", dnsTitle} {
		model := openReportTab(t, newRootModel(TuiOpt{Report: report}), tab)
		wantCount := 1
		if tab == "saved" {
			wantCount = 3
		}
		view := ansi.Strip(model.render())
		if strings.Count(view, "⚠️ Unknown") != wantCount || strings.Contains(view, "ignored") {
			t.Errorf("tab %q did not show its unknown reasons: %s", tab, view)
		}
	}
	report.Dns.Leak.Items = nil
	model := openReportTab(t, newRootModel(TuiOpt{Report: report}), dnsTitle)
	if view := ansi.Strip(model.render()); !strings.Contains(view, "⚠️ Unknown") {
		t.Errorf("empty DNS Leak table did not show its unknown reason: %s", view)
	}
}

func TestImportRendersReportTextLiterally(t *testing.T) {
	importTestConfig(t)
	const color = "\x1b[38;5;123m"
	section := color + "Saved\nwebhost"
	report := &fullcheck.ReportDto{
		Version: 1,
		Whoami: &fullcheck.WhoamiDto{
			Status: fullcheck.StatusDto{Reason: "NONE"}, Org: color + "Example\rNetwork",
		},
		Webhost: map[string]fullcheck.WebhostDto{section: {Items: []fullcheck.WebhostItemDto{{
			Group: color + "First\thost",
		}}}},
		Dns: &fullcheck.DnsDto{Plain: &fullcheck.DnsReportDto{
			Providers: map[string][]fullcheck.StatusDto{color + "Test\nprovider": {{Reason: "NONE"}}},
		}},
	}
	model := newRootModel(TuiOpt{Report: report, ReportPath: color + "reports/test.json"})
	for _, tt := range []struct{ tab, want string }{
		{"", "Saved webhost"}, {whoamiTitle, "Example Network"}, {section, "First host"}, {dnsTitle, "Test provider"},
	} {
		if tt.tab != "" {
			model = openReportTab(t, model, tt.tab)
		}
		view := model.render()
		if strings.Contains(view, color) || !strings.Contains(ansi.Strip(view), tt.want) {
			t.Errorf("tab %q did not display plain report text %q", tt.tab, tt.want)
		}
	}
}

func TestLiveModelsKeepTypedErrors(t *testing.T) {
	importTestConfig(t)
	joined := errors.Join(dns.ErrResolveSpoofing, dns.ErrProviderTimeout)
	whoamiState, _ := whoamiUpdate(whoamiModel{}, whoamiResultMsg{err: joined})
	cidr, _ := cidrwhitelistUpdate(cidrwhitelistModel{}, cidrwhitelistResultMsg{err: cidrwhitelist.ErrNoInetAccess})
	dnsState := dnsProcessPlainProvider(dnsProviderPlainMsg{Provider: "example", Verdict: joined}, dnsInitModel())
	if whoamiState.err != joined || cidr.err != cidrwhitelist.ErrNoInetAccess || dnsState.providerRows["example"].plainVerdict != joined {
		t.Fatal("live model replaced its original error")
	}
	item := webhostItemMsg{Result: webhost.SingleResult{Alive: inetutil.ErrTlsBadRecordMac}}
	webhostState := webhostProcessItem(item, webhostInitModel())
	if !strings.HasPrefix(webhostState.table.Rows()[0][6], "⚠️ ") {
		t.Fatal("live webhost formatter lost a typed TLS warning")
	}
	if got := webhostPrettyAlive(webhost.ErrBlockedBySni); got != "🔴 tls: blocked by sni" {
		t.Errorf("SNI block message = %q", got)
	}
	if whoamiView(whoamiState) != "error when fetching ;(" || !strings.Contains(ansi.Strip(cidrwhitelistView(cidrwhitelistModel{err: joined})), "Internal error ;(") {
		t.Fatal("live generic errors lost their usual messages")
	}
}

func TestImportSingleDnsTableNavigation(t *testing.T) {
	importTestConfig(t)
	for _, tt := range []struct {
		name         string
		leakOnly     bool
		otherPresent bool
	}{
		{"providers only", false, false},
		{"leak only", true, false},
		{"failed leak", false, true},
		{"empty providers", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			leakOnly := tt.leakOnly
			dnsReport := &fullcheck.DnsDto{}
			wrongDirection := tea.KeyRight
			if leakOnly {
				dnsReport.Leak = &fullcheck.DnsLeakDto{Status: fullcheck.StatusDto{Reason: "NONE"}, Items: []inetlookup.IpInfoStrings{
					{Ip: "192.0.2.11invalid", Location: "\u00e9"}, {Ip: "192.0.2.10"}, {Ip: "192.0.2.2"},
				}}
				wrongDirection = tea.KeyLeft
			} else {
				dnsReport.Plain = &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{
					"a": {{Reason: "CHECKERS_DNS_ERR_RESOLVE_SPOOFING"}}, "b": {{Reason: "NONE"}},
				}}
			}
			if tt.otherPresent {
				if leakOnly {
					dnsReport.Plain = &fullcheck.DnsReportDto{Status: fullcheck.StatusDto{Reason: "NONE"}}
				} else {
					dnsReport.Leak = &fullcheck.DnsLeakDto{Status: fullcheck.StatusDto{Reason: "UNKNOWN"}}
				}
			}
			model := openReportTab(t, newRootModel(TuiOpt{Report: &fullcheck.ReportDto{Version: 1, Dns: dnsReport}}), "DNS")
			for _, code := range []rune{wrongDirection, tea.KeyDown} {
				next, _ := model.Update(tea.KeyPressMsg{Code: code})
				model = next.(rootModel)
			}
			visible, hidden := model.dnsModel.providerTable, model.dnsModel.leakTable
			if leakOnly {
				visible, hidden = hidden, visible
				rows := visible.Rows()
				if rows[0][0] != "192.0.2.2" || rows[1][0] != "192.0.2.10" || rows[2][0] != "192.0.2.11invalid" {
					t.Fatalf("invalid IP sort order: %v", rows)
				}
			} else if !strings.Contains(visible.Rows()[0][1], "response spoofing") {
				t.Fatal("status reason disappeared when message was absent")
			}
			if !visible.Focused() || hidden.Focused() || visible.Cursor() != 1 {
				t.Fatal("navigation moved focus away from the populated table")
			}
		})
	}
}

func TestLiveInitializationAfterReportSupport(t *testing.T) {
	importTestConfig(t)
	for _, tt := range []struct {
		name string
		tab  tab
		msg  tea.Msg
	}{
		{"whoami", whoamiTab, nil},
		{"cidr", cidrwhitelistTab, nil},
		{"webhost", webhostTab, nil},
		{"dns", dnsTab, nil},
		{"all", allTab, allInitMsg{}},
		{"updater", updaterTab, updaterInitMsg{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newRootModel(TuiOpt{})
			msg := tt.msg
			if msg == nil {
				model.router.inited = true
				for i, item := range model.router.Menu.items {
					if item.Tab == tt.tab {
						model.router.Menu.pos = i
						break
					}
				}
				next, choose := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				model = next.(rootModel)
				if choose == nil || model.router.Tab != tt.tab {
					t.Fatal("live menu did not select the checker")
				}
				msg = choose()
			}
			next, cmd := model.Update(msg)
			model = next.(rootModel)
			defer model.cleanupTab()
			fetching := false
			switch tt.tab {
			case whoamiTab:
				fetching = model.whoamiModel.fetching && model.whoamiModel.ctx != nil
			case cidrwhitelistTab:
				fetching = model.cidrwhitelistModel.fetching && model.cidrwhitelistModel.ctx != nil
			case webhostTab:
				fetching = model.webhostModel.fetching && model.webhostModel.ctx != nil
				model.webhostModel.farmTimeout = true
				if !strings.Contains(webhostView(model.webhostModel), "(47s)") {
					t.Fatal("live timeout lost its configured duration")
				}
			case dnsTab:
				fetching = model.dnsModel.fetching && model.dnsModel.ctx != nil
			case allTab:
				fetching = model.allModel.fetching && model.allModel.ctx != nil
			case updaterTab:
				fetching = model.updaterModel.fetching && model.updaterModel.ctx != nil
			}
			if cmd == nil || !fetching {
				t.Fatal("live initialization no longer schedules its producer")
			}
			if (tt.tab == allTab || tt.tab == updaterTab) && (model.router.Tab != tt.tab || model.router.inited) {
				t.Fatal("live startup initialized the menu before selecting its tab")
			}
		})
	}
}
