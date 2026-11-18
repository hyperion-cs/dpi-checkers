package primitives

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const DotChar = " • "

func TerminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}

func CountryIsoToFlagEmoji(iso string) string {
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
