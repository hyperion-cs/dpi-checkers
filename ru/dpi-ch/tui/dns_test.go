package tui

import (
	"errors"
	"testing"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
)

func TestDnsProviderTimeoutVerdict(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want string
	}{
		{nil, "✅ not detected"},
		{checkers.ErrDnsProviderTimeout, "❗️timeout"},
		{errors.Join(checkers.ErrDnsResolveSpoofing, checkers.ErrDnsProviderTimeout), "❗️response spoofing, timeout"},
	} {
		if got := dnsPrettyProviderVerdict(tt.err); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
