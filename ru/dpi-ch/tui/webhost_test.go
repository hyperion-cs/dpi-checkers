package tui

import (
	"testing"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
)

func TestWebhostTypedTimeout(t *testing.T) {
	s := &session{}
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
	model := webhostModel{
		session: s, inited: true, fetching: true,
		out: webhost.RunOut{Out: items, Progress: progress, Summary: summary},
	}
	cmd := webhostConsumerCmd(s, model.out)
	for i := 0; i <= cap(progress); i++ {
		msg := cmd()
		model, cmd = webhostUpdate(model, msg)
		if i < cap(progress) && model.farmTimeout {
			t.Fatal("progress text set the timeout flag")
		}
	}
	if model.fetching || !model.farmTimeout || cmd != nil {
		t.Fatalf("missing final timeout: fetching=%v, timeout=%v", model.fetching, model.farmTimeout)
	}
	model, _ = webhostUpdate(model, msgFor(s, webhostProducerDoneMsg{}))
	if model.farmTimeout {
		t.Fatal("normal completion retained the timeout flag")
	}
}
