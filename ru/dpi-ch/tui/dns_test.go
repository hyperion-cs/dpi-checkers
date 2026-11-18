package tui

import (
	"errors"
	"strings"
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
		if got := dnsPrettyProviderVerdict(tt.err, false); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestDnsCompletionClearsPendingVerdicts(t *testing.T) {
	importTestConfig(t)
	model := dnsInitModel()
	model.fetching = true
	model = dnsProcessPlainProvider(dnsProviderPlainMsg{Provider: "example resolver"}, model)
	if !strings.Contains(model.providerTable.Rows()[0][2], "checking") {
		t.Fatal("missing provider result was not pending")
	}
	model, cmd := dnsUpdate(model, msgFor(model.session, dnsProducerDoneMsg{}))
	if model.fetching || cmd != nil || model.providerTable.Rows()[0][2] != "—" {
		t.Fatal("completed DNS table still shows a pending check")
	}
}
