package screens

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func TestWebhostTypedTimeout(t *testing.T) {
	cfg := &config.Get().Checkers.Webhost
	farmTimeout := cfg.FarmTimeout
	t.Cleanup(func() { cfg.FarmTimeout = farmTimeout })
	cfg.FarmTimeout = 47 * time.Second

	progress := make(chan string, 16)
	for range cap(progress) {
		progress <- "farming timeout appears in ordinary text"
	}
	close(progress)
	items := make(chan webhost.Item)
	close(items)
	summary := make(chan webhost.Summary, 1)
	summary <- webhost.Summary{FarmTimeout: true}
	close(summary)
	screen, _ := openWebhost(t.Context(), nil)
	model := screen.(*webhostModel)
	cmd := model.Update(webhostProducerStartedMsg{out: webhost.RunOut{Out: items, Progress: progress, Summary: summary}})
	for i := 0; i <= cap(progress); i++ {
		msg := cmd()
		cmd = model.Update(msg)
		if i < cap(progress) && model.farmTimeout {
			t.Fatal("progress text set the timeout flag")
		}
	}
	if model.fetching || !model.farmTimeout || cmd != nil {
		t.Fatalf("missing final timeout: fetching=%v, timeout=%v", model.fetching, model.farmTimeout)
	}
	if got := model.View(); !strings.Contains(got, "farming timeout exceeded (47s)") {
		t.Fatalf("live timeout lost its configured duration: %s", got)
	}
	model.Update(webhostProducerDoneMsg{})
	if model.farmTimeout {
		t.Fatal("normal completion retained the timeout flag")
	}
}

func TestWebhostSummaryErrorWithPartialResults(t *testing.T) {
	visibleRows := &config.Get().Checkers.Webhost.TableMaxVisibleRows
	saved := *visibleRows
	t.Cleanup(func() { *visibleRows = saved })
	*visibleRows = 1

	model := newWebhostModel()
	model.Update(webhostItemMsg{Group: "partial"})
	err := fmt.Errorf("\x1b[31m%w\x1b[0m\ninvalid filter", webhost.ErrInternal)
	model.Update(webhostProducerDoneMsg{FarmTimeout: true, Err: err})

	const want = "⏰ farming timeout exceeded\n⚠️ error: webhost: internal error invalid filter\ncount: 1 pcs."
	if got := model.View(); !strings.Contains(got, "partial") || !strings.Contains(got, want) {
		t.Fatalf("missing partial row, final error, or timeout: %q", got)
	}
}

func TestWebhostRendersTypedTlsErrors(t *testing.T) {
	item := webhostItemMsg{Result: webhost.SingleResult{Alive: inetutil.ErrTlsBadRecordMac}}
	model := newWebhostModel()
	model.Update(item)
	if got := model.table.Rows()[0][6]; !strings.HasPrefix(got, "⚠️ ") {
		t.Fatalf("typed TLS warning = %q", got)
	}
	if got := webhostPrettyAlive(webhost.ErrBlockedBySni); got != "🔴 tls: blocked by sni" {
		t.Errorf("SNI block message = %q", got)
	}
}

func TestWebhostShortensUnicodeCertificateName(t *testing.T) {
	cn := strings.Repeat("例", 26) + ".com"
	want := "///" + strings.Repeat("例", 11) + ".com"
	if got := webhostPrettySanCn(nil, cn); got != want {
		t.Errorf("shortened certificate name = %q, want %q", got, want)
	}
}
