package screens

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

const unknownResultText = "⚠️ Unknown"

func NewReportMenu(report *fullcheck.ReportDto, file string) *Menu {
	menu := &Menu{header: "Results from: " + primitives.TerminalText(file)}
	if result := report.Whoami; result != nil {
		menu.items = append(menu.items, reportItem(whoamiTitle, func() Screen {
			return &whoamiModel{
				result: whoami.Result{
					Ip: result.Ip, Subnet: result.Subnet, Asn: result.Asn, Org: result.Org,
					Location: result.Location, Ttlb: time.Duration(result.TtlbMs) * time.Millisecond,
				},
				err: fullcheck.ErrorFromReason(result.Status.Reason),
			}
		}))
	}
	if result := report.CidrWhitelist; result != nil {
		menu.items = append(menu.items, reportItem(cidrwhitelistTitle, func() Screen {
			return &cidrwhitelistModel{err: fullcheck.ErrorFromReason(result.Status.Reason)}
		}))
	}
	for _, name := range slices.Sorted(maps.Keys(report.Webhost)) {
		menu.items = append(menu.items, reportItem(name, func() Screen {
			return reportWebhost(report.Webhost[name])
		}))
	}
	if report.Dns.HasResults() {
		menu.items = append(menu.items, reportItem(dnsTitle, func() Screen { return reportDns(report.Dns) }))
	}
	return menu
}

func reportItem(name string, view func() Screen) menuItem {
	return menuItem{name: name, featured: true, open: func(context.Context) (Screen, tea.Cmd) {
		return view(), nil
	}}
}

func reportWebhost(section fullcheck.WebhostDto) *webhostModel {
	items := make([]webhostTableItem, 0, len(section.Items))
	for _, result := range section.Items {
		items = append(items, reportWebhostItem(result))
	}
	model := newWebhostModel()
	model.applyItems(items)
	model.farmTimeout = section.FarmTimeout
	return model
}

func reportDns(result *fullcheck.DnsDto) *dnsModel {
	model := newDnsModel()
	model.showProviders = result.Plain != nil || result.Doh != nil
	model.showLeak = result.Leak != nil
	if result.Plain != nil {
		for name, statuses := range result.Plain.Providers {
			verdict := model.providerVerdict(name)
			verdict.plainVerdict = reportDnsError(statuses)
			model.providerRows[name] = verdict
		}
	}
	if result.Doh != nil {
		for name, statuses := range result.Doh.Providers {
			verdict := model.providerVerdict(name)
			verdict.dohVerdict = reportDnsError(statuses)
			model.providerRows[name] = verdict
		}
	}
	model.updateProviderTable()
	if result.Leak != nil {
		model.leakErr = fullcheck.ErrorFromReason(result.Leak.Status.Reason)
		model.applyLeakItems(result.Leak.Items)
	}
	model.focusPopulatedTable()
	return model
}

func resultErrorText(err error, fallback string) string {
	if err == fullcheck.ErrUnknownReason {
		return unknownResultText
	}
	return fallback
}

func reportWebhostItem(item fullcheck.WebhostItemDto) webhostTableItem {
	r := webhostTableItem{
		group: item.Group,
		info: inetlookup.IpInfoStrings{
			Ip: item.IP, Subnet: item.Prefix, Asn: item.AS, Org: item.Org, Location: item.Location,
		},
		alive:      fullcheck.ErrorFromReason(item.Alive.Reason),
		tcp1620:    fullcheck.ErrorFromReason(item.Tcp1620.Reason),
		siberian:   fullcheck.ErrorFromReason(item.Siberian.Reason),
		tlsVersion: " — ",
		txKbps:     item.BurstTxKbps,
		rxKbps:     item.BurstRxKbps,
	}
	if item.Tls != nil {
		r.tlsVersion, r.san, r.cn = item.Tls.V, item.Tls.San, item.Tls.Cn
	}
	return r
}

func reportDnsError(statuses []fullcheck.StatusDto) error {
	switch len(statuses) {
	case 0:
		return fullcheck.ErrUnknownReason
	case 1:
		return fullcheck.ErrorFromReason(statuses[0].Reason)
	}
	errs := make([]error, 0, len(statuses))
	for _, status := range statuses {
		errs = append(errs, fullcheck.ErrorFromReason(status.Reason))
	}
	return errors.Join(errs...)
}
