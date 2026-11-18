package tui

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"charm.land/bubbles/v2/table"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
)

type webhostTableItem struct {
	group      string
	info       inetlookup.IpInfoStrings
	alive      error
	tcp1620    error
	siberian   error
	tlsVersion string
	san        []string
	cn         string
	txKbps     *float64
	rxKbps     *float64
}

func webhostApplyItems(model webhostModel, items []webhostTableItem) webhostModel {
	cfg := config.Get().Checkers.Webhost
	rows := model.table.Rows()
	for _, item := range items {
		speed := "⚠️ skip"
		if item.txKbps != nil && item.rxKbps != nil {
			speed = fmt.Sprintf("↑%.1f ↓%.1f", *item.txKbps, *item.rxKbps)
		}
		rows = append(rows, terminalRow(table.Row{
			item.group, item.info.Org, item.info.Asn,
			countryIsoToFlagEmoji(item.info.Location) + " " + item.info.Location,
			item.info.Ip, item.info.Subnet,
			webhostPrettyAlive(item.alive), item.tlsVersion, webhostPrettySanCn(item.san, item.cn),
			webhostPrettyTcp1620(item.tcp1620), webhostPrettySiberian(item.siberian), speed,
		}))
	}
	slices.SortFunc(rows, func(a, b table.Row) int {
		const ru = "🇷🇺 RU"
		if a[3] == ru && b[3] != ru {
			return 1
		}
		if a[3] != ru && b[3] == ru {
			return -1
		}

		return cmp.Compare(a[0], b[0]) // by group
	})

	columns := []table.Column{
		{Title: "Group", Width: tableCellMaxLen(rows, 0, 5)},
		{Title: "Org", Width: tableCellMaxLen(rows, 1, 3)},
		{Title: "AS", Width: tableCellMaxLen(rows, 2, 7)},
		{Title: "Loc", Width: 5},
		{Title: "IP", Width: tableCellMaxLen(rows, 4, 2)},
		{Title: "Prefix", Width: tableCellMaxLen(rows, 5, 6)},
		{Title: "Alive", Width: tableCellMaxLen(rows, 6, 6)},
		{Title: "TlsV", Width: tableCellMaxLen(rows, 7, 4)},
		{Title: "Cert SAN/CN", Width: tableCellMaxLen(rows, 8, 4)},
		{Title: "Tcp 16-20", Width: tableCellMaxLen(rows, 9, 9)},
		{Title: "Siberian", Width: tableCellMaxLen(rows, 10, 8)},
		{Title: "Burst kb/s", Width: tableCellMaxLen(rows, 11, 10)},
	}

	model.table.SetColumns(columns)
	model.table.SetRows(rows)
	model.table.SetHeight(tableHeight(model.table.Rows(), cfg.TableMaxVisibleRows))
	model.table.SetWidth(tableWidth(model.table.Columns()))

	return model
}

func dnsApplyLeakItems(model dnsModel, items []inetlookup.IpInfoStrings) dnsModel {
	rows := model.leakTable.Rows()
	for _, item := range items {
		rows = append(rows, terminalRow(table.Row{
			item.Ip,
			item.Subnet,
			item.Asn,
			item.Org,
			fmt.Sprintf("%s %s", countryIsoToFlagEmoji(item.Location), item.Location),
		}))
	}

	// by ip
	slices.SortFunc(rows, func(a, b table.Row) int {
		ai, ae := netip.ParseAddr(a[0])
		bi, be := netip.ParseAddr(b[0])
		if (ae == nil) != (be == nil) {
			if ae == nil {
				return -1
			}
			return 1
		}
		if ae == nil && be == nil {
			return ai.Compare(bi)
		}
		return strings.Compare(a[0], b[0])
	})
	rows = slices.CompactFunc(rows, func(a, b table.Row) bool { return a[0] == b[0] })

	columns := []table.Column{
		{Title: "IP", Width: tableCellMaxLen(rows, 0, 2)},
		{Title: "Prefix", Width: tableCellMaxLen(rows, 1, 6)},
		{Title: "AS", Width: tableCellMaxLen(rows, 2, 7)},
		{Title: "Org", Width: tableCellMaxLen(rows, 3, 3)},
		{Title: "Location", Width: 8},
	}

	model.setTableRows(&model.leakTable, columns, rows)
	return model
}

func dnsUpdateProviderTable(model dnsModel) dnsModel {
	rows := make([]table.Row, 0, len(model.providerRows))
	for id, s := range model.providerRows {
		rows = append(rows, table.Row{
			terminalText(id),
			dnsPrettyProviderVerdict(s.plainVerdict, model.fetching),
			dnsPrettyProviderVerdict(s.dohVerdict, model.fetching),
		})
	}
	return dnsSetProviderRows(model, rows)
}

func dnsSetProviderRows(model dnsModel, rows []table.Row) dnsModel {
	slices.SortFunc(rows, func(a, b table.Row) int {
		return strings.Compare(a[0], b[0]) // by provider name
	})
	columns := []table.Column{
		{Title: "Provider", Width: tableCellMaxLen(rows, 0, 14)},
		{Title: "Plain", Width: tableCellMaxLen(rows, 1, 14)},
		{Title: "DoH", Width: tableCellMaxLen(rows, 2, 14)},
	}

	model.setTableRows(&model.providerTable, columns, rows)
	return model
}

func (model *dnsModel) setTableRows(tbl *table.Model, columns []table.Column, rows []table.Row) {
	tbl.SetColumns(columns)
	tbl.SetRows(rows)

	limit := config.Get().Checkers.Dns.TableMaxVisibleRows
	height := max(tableHeight(model.providerTable.Rows(), limit), tableHeight(model.leakTable.Rows(), limit))
	model.providerTable.SetHeight(height)
	model.leakTable.SetHeight(height)
	tbl.SetWidth(tableWidth(columns))
}

func terminalRow(row table.Row) table.Row {
	for i, cell := range row {
		row[i] = terminalText(cell)
	}
	return row
}
