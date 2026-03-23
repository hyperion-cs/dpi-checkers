package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"

	"github.com/charmbracelet/bubbles/table"
)

func webhostPrettyAlive(err error) string {
	if err == nil {
		return "🟢 Yes"
	}

	return fmt.Sprintf("🔴 %s", err)
}

func webhostPrettyTcp1620(err error) string {
	switch err {
	case nil:
		return "✅ No"
	case httputil.ErrTcpWriteTimeout, httputil.ErrTcpReadTimeout:
		return "❗️Detected"
	case checkers.ErrWebhostSkip:
		return "⚠️ Skip"
	case httputil.ErrTlsWriteBrokenPipe:
		return "⚠️ Not supported by host"
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
		if len(v[pos]) > max {
			max = len(v[pos])
		}
	}
	return max
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
