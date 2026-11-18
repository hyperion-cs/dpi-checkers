package screens

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"slices"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

var errPending = errors.New("err: pending")

type dnsModel struct {
	fetching      bool
	spinner       spinner.Model
	progress      string
	showProviders bool
	showLeak      bool
	leakErr       error

	providerRows  map[string]dnsVerdictModel
	providerTable table.Model
	leakTable     table.Model
	out           dnsChannelModel
}

type dnsVerdictModel struct {
	plainVerdict error
	dohVerdict   error
}

type dnsChannelModel struct {
	providerPlain <-chan dns.Verdict
	providerDoh   <-chan dns.Verdict
	leak          <-chan dns.LeakResult
}

type dnsProducerStartedMsg struct {
	out dnsChannelModel
}

type dnsProducerDoneMsg struct{}
type dnsLeakMsg dns.LeakResult
type dnsProviderPlainMsg dns.Verdict
type dnsProviderDohMsg dns.Verdict

func openDns(ctx context.Context) (Screen, tea.Cmd) {
	model := newDnsModel()
	model.fetching = true
	return model, tea.Batch(model.spinner.Tick, dnsProducerStartCmd(ctx))
}

func newDnsModel() *dnsModel {
	spin := primitives.NewSpinner()

	providerTable := table.New(
		table.WithFocused(true),
		table.WithStyles(primitives.TableStyle(true)),
		table.WithKeyMap(primitives.TableKeyMap()),
	)

	leakTable := table.New(
		table.WithFocused(false),
		table.WithStyles(primitives.TableStyle(false)),
		table.WithKeyMap(primitives.TableKeyMap()),
	)

	return &dnsModel{
		spinner:       spin,
		showProviders: true,
		showLeak:      true,
		providerRows:  map[string]dnsVerdictModel{},
		providerTable: providerTable,
		leakTable:     leakTable,
	}
}

func (model *dnsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case dnsProducerStartedMsg:
		model.out = msg.out
	case dnsProviderPlainMsg:
		model.processPlainProvider(msg)
	case dnsProviderDohMsg:
		model.processDohProvider(msg)
	case dnsLeakMsg:
		model.processLeak(msg)
	case dnsProducerDoneMsg:
		model.fetching = false
		model.out = dnsChannelModel{}
		model.focusPopulatedTable()
		model.updateProviderTable()
		return nil
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return cmd
		}
		return nil
	case tea.KeyPressMsg:
		return model.updateTables(msg)
	default:
		return nil
	}
	return dnsConsumerCmd(model.out)
}

func (model *dnsModel) updateTables(msg tea.KeyPressMsg) tea.Cmd {
	switch primitives.NormKey(msg.String()) {
	case "left":
		if model.fetching || len(model.providerTable.Rows()) > 0 {
			model.focusProviders(true)
		}
	case "right":
		if model.fetching || len(model.leakTable.Rows()) > 0 {
			model.focusProviders(false)
		}
	default:
		focused := &model.providerTable
		if model.leakTable.Focused() {
			focused = &model.leakTable
		}
		var cmd tea.Cmd
		*focused, cmd = focused.Update(msg)
		return cmd
	}
	return nil
}

func dnsProducerStartCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		return dnsProducerStartedMsg{
			out: dnsChannelModel{
				leak:          dns.LeakGochan(ctx),
				providerPlain: dns.PlainGochan(ctx),
				providerDoh:   dns.DohGochan(ctx),
			},
		}
	}
}

func dnsConsumerCmd(out dnsChannelModel) tea.Cmd {
	return func() tea.Msg {
		for out.providerPlain != nil || out.providerDoh != nil || out.leak != nil {
			select {
			case v, ok := <-out.providerPlain:
				if !ok {
					out.providerPlain = nil
					continue
				}
				return dnsProviderPlainMsg(v)
			case v, ok := <-out.providerDoh:
				if !ok {
					out.providerDoh = nil
					continue
				}
				return dnsProviderDohMsg(v)
			case v, ok := <-out.leak:
				if !ok {
					out.leak = nil
					continue
				}
				return dnsLeakMsg(v)
			}
		}
		return dnsProducerDoneMsg{}
	}
}

func (model *dnsModel) providerVerdict(name string) dnsVerdictModel {
	if verdict, ok := model.providerRows[name]; ok {
		return verdict
	}
	return dnsVerdictModel{plainVerdict: errPending, dohVerdict: errPending}
}

func (model *dnsModel) processPlainProvider(msg dnsProviderPlainMsg) {
	v := model.providerVerdict(msg.Provider)
	v.plainVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	model.progress = fmt.Sprintf("[%s] plain: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict, model.fetching))
	model.updateProviderTable()
}

func (model *dnsModel) processDohProvider(msg dnsProviderDohMsg) {
	v := model.providerVerdict(msg.Provider)
	v.dohVerdict = msg.Verdict
	model.providerRows[msg.Provider] = v
	model.progress = fmt.Sprintf("[%s] doh: %s", msg.Provider, dnsPrettyProviderVerdict(msg.Verdict, model.fetching))
	model.updateProviderTable()
}

func (model *dnsModel) processLeak(msg dnsLeakMsg) {
	model.progress = "dns leak received"
	if msg.Err != nil {
		model.leakErr = msg.Err
		model.progress = "dns leak internal err"
	}
	model.applyLeakItems(msg.Items)
}

func (model *dnsModel) focusPopulatedTable() {
	hasProviders := len(model.providerTable.Rows()) > 0
	hasLeak := len(model.leakTable.Rows()) > 0
	if hasProviders != hasLeak {
		model.focusProviders(hasProviders)
	}
}

func (model *dnsModel) focusProviders(providers bool) {
	if model.providerTable.Focused() == providers {
		return
	}
	model.providerTable.Blur()
	model.leakTable.Blur()
	model.providerTable.SetStyles(primitives.TableStyle(providers))
	model.leakTable.SetStyles(primitives.TableStyle(!providers))
	if providers {
		model.providerTable.Focus()
	} else {
		model.leakTable.Focus()
	}
}

func (model *dnsModel) applyLeakItems(items []inetlookup.IpInfoStrings) {
	if len(items) == 0 {
		return
	}
	rows := model.leakTable.Rows()
	for _, item := range items {
		rows = append(rows, primitives.TerminalRow(table.Row{
			item.Ip,
			item.Subnet,
			item.Asn,
			item.Org,
			fmt.Sprintf("%s %s", primitives.CountryIsoToFlagEmoji(item.Location), item.Location),
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
		{Title: "IP", Width: primitives.TableCellMaxLen(rows, 0, 2)},
		{Title: "Prefix", Width: primitives.TableCellMaxLen(rows, 1, 6)},
		{Title: "AS", Width: primitives.TableCellMaxLen(rows, 2, 7)},
		{Title: "Org", Width: primitives.TableCellMaxLen(rows, 3, 3)},
		{Title: "Location", Width: 8},
	}

	model.setTableRows(&model.leakTable, columns, rows)
}

func (model *dnsModel) updateProviderTable() {
	rows := make([]table.Row, 0, len(model.providerRows))
	for id, s := range model.providerRows {
		rows = append(rows, table.Row{
			primitives.TerminalText(id),
			dnsPrettyProviderVerdict(s.plainVerdict, model.fetching),
			dnsPrettyProviderVerdict(s.dohVerdict, model.fetching),
		})
	}
	slices.SortFunc(rows, func(a, b table.Row) int {
		return strings.Compare(a[0], b[0]) // by provider name
	})
	columns := []table.Column{
		{Title: "Provider", Width: primitives.TableCellMaxLen(rows, 0, 14)},
		{Title: "Plain", Width: primitives.TableCellMaxLen(rows, 1, 14)},
		{Title: "DoH", Width: primitives.TableCellMaxLen(rows, 2, 14)},
	}

	model.setTableRows(&model.providerTable, columns, rows)
}

func (model *dnsModel) setTableRows(tbl *table.Model, columns []table.Column, rows []table.Row) {
	tbl.SetColumns(columns)
	tbl.SetRows(rows)

	limit := config.Get().Checkers.Dns.TableMaxVisibleRows
	height := max(primitives.TableHeight(model.providerTable.Rows(), limit), primitives.TableHeight(model.leakTable.Rows(), limit))
	model.providerTable.SetHeight(height)
	model.leakTable.SetHeight(height)
	tbl.SetWidth(primitives.TableWidth(columns))
}

func (model *dnsModel) View() string {
	var r string
	providerTotal := len(model.providerTable.Rows())
	leakTotal := len(model.leakTable.Rows())

	var providerTbl, leakTbl string
	if model.showProviders && (providerTotal > 0 || !model.fetching) {
		providerTbl = "> DNS resolvers censorship:\n" + dnsTableView(model.providerTable)
	}
	if model.showLeak && (leakTotal > 0 || !model.fetching) {
		leakTbl = "> Current DNS resolver hijacking test. Actually used:\n"
		var warning string
		if model.leakErr != nil {
			warning = resultErrorText(model.leakErr, "⚠️ "+primitives.TerminalText(model.leakErr.Error()))
		}
		if leakTotal == 0 {
			text := cmp.Or(warning, "⚠️ No DNS resolver results")
			leakTbl += primitives.TableOuterBorderStyle().Render(" " + text + " ")
		} else {
			leakTbl += dnsTableView(model.leakTable)
			if warning != "" {
				leakTbl += "\n" + primitives.WarningStyle.Render(warning)
			}
		}
	}

	if providerTbl != "" || leakTbl != "" {
		r += lipgloss.JoinHorizontal(
			lipgloss.Top,
			providerTbl,
			leakTbl,
		)

		if model.fetching {
			r += "\n\n"
		}
	}

	if model.fetching {
		r += fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}

	return r
}

func dnsTableView(tbl table.Model) string {
	total := len(tbl.Rows())
	cursor := min(tbl.Cursor()+1, total)
	help := fmt.Sprintf("↑/↓ up/down; ←/→ left/right table; cursor: %d/%d", cursor, total)
	return primitives.TableOuterBorderStyle().Render(tbl.View() + "\n " + primitives.SubtleStyle.Render(help))
}

func dnsPrettyProviderVerdict(err error, fetching bool) string {
	if err == nil {
		return "✅ not detected"
	}

	if errors.Is(err, errPending) {
		if fetching {
			return "⏰ checking..."
		}
		return "—"
	}

	dnsErrs := dns.Errors(err)
	var items []string
	if len(dnsErrs) > 0 {
		items = append(items, dnsPrettyErrors(dnsErrs))
	}
	causes := []error{err}
	if group, ok := err.(interface{ Unwrap() []error }); ok {
		causes = group.Unwrap()
	}
	for _, cause := range causes {
		if inetutil.IsInetutilErr(cause) {
			items = append(items, "❗️ "+cause.Error())
		}
	}
	if errors.Is(err, fullcheck.ErrUnknownReason) {
		items = append(items, unknownResultText)
	}
	// TODO: disable this when HTTP/2 is supported.
	// It only works as long as we trust providers that use DoH.
	if len(items) == 1 && len(dnsErrs) == 1 && dnsErrs[0] == dns.ErrDohNon2xxResp {
		return "✅ not detected"
	}
	if len(items) > 0 {
		return strings.Join(items, ", ")
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return "⚠️ lookup error"
	}

	log.Println("dnsPrettyProviderVerdict", err)
	return "⚠️ internal error"
}

func dnsPrettyErrors(errs []error) string {
	items := make([]string, 0, len(errs))
	for _, err := range errs {
		switch err {
		case dns.ErrProviderHijacking:
			items = append(items, "provider hijacking")
		case dns.ErrResolveSpoofing:
			items = append(items, "response spoofing")
		case dns.ErrNxdomainSpoofing:
			items = append(items, "nxdomain spoofing")
		case dns.ErrDohBootstrapSpoofing:
			items = append(items, "bootstrap spoofing")
		case dns.ErrDohBootstrapEmpty:
			items = append(items, "empty bootstrap")
		case dns.ErrDohInsecure:
			items = append(items, "invalid https certificate")
		case dns.ErrDohNon2xxResp:
			items = append(items, "non-2xx response")
		case dns.ErrSkip:
			items = append(items, "skip")
		case dns.ErrProviderTimeout:
			items = append(items, "timeout")
		}
	}

	prefix := "❗️"
	switch errs[0] {
	case dns.ErrDohBootstrapEmpty, dns.ErrDohNon2xxResp:
		prefix = "⚠️ "
	case dns.ErrSkip:
		prefix = "⏩ "
	}
	return prefix + strings.Join(items, ", ")
}
