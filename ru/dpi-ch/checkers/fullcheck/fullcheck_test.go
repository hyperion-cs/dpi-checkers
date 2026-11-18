package fullcheck

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/internal/testutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func TestProgressBackpressure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		ch := make(chan Progress, 1)
		ch <- Progress{Msg: "detail"}
		sendProgress(ctx, ch, Progress{Msg: "discardable detail"})
		terminal := Progress{Task: TaskWhoami, State: StateOk}
		done := make(chan struct{})
		go func() {
			sendProgress(ctx, ch, terminal)
			close(done)
		}()
		if canceled {
			cancel()
		} else {
			<-ch
			select {
			case got := <-ch:
				if got != terminal {
					t.Fatalf("got %v, want %v", got, terminal)
				}
			case <-time.After(time.Second):
				cancel()
				t.Fatal("completion was lost with a full buffer")
			}
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("progress sender did not stop")
		}
		cancel()
	}
}

func TestTaskCompletion(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.All.Checkers = []string{"cidrwhitelist", "dns", "webhost"}
	cfg.All.Format = "json"
	cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
	cfg.All.TsFormat = ""
	cfg.Checkers.CidrWhitelist.Regular = nil
	cfg.Checkers.CidrWhitelist.Whitelisted = nil
	cfg.Checkers.Dns.Leak.Times = 0
	cfg.Checkers.Dns.Leak.Workers = 1
	cfg.Checkers.Dns.Resolve.PlainOpt.Workers = 1
	cfg.Checkers.Dns.Resolve.DohOpt.Workers = 1
	cfg.Checkers.Dns.Resolve.Providers = nil
	cfg.Checkers.Webhost.Sections = make([]config.WebhostSection, 40)
	for i := range cfg.Checkers.Webhost.Sections {
		cfg.Checkers.Webhost.Sections[i].Name = "same name"
	}
	cfg.Checkers.Webhost.Sections[1].Targets = []config.WebhostTarget{{Filter: ")"}}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	seen := make(map[[2]int]bool)
	var last Progress
	for p := range Gochan(ctx) {
		last = p
		if p.Task == TaskNone {
			continue
		}
		id := [2]int{int(p.Task), p.Index}
		if seen[id] {
			t.Fatalf("duplicate completion: %v", p)
		}
		seen[id] = true
		want := StateOk
		if p.Task == TaskWebhost && p.Index == 1 {
			want = StateError
		}
		if p.State != want {
			t.Fatalf("got state %v, want %v for %v", p.State, want, id)
		}
	}
	if len(seen) != 44 || last.Task != TaskNone || last.State != StateOk {
		t.Fatalf("unfinished ALL: %d tasks, last event %v", len(seen), last)
	}
}

func TestSaveError(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.All.Checkers = nil
	cfg.All.Format = "invalid"
	cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
	var last Progress
	for p := range Gochan(t.Context()) {
		last = p
	}
	if last.Task != TaskNone || last.State != StateError {
		t.Fatalf("missing save error: %v", last)
	}
}

func TestDnsProviderTimeout(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			addr := testutil.DnsServer(t, "missing.example.")
			cfg := testutil.DnsConfig(t, fmt.Sprintf(`checkers:
  dns:
    resolve:
      targets: [{host: missing.example.}, {host: silent.example.}]
      providers:
        - name: local
          plain: {hosts: [%q]}
`, addr))
			cfg.All.Checkers = []string{"dns"}
			cfg.All.Format = format
			cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
			cfg.All.TsFormat = ""
			cfg.Checkers.Dns.Leak.Times = 0
			cfg.Checkers.Dns.Resolve.PlainOpt.ProviderTimeout = 100 * time.Millisecond
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			seen := false
			for p := range Gochan(ctx) {
				if p.Task == TaskDnsPlain {
					seen = true
					if p.State != StateTimeout {
						t.Errorf("got state %v, want timeout", p.State)
					}
				}
			}
			if !seen || ctx.Err() != nil {
				t.Fatalf("completion received: %v, context: %v", seen, ctx.Err())
			}
			file, err := savePath()
			if err != nil {
				t.Fatal(err)
			}
			report, err := Import(file)
			if err != nil {
				t.Fatal(err)
			}
			statuses := report.Dns.Plain.Providers["local"]
			if len(statuses) != 2 || statuses[0].Code != "NXDOMAIN_SPOOFING" || statuses[1].Code != "PROVIDER_TIMEOUT" ||
				ErrorFromReason(statuses[0].Reason) != dns.ErrNxdomainSpoofing || ErrorFromReason(statuses[1].Reason) != dns.ErrProviderTimeout {
				t.Fatalf("lost DNS findings in report: %v", statuses)
			}
		})
	}
}

func TestCancellationWithFullProgress(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.All.Checkers = []string{"webhost"}
	cfg.Checkers.Webhost.Sections = make([]config.WebhostSection, 40)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	out := Gochan(ctx)
	<-ctx.Done()
	if len(out) != cap(out) {
		t.Fatalf("progress is not full: %d/%d", len(out), cap(out))
	}
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ALL did not close after cancellation with a full progress buffer")
	}
}

func TestWebhostTimeout(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cfg := testutil.WebhostConfig(t)
			cfg.Checkers.Webhost.FarmTimeout = time.Nanosecond
			cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "section"}}
			cfg.All.Checkers = []string{"webhost"}
			cfg.All.Format = format
			cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
			cfg.All.TsFormat = "-2006-01-02_15-04-05.000000000"
			var last Progress
			for progress := range Gochan(t.Context()) {
				last = progress
			}
			file, ok := strings.CutPrefix(last.Msg, "done; saved to ")
			if last.State != StateOk || !ok {
				t.Fatalf("missing saved report path: %v", last)
			}
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			prefix := "{\n  \"Version\": 1,\n"
			if format == "yaml" {
				prefix = "version: 1\n"
			}
			if !bytes.HasPrefix(body, []byte(prefix)) {
				t.Fatalf("report should begin with version 1: %s", body)
			}
			report, err := Import(file)
			if err != nil {
				t.Fatal(err)
			}
			section, ok := report.Webhost["section"]
			if !ok || !section.FarmTimeout || !reflect.DeepEqual(section.Items, []WebhostItemDto{}) {
				t.Fatalf("expected a timed-out section with an empty list of items: %s", body)
			}
		})
	}
}
