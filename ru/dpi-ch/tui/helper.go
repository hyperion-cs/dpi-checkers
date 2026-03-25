package tui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

func dnsPrettyProviderVerdict(err error) string {
	if httputil.IsHttputilErr(err) {
		return fmt.Sprintf("❗️ %s", err)
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return "⚠️ lookup error"
	}

	switch err {
	case nil:
		return "✅ not detected"
	case ErrPending:
		return "⏰ checking..."
	case checkers.ErrDnsResolveSpoofing:
		return "❗️response spoofing"
	case checkers.ErrDnsDohBootstrapSpoofing:
		return "❗️bootstrap spoofing"
	case checkers.ErrDnsDohBootstrapEmpty:
		return "⚠️ empty bootstrap"
	case checkers.ErrDnsDohInsecure, httputil.ErrTlsCertificateInvalid:
		return "❗️invalid https certificate"
	case checkers.ErrDnsDohNon2xxResp:
		return "⚠️ non-2xx response"
	case checkers.ErrDnsSkip:
		return "⏩ skip"
	default:
		log.Println("dnsPrettyProviderVerdict", err)
		return "⚠️ internal error"
	}
}

func webhostPrettyAlive(err error) string {
	if err == nil {
		return "🟢 yes"
	}

	return fmt.Sprintf("🔴 %s", err)
}

func webhostPrettyTcp1620(err error) string {
	switch err {
	case nil:
		return "✅ no"
	case httputil.ErrTcpWriteTimeout, httputil.ErrTcpReadTimeout:
		return "❗️detected"
	case checkers.ErrWebhostSkip:
		return "⚠️ skip"
	case httputil.ErrTlsWriteBrokenPipe:
		return "⚠️ not supported by host"
	default:
		return fmt.Sprintf("⚠️ %s", err)
	}
}

func countryIsoToFlagEmoji(iso string) string {
	if len(iso) != 2 {
		return ""
	}

	runes := []rune(iso)
	for i := range 2 {
		c := runes[i]
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
