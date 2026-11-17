package checkers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func TestFullCheckProgressBackpressure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		ch := make(chan FullCheckProgress, 1)
		ch <- FullCheckProgress{Msg: "detail"}
		fullCheckSendProgress(ctx, ch, FullCheckProgress{Msg: "discardable detail"})
		terminal := FullCheckProgress{Task: FullCheckTaskWhoami, State: FullCheckStateOk}
		done := make(chan struct{})
		go func() {
			fullCheckSendProgress(ctx, ch, terminal)
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

func TestFullCheckTaskCompletion(t *testing.T) {
	cfg := webhostTestConfig(t)
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
	var last FullCheckProgress
	for p := range FullCheckGochan(ctx) {
		last = p
		if p.Task == FullCheckTaskNone {
			continue
		}
		id := [2]int{int(p.Task), p.Index}
		if seen[id] {
			t.Fatalf("duplicate completion: %v", p)
		}
		seen[id] = true
		want := FullCheckStateOk
		if p.Task == FullCheckTaskWebhost && p.Index == 1 {
			want = FullCheckStateError
		}
		if p.State != want {
			t.Fatalf("got state %v, want %v for %v", p.State, want, id)
		}
	}
	if len(seen) != 44 || last.Task != FullCheckTaskNone || last.State != FullCheckStateOk {
		t.Fatalf("unfinished ALL: %d tasks, last event %v", len(seen), last)
	}
}

func TestFullCheckSaveError(t *testing.T) {
	cfg := webhostTestConfig(t)
	cfg.All.Checkers = nil
	cfg.All.Format = "invalid"
	cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
	var last FullCheckProgress
	for p := range FullCheckGochan(t.Context()) {
		last = p
	}
	if last.Task != FullCheckTaskNone || last.State != FullCheckStateError {
		t.Fatalf("missing save error: %v", last)
	}
}

func TestFullCheckCancellationWithFullProgress(t *testing.T) {
	cfg := webhostTestConfig(t)
	cfg.All.Checkers = []string{"webhost"}
	cfg.Checkers.Webhost.Sections = make([]config.WebhostSection, 40)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	out := FullCheckGochan(ctx)
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
