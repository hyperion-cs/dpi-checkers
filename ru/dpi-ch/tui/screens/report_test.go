package screens

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

func openReportScreen(t *testing.T, menu *Menu, name string) Screen {
	t.Helper()
	for i, item := range menu.items {
		if item.name == name {
			menu.pos = i
			_, open := menu.Selection()
			screen, cmd := open(t.Context())
			if cmd != nil {
				t.Fatal("opening saved results scheduled a command")
			}
			return screen
		}
	}
	t.Fatalf("missing menu entry %q", name)
	return nil
}

func showReportTableRows(t *testing.T) {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.Checkers.Dns.TableMaxVisibleRows = 5
	cfg.Checkers.Webhost.TableMaxVisibleRows = 5
}

func TestImportMenuAndSavedScreens(t *testing.T) {
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "local config only"}}
	cfg.Checkers.Webhost.FarmTimeout = 47 * time.Second
	report := &fullcheck.ReportDto{
		Version:       1,
		Whoami:        &fullcheck.WhoamiDto{Status: fullcheck.StatusDto{Code: "ERR", Reason: "NONE"}, Ip: "192.0.2.1", TtlbMs: 42},
		CidrWhitelist: &fullcheck.CidrWhitelistDto{Status: fullcheck.StatusDto{Code: "OK", Reason: "CHECKERS_CIDRWHITELIST_ERR_DETECTED"}},
		Webhost: map[string]fullcheck.WebhostDto{
			"saved section": {FarmTimeout: true},
		},
	}
	menu := NewReportMenu(report, "/reports/saved.yml")
	if menu.Loading() || menu.Tick() != nil {
		t.Fatal("viewer entered live initialization")
	}
	var names []string
	for _, item := range menu.items {
		names = append(names, item.name)
	}
	if !reflect.DeepEqual(names, []string{"Who am I?", "Am I under the CIDR whitelist?", "saved section"}) {
		t.Fatalf("menu depends on local configuration: %v", names)
	}
	if !strings.Contains(ansi.Strip(menu.View()), "Results from: /reports/saved.yml") {
		t.Fatal("menu lost report path")
	}
	whoami := openReportScreen(t, menu, whoamiTitle).(*whoamiModel)
	if whoami.fetching || !strings.Contains(whoami.View(), "192.0.2.1") {
		t.Fatal("Whoami did not display saved results")
	}
	cidr := openReportScreen(t, menu, cidrwhitelistTitle).(*cidrwhitelistModel)
	if cidr.err != cidrwhitelist.ErrDetected || !strings.Contains(ansi.Strip(cidr.View()), "You're UNDER one") {
		t.Fatal("CIDR did not display the saved status in the first frame")
	}
	webhost := openReportScreen(t, menu, "saved section").(*webhostModel)
	view := ansi.Strip(webhost.View())
	if !webhost.farmTimeout || webhost.fetching || !strings.Contains(view, "farming timeout exceeded") || strings.Contains(view, "(47s)") {
		t.Fatalf("invalid imported timeout view: %s", view)
	}
	if report.Whoami.Ip != "192.0.2.1" || !report.Webhost["saved section"].FarmTimeout {
		t.Fatal("viewer mutated the report")
	}
}

func TestImportUsesLiveWebhostTable(t *testing.T) {
	item := webhost.Item{
		Group: "example",
		Result: webhost.SingleResult{
			IpInfo:     inetlookup.IpInfo{Ip: netip.MustParseAddr("192.0.2.1"), Subnet: netip.MustParsePrefix("192.0.2.0/24"), Asn: 64496, Org: "Example", CountryIso: "ZZ"},
			Tls:        &webhost.Tls{V: 0x0304, Cn: "example.invalid", San: []string{"example.invalid"}},
			Tcp1620:    inetutil.ErrTcpReadTimeout,
			Throughput: webhost.Throughput{TxBytes: 1250, RxBytes: 2500, TxElapsed: time.Second, RxElapsed: time.Second},
		},
	}
	live := newWebhostModel()
	live.Update(webhostItemMsg(item))
	txKbps, rxKbps := 10.0, 20.0
	dto := fullcheck.WebhostItemDto{
		Group: "example", Org: "Example", AS: "AS64496", Location: "ZZ", IP: "192.0.2.1", Prefix: "192.0.2.0/24",
		Alive: fullcheck.StatusDto{Reason: "NONE"}, Tls: &fullcheck.WebhostTlsDto{V: "1.3", Cn: "example.invalid", San: []string{"example.invalid"}},
		Tcp1620: fullcheck.StatusDto{Reason: "INETUTIL_ERR_TCP_READ_TIMEOUT"}, Siberian: fullcheck.StatusDto{Reason: "NONE"},
		BurstTxKbps: &txKbps, BurstRxKbps: &rxKbps,
	}
	report := &fullcheck.ReportDto{Version: 1, Webhost: map[string]fullcheck.WebhostDto{"saved": {Items: []fullcheck.WebhostItemDto{dto, dto}}}}
	model := openReportScreen(t, NewReportMenu(report, ""), "saved").(*webhostModel)
	if !reflect.DeepEqual(model.table.Rows(), append(live.table.Rows(), live.table.Rows()...)) {
		t.Fatal("live and imported webhost rows differ")
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.table.Cursor() != 1 {
		t.Fatal("imported table lost keyboard navigation")
	}
}

func TestImportUsesLiveDnsTables(t *testing.T) {
	leakItems := []inetlookup.IpInfoStrings{{Ip: "192.0.2.2"}, {Ip: "192.0.2.1"}}
	report := &fullcheck.ReportDto{Version: 1, Dns: &fullcheck.DnsDto{
		Plain: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{"example resolver": {{Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}}}},
		Doh: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{
			"non2xx resolver": {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}, {Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}},
		}},
		Leak: &fullcheck.DnsLeakDto{Status: fullcheck.StatusDto{Reason: "NONE"}, Items: leakItems},
	}}
	model := openReportScreen(t, NewReportMenu(report, ""), dnsTitle).(*dnsModel)
	live := newDnsModel()
	live.processPlainProvider(dnsProviderPlainMsg{Provider: "example resolver", Verdict: dns.ErrProviderTimeout})
	live.processDohProvider(dnsProviderDohMsg{
		Provider: "non2xx resolver",
		Verdict:  errors.Join(dns.ErrDohNon2xxResp, dns.ErrProviderTimeout),
	})
	live.processLeak(dnsLeakMsg{Items: leakItems})
	if got, want := model.providerTable.Rows(), live.providerTable.Rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("imported provider rows %v, want live rows %v", got, want)
	}
	if got, want := model.leakTable.Rows(), live.leakTable.Rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("imported leak rows %v, want live rows %v", got, want)
	}
	row := model.providerTable.Rows()[0]
	if row[1] != "❗️timeout" || row[2] != "—" || model.leakTable.Rows()[0][0] != "192.0.2.1" {
		t.Fatalf("invalid imported DNS data: %v", row)
	}
	for _, current := range []*dnsModel{model, live} {
		current.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if !current.leakTable.Focused() || current.providerTable.Focused() {
			t.Fatal("DNS tables lost focus switching")
		}
		current.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if current.leakTable.Cursor() != 1 {
			t.Fatal("DNS table lost cursor navigation")
		}
	}
}

func TestImportMissingDnsAndUnknownStatus(t *testing.T) {
	report := &fullcheck.ReportDto{Version: 1, Dns: &fullcheck.DnsDto{
		Doh: &fullcheck.DnsReportDto{Providers: map[string][]fullcheck.StatusDto{
			"future reason":  {{Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"}, {Reason: "FUTURE_CODE"}},
			"missing reason": {{Code: "OK", Msg: "ignored"}},
			"non2xx only":    {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}},
			"non2xx unknown": {{Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}, {Reason: "UNKNOWN"}},
			"network error":  {{Reason: "INETUTIL_ERR_TCP_CONN_TIMEOUT"}, {Reason: "UNKNOWN"}},
		}},
	}}
	model := openReportScreen(t, NewReportMenu(report, ""), dnsTitle).(*dnsModel)
	view := ansi.Strip(model.View())
	if model.showLeak || strings.Contains(view, "Actually used") || strings.Contains(view, "checking") || strings.Contains(view, "ignored") {
		t.Fatalf("missing data was replaced with invented results: %s", view)
	}
	want := map[string]string{
		"future reason":  "❗️timeout, ⚠️ Unknown",
		"missing reason": "⚠️ Unknown",
		"non2xx only":    "✅ not detected",
		"non2xx unknown": "⚠️ non-2xx response, ⚠️ Unknown",
		"network error":  "❗️ tcp: connection timeout, ⚠️ Unknown",
	}
	for _, row := range model.providerTable.Rows() {
		if row[1] != "—" || row[2] != want[row[0]] {
			t.Errorf("row %q got %q and %q, want — and %q", row[0], row[1], row[2], want[row[0]])
		}
	}
}

func TestImportShowsUnknownForMissingCheckerReason(t *testing.T) {
	showReportTableRows(t)
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
		model := openReportScreen(t, NewReportMenu(report, ""), tab)
		wantCount := 1
		if tab == "saved" {
			wantCount = 3
		}
		view := ansi.Strip(model.View())
		if strings.Count(view, "⚠️ Unknown") != wantCount || strings.Contains(view, "ignored") {
			t.Errorf("tab %q did not show its unknown reasons: %s", tab, view)
		}
	}
	report.Dns.Leak.Items = nil
	model := openReportScreen(t, NewReportMenu(report, ""), dnsTitle)
	if view := ansi.Strip(model.View()); !strings.Contains(view, "⚠️ Unknown") {
		t.Errorf("empty DNS Leak table did not show its unknown reason: %s", view)
	}
}

func TestImportRendersReportTextLiterally(t *testing.T) {
	showReportTableRows(t)
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
	menu := NewReportMenu(report, color+"reports/test.json")
	for _, tt := range []struct{ tab, want string }{
		{"", "Saved webhost"}, {whoamiTitle, "Example Network"}, {section, "First host"}, {dnsTitle, "Test provider"},
	} {
		var model Screen = menu
		if tt.tab != "" {
			model = openReportScreen(t, menu, tt.tab)
		}
		view := model.View()
		if strings.Contains(view, color) || !strings.Contains(ansi.Strip(view), tt.want) {
			t.Errorf("tab %q did not display plain report text %q", tt.tab, tt.want)
		}
	}
}

func TestImportUsesLiveAddressViews(t *testing.T) {
	report := &fullcheck.ReportDto{
		Whoami: &fullcheck.WhoamiDto{Status: fullcheck.StatusDto{Reason: "INETUTIL_ERR_INTERNAL"}},
		CidrWhitelist: &fullcheck.CidrWhitelistDto{
			Status: fullcheck.StatusDto{Reason: "CHECKERS_CIDRWHITELIST_ERR_NO_INET_ACCESS"},
		},
	}
	menu := NewReportMenu(report, "")
	importedWhoami := openReportScreen(t, menu, whoamiTitle)
	liveWhoami := &whoamiModel{}
	liveWhoami.Update(whoamiResultMsg{err: inetutil.ErrInternal})
	if liveWhoami.err != inetutil.ErrInternal || liveWhoami.View() != "error when fetching ;(" || liveWhoami.View() != importedWhoami.View() {
		t.Fatal("Whoami did not preserve its typed error and fallback view")
	}

	importedCidr := openReportScreen(t, menu, cidrwhitelistTitle)
	liveCidr := &cidrwhitelistModel{}
	liveCidr.Update(cidrwhitelistResultMsg{err: cidrwhitelist.ErrNoInetAccess})
	if liveCidr.err != cidrwhitelist.ErrNoInetAccess || liveCidr.View() != importedCidr.View() {
		t.Fatal("CIDR did not preserve its typed error and specialized view")
	}
	liveCidr.Update(cidrwhitelistResultMsg{err: inetutil.ErrInternal})
	if got := ansi.Strip(liveCidr.View()); got != "Internal error ;(" {
		t.Fatalf("CIDR error view = %q", got)
	}
}

func TestImportSingleDnsTableNavigation(t *testing.T) {
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
			menu := NewReportMenu(&fullcheck.ReportDto{Version: 1, Dns: dnsReport}, "")
			model := openReportScreen(t, menu, dnsTitle).(*dnsModel)
			for _, code := range []rune{wrongDirection, tea.KeyDown} {
				model.Update(tea.KeyPressMsg{Code: code})
			}
			visible, hidden := model.providerTable, model.leakTable
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
