package screens

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
	"golang.org/x/net/publicsuffix"
)

type webhostModel struct {
	fetching    bool
	spinner     spinner.Model
	progress    string
	table       table.Model
	farmTimeout bool
	farmLimit   time.Duration
	err         error

	out webhost.RunOut
}

type webhostProducerStartedMsg struct {
	out webhost.RunOut
}
type webhostProducerDoneMsg webhost.Summary
type webhostItemMsg webhost.Item
type webhostProgressMsg string

func newWebhostModel() *webhostModel {
	spin := primitives.NewSpinner()

	t := table.New(
		table.WithFocused(true),
		table.WithStyles(primitives.TableStyle(true)),
		table.WithKeyMap(primitives.TableKeyMap()),
	)

	return &webhostModel{
		table:   t,
		spinner: spin,
	}
}

func openWebhost(ctx context.Context, targets []config.WebhostTarget) (Screen, tea.Cmd) {
	model := newWebhostModel()
	model.fetching = true
	model.farmLimit = config.Get().Checkers.Webhost.FarmTimeout
	start := func() tea.Msg {
		return webhostProducerStartedMsg{out: webhost.Run(ctx, targets)}
	}
	return model, tea.Batch(model.spinner.Tick, start)
}

func (model *webhostModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case webhostProducerStartedMsg:
		model.out = msg.out
		return webhostConsumerCmd(model.out)
	case webhostItemMsg:
		model.processItem(msg)
		return webhostConsumerCmd(model.out)
	case webhostProgressMsg:
		model.progress = string(msg)
		return webhostConsumerCmd(model.out)
	case webhostProducerDoneMsg:
		model.fetching = false
		model.farmTimeout = msg.FarmTimeout
		model.err = msg.Err
	case tea.KeyPressMsg:
		var cmd tea.Cmd
		model.table, cmd = model.table.Update(msg)
		return cmd
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func webhostConsumerCmd(out webhost.RunOut) tea.Cmd {
	return func() tea.Msg {
		for out.Out != nil || out.Progress != nil {
			select {
			case v, ok := <-out.Out:
				if !ok {
					out.Out = nil
					continue
				}
				return webhostItemMsg(v)
			case v, ok := <-out.Progress:
				if !ok {
					out.Progress = nil
					continue
				}
				return webhostProgressMsg(v)
			}
		}

		return webhostProducerDoneMsg(<-out.Summary)
	}
}

func (model *webhostModel) View() string {
	var r string
	cfg := config.Get().Checkers.Webhost
	total := len(model.table.Rows())

	if total > 0 {
		cursor := model.table.Cursor() + 1
		over := ""
		if total > cfg.TableMaxVisibleRows {
			over = " 👀"
		}

		inner := model.table.View() +
			"\n " + model.table.HelpView() +
			primitives.SubtleStyle.Render(fmt.Sprintf("; cursor: %d/%d%s", cursor, total, over))

		r += primitives.TableOuterBorderStyle().Render(inner) + "\n\n"
	}
	if model.fetching {
		r += fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}
	if model.farmTimeout {
		r += "⏰ farming timeout exceeded"
		if model.farmLimit > 0 {
			r += " (" + model.farmLimit.String() + ")"
		}
		r += "\n"
	}
	if model.err != nil {
		r += "⚠️ error: " + primitives.TerminalText(model.err.Error()) + "\n"
	}
	r += fmt.Sprintf("count: %d pcs.", total)
	return r
}

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

func (model *webhostModel) applyItems(items []webhostTableItem) {
	cfg := config.Get().Checkers.Webhost
	rows := model.table.Rows()
	for _, item := range items {
		speed := "⚠️ skip"
		if item.txKbps != nil && item.rxKbps != nil {
			speed = fmt.Sprintf("↑%.1f ↓%.1f", *item.txKbps, *item.rxKbps)
		}
		rows = append(rows, primitives.TerminalRow(table.Row{
			item.group, item.info.Org, item.info.Asn,
			primitives.CountryIsoToFlagEmoji(item.info.Location) + " " + item.info.Location,
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
		{Title: "Group", Width: primitives.TableCellMaxLen(rows, 0, 5)},
		{Title: "Org", Width: primitives.TableCellMaxLen(rows, 1, 3)},
		{Title: "AS", Width: primitives.TableCellMaxLen(rows, 2, 7)},
		{Title: "Loc", Width: 5},
		{Title: "IP", Width: primitives.TableCellMaxLen(rows, 4, 2)},
		{Title: "Prefix", Width: primitives.TableCellMaxLen(rows, 5, 6)},
		{Title: "Alive", Width: primitives.TableCellMaxLen(rows, 6, 6)},
		{Title: "TlsV", Width: primitives.TableCellMaxLen(rows, 7, 4)},
		{Title: "Cert SAN/CN", Width: primitives.TableCellMaxLen(rows, 8, 4)},
		{Title: "Tcp 16-20", Width: primitives.TableCellMaxLen(rows, 9, 9)},
		{Title: "Siberian", Width: primitives.TableCellMaxLen(rows, 10, 8)},
		{Title: "Burst kb/s", Width: primitives.TableCellMaxLen(rows, 11, 10)},
	}

	model.table.SetColumns(columns)
	model.table.SetRows(rows)
	model.table.SetHeight(primitives.TableHeight(model.table.Rows(), cfg.TableMaxVisibleRows))
	model.table.SetWidth(primitives.TableWidth(model.table.Columns()))

}

func (model *webhostModel) processItem(msg webhostItemMsg) {
	model.progress = fmt.Sprintf(
		`webhost checker => for "%s" host is ready: %v`,
		msg.Group,
		msg.Result.IpInfo.Ip,
	)

	item := webhostTableItem{
		group: msg.Group, info: inetlookup.IpInfoAsStrings(msg.Result.IpInfo),
		alive: msg.Result.Alive, tcp1620: msg.Result.Tcp1620, siberian: msg.Result.Siberian,
		tlsVersion: msg.Result.Tls.VersionString(),
	}
	if msg.Result.Tls != nil {
		item.san, item.cn = msg.Result.Tls.San, msg.Result.Tls.Cn
	}
	item.txKbps, item.rxKbps = msg.Result.Throughput.Kbps()
	model.applyItems([]webhostTableItem{item})
}

func webhostPrettyAlive(err error) string {
	switch err {
	case nil:
		return "🟢 yes"
	case inetutil.ErrHttpMalformedResponse:
		return "🟢 custom http"
	case fullcheck.ErrUnknownReason:
		return unknownResultText
	case inetutil.ErrTlsInvalidKeyShare, inetutil.ErrTlsBadRecordMac:
		return fmt.Sprintf("⚠️ %s", err)
	}

	return fmt.Sprintf("🔴 %s", err)
}

func webhostPrettyTcp1620(err error) string {
	switch err {
	case nil:
		return "✅ no"
	case inetutil.ErrTcpWriteTimeout, inetutil.ErrTcpReadTimeout:
		return "❗️detected"
	case webhost.ErrSkip:
		return "⚠️ skip"
	case inetutil.ErrTlsWriteBrokenPipe:
		return "⚠️ not supported by host"
	default:
		return resultErrorText(err, fmt.Sprintf("⚠️ %s", err))
	}
}

func webhostPrettySanCn(san []string, cn string) string {
	const sub, max = 15, 25
	x := inetutil.TlsSanCn(san, cn)
	res := x.Name
	if res == "" {
		res = "—"
	}

	if utf8.RuneCountInString(x.Name) > max {
		if name, err := publicsuffix.EffectiveTLDPlusOne(x.Name); err == nil && utf8.RuneCountInString(name) <= max {
			res = "///." + name
		} else {
			runes := []rune(x.Name)
			res = "///" + string(runes[len(runes)-sub:])
		}
	}

	switch {
	case x.Single && x.Wild:
		res = "[*.]" + res
	case x.Wild:
		res = "*." + res
	}

	if x.Some {
		res += "+"
	}

	return res
}

func webhostPrettySiberian(err error) string {
	switch err {
	case nil:
		return "✅ no"
	case inetutil.ErrTlsHandshakeTimeout, inetutil.ErrTlsHandshakeFail:
		return "❗️detected"
	case webhost.ErrSkip:
		return "⚠️ skip"
	default:
		return resultErrorText(err, fmt.Sprintf("⚠️ %s", err))
	}
}
