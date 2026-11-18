package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
)

func TestDnsProviderTimeoutVerdict(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want string
	}{
		{nil, "✅ not detected"},
		{dns.ErrProviderTimeout, "❗️timeout"},
		{errors.Join(dns.ErrResolveSpoofing, dns.ErrProviderTimeout), "❗️response spoofing, timeout"},
	} {
		if got := dnsPrettyProviderVerdict(tt.err, false); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestDnsCompletionClearsPendingVerdicts(t *testing.T) {
	model := newDnsModel()
	model.fetching = true
	verdict := errors.Join(dns.ErrResolveSpoofing, dns.ErrProviderTimeout)
	model.processPlainProvider(dnsProviderPlainMsg{Provider: "example resolver", Verdict: verdict})
	if model.providerRows["example resolver"].plainVerdict != verdict {
		t.Fatal("live DNS model replaced its original verdict")
	}
	if got := model.progress; got != "[example resolver] plain: ❗️response spoofing, timeout" {
		t.Fatalf("progress did not update with the verdict: %q", got)
	}
	if !strings.Contains(model.providerTable.Rows()[0][2], "checking") {
		t.Fatal("missing provider result was not pending")
	}
	cmd := model.Update(dnsProducerDoneMsg{})
	if model.fetching || cmd != nil || model.providerTable.Rows()[0][2] != "—" {
		t.Fatal("completed DNS table still shows a pending check")
	}
}

func TestDnsLeakKeepsPartialResults(t *testing.T) {
	model := newDnsModel()
	model.fetching = true
	model.Update(dnsLeakMsg{})
	err := errors.New("dns: leak lookup failed")
	model.Update(dnsLeakMsg{Err: err})
	model.Update(dnsLeakMsg{Items: []inetlookup.IpInfoStrings{{Ip: "192.0.2.1"}}})
	model.Update(dnsProducerDoneMsg{})
	if model.leakTable.Cursor() != 0 || len(model.leakTable.Rows()) != 1 {
		t.Fatal("first leak result was not selected")
	}
	if model.leakErr != err || !strings.Contains(model.View(), "⚠️ "+err.Error()) {
		t.Fatal("later leak results hid an earlier error")
	}
}
