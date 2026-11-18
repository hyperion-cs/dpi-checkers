package tui

import (
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
)

func reportMenu(report *checkers.FullCheckDto, file string) *MenuState {
	m := &MenuState{header: "Results from: " + terminalText(file)}
	if report.Whoami != nil {
		m.Add(whoamiTitle, "", whoamiTab, true, nil)
	}
	if report.CidrWhitelist != nil {
		m.Add(cidrwhitelistTitle, "", cidrwhitelistTab, true, nil)
	}
	for _, name := range slices.Sorted(maps.Keys(report.Webhost)) {
		m.Add(name, "", webhostTab, true, nil)
	}
	if report.Dns.HasResults() {
		m.Add(dnsTitle, "", dnsTab, true, nil)
	}
	return m
}

func (rm *rootModel) openReportTab(item MenuItem) {
	switch item.Tab {
	case whoamiTab:
		r := rm.report.Whoami
		rm.whoamiModel = whoamiModel{
			result: checkers.WhoamiResult{
				Ip: r.Ip, Subnet: r.Subnet, Asn: r.Asn, Org: r.Org,
				Location: r.Location, Ttlb: time.Duration(r.TtlbMs) * time.Millisecond,
			},
			err: checkers.FullCheckErrorFromReason(r.Status.Reason),
		}
	case cidrwhitelistTab:
		rm.cidrwhitelistModel = cidrwhitelistModel{err: checkers.FullCheckErrorFromReason(rm.report.CidrWhitelist.Status.Reason)}
	case webhostTab:
		section := rm.report.Webhost[item.Name]
		items := make([]webhostTableItem, 0, len(section.Items))
		for _, result := range section.Items {
			items = append(items, reportWebhostItem(result))
		}
		rm.webhostModel = webhostApplyItems(webhostInitModel(), items)
		rm.webhostModel.farmTimeout = section.FarmTimeout
	case dnsTab:
		r := rm.report.Dns
		model := dnsInitModel()
		model.showProviders = r.Plain != nil || r.Doh != nil
		model.showLeak = r.Leak != nil
		if r.Plain != nil {
			for name, statuses := range r.Plain.Providers {
				model.providerRows[name] = dnsVerdictModel{
					plainVerdict: reportDnsError(statuses),
					dohVerdict:   ErrPending,
				}
			}
		}
		if r.Doh != nil {
			for name, statuses := range r.Doh.Providers {
				verdict, ok := model.providerRows[name]
				if !ok {
					verdict.plainVerdict = ErrPending
				}
				verdict.dohVerdict = reportDnsError(statuses)
				model.providerRows[name] = verdict
			}
		}
		model = dnsUpdateProviderTable(model)
		if r.Leak != nil {
			model.leakErr = checkers.FullCheckErrorFromReason(r.Leak.Status.Reason)
			model = dnsApplyLeakItems(model, r.Leak.Items)
		}
		model.focusPopulatedTable()
		rm.dnsModel = model
	}
}

func reportWebhostItem(item checkers.FullCheckWebhostItemDto) webhostTableItem {
	r := webhostTableItem{
		group: item.Group,
		info: inetlookup.IpInfoStrings{
			Ip: item.IP, Subnet: item.Prefix, Asn: item.AS, Org: item.Org, Location: item.Location,
		},
		alive:      checkers.FullCheckErrorFromReason(item.Alive.Reason),
		tcp1620:    checkers.FullCheckErrorFromReason(item.Tcp1620.Reason),
		siberian:   checkers.FullCheckErrorFromReason(item.Siberian.Reason),
		tlsVersion: " — ",
		txKbps:     item.BurstTxKbps,
		rxKbps:     item.BurstRxKbps,
	}
	if item.Tls != nil {
		r.tlsVersion, r.san, r.cn = item.Tls.V, item.Tls.San, item.Tls.Cn
	}
	return r
}

func reportDnsError(statuses []checkers.FullCheckStatusDto) error {
	switch len(statuses) {
	case 0:
		return checkers.ErrFullCheckUnknownReason
	case 1:
		return checkers.FullCheckErrorFromReason(statuses[0].Reason)
	}
	errs := make([]error, 0, len(statuses))
	for _, status := range statuses {
		errs = append(errs, checkers.FullCheckErrorFromReason(status.Reason))
	}
	return errors.Join(errs...)
}
