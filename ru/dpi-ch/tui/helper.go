package tui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"golang.org/x/net/publicsuffix"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const unknownResultText = "⚠️ Unknown"

var (
	KM_UP    = []string{"up", "k", "л", "ctrl+p", "ctrl+з"}
	KM_DOWN  = []string{"down", "j", "о", "ctrl+n", "ctrl+т"}
	KM_LEFT  = []string{"left", "h", "р", "ctrl+b", "ctrl+и"}
	KM_RIGHT = []string{"right", "l", "д", "ctrl+f", "ctrl+а"}
)

func dnsPrettyProviderVerdict(err error, fetching bool) string {
	if err == nil {
		return "✅ not detected"
	}

	if errors.Is(err, ErrPending) {
		if fetching {
			return "⏰ checking..."
		}
		return "—"
	}

	dnsErrs := checkers.DnsErrors(err)
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
	if errors.Is(err, checkers.ErrFullCheckUnknownReason) {
		items = append(items, unknownResultText)
	}
	// TODO: disable this when HTTP/2 is supported.
	// It only works as long as we trust providers that use DoH.
	if len(items) == 1 && len(dnsErrs) == 1 && dnsErrs[0] == checkers.ErrDnsDohNon2xxResp {
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
		case checkers.ErrDnsProviderHijacking:
			items = append(items, "provider hijacking")
		case checkers.ErrDnsResolveSpoofing:
			items = append(items, "response spoofing")
		case checkers.ErrDnsNxdomainSpoofing:
			items = append(items, "nxdomain spoofing")
		case checkers.ErrDnsDohBootstrapSpoofing:
			items = append(items, "bootstrap spoofing")
		case checkers.ErrDnsDohBootstrapEmpty:
			items = append(items, "empty bootstrap")
		case checkers.ErrDnsDohInsecure:
			items = append(items, "invalid https certificate")
		case checkers.ErrDnsDohNon2xxResp:
			items = append(items, "non-2xx response")
		case checkers.ErrDnsSkip:
			items = append(items, "skip")
		case checkers.ErrDnsProviderTimeout:
			items = append(items, "timeout")
		}
	}

	prefix := "❗️"
	switch errs[0] {
	case checkers.ErrDnsDohBootstrapEmpty, checkers.ErrDnsDohNon2xxResp:
		prefix = "⚠️ "
	case checkers.ErrDnsSkip:
		prefix = "⏩ "
	}
	return prefix + strings.Join(items, ", ")
}

func webhostPrettyAlive(err error) string {
	switch err {
	case nil:
		return "🟢 yes"
	case inetutil.ErrHttpMalformedResponse:
		return "🟢 custom http"
	case checkers.ErrFullCheckUnknownReason, inetutil.ErrTlsInvalidKeyShare, inetutil.ErrTlsBadRecordMac:
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
	case checkers.ErrWebhostSkip:
		return "⚠️ skip"
	case inetutil.ErrTlsWriteBrokenPipe:
		return "⚠️ not supported by host"
	default:
		return fmt.Sprintf("⚠️ %s", err)
	}
}

func webhostPrettySanCn(san []string, cn string) string {
	const sub, max = 15, 25
	x := inetutil.TlsSanCn(san, cn)
	res := x.Name
	if res == "" {
		res = "—"
	}

	if len(x.Name) > max {
		if name, err := publicsuffix.EffectiveTLDPlusOne(x.Name); err == nil && len(name) <= max {
			res = "///." + name
		} else {
			res = "///" + res[len(res)-sub:]
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
	case checkers.ErrWebhostSkip:
		return "⚠️ skip"
	default:
		return fmt.Sprintf("⚠️ %s", err)
	}
}

func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}

func countryIsoToFlagEmoji(iso string) string {
	if len(iso) != 2 {
		return ""
	}

	runes := []rune(iso)
	for i, c := range runes {
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if c < 'A' || c > 'Z' {
			return ""
		}
		runes[i] = rune(0x1F1E6 + (c - 'A'))
	}

	return string(runes)
}

func tableCellMaxLen(rows []table.Row, pos, min int) int {
	max := min
	for _, v := range rows {
		w := lipgloss.Width(v[pos])
		if w > max {
			max = w
		}
	}
	return max
}

func tableHeight(rows []table.Row, maxVisibleRows int) int {
	const extraHeight = 2 // internal table extra height
	return min(maxVisibleRows, len(rows)) + extraHeight
}

func tableWidth(cols []table.Column) int {
	const extraWidth = 2 // internal column extra width
	width := extraWidth * len(cols)
	for _, col := range cols {
		width += col.Width
	}
	return width
}

func isTimeoutErr(err error) bool {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, os.ErrDeadlineExceeded) {
			return true
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return true
		}
	}
	return false
}

// Normalizes control keys. Supports vim-style key bindings.
func normKey(s string) string {
	switch {
	case slices.Contains(KM_UP, s):
		return "up"
	case slices.Contains(KM_DOWN, s):
		return "down"
	case slices.Contains(KM_LEFT, s):
		return "left"
	case slices.Contains(KM_RIGHT, s):
		return "right"
	}

	return s
}

func tableKeyMap() table.KeyMap {
	km := table.DefaultKeyMap()

	km.LineUp = key.NewBinding(
		key.WithKeys(KM_UP...),
		key.WithHelp("↑/k", "up"),
	)

	km.LineDown = key.NewBinding(
		key.WithKeys(KM_DOWN...),
		key.WithHelp("↓/j", "down"),
	)

	km.HalfPageDown = key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "½ page down"),
	)

	return km
}
