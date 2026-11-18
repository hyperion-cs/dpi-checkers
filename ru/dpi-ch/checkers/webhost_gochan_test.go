package checkers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	yaml "go.yaml.in/yaml/v3"
)

func webhostTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.Checkers.Webhost.Workers = 1
	cfg.Checkers.Webhost.FarmTimeout = 0
	cfg.Subnetfilter.Workers = 1
	cfg.WebhostFarm.Workers = 1
	cfg.InetlookupGeolitecsv.CidrAs = "../inetlookup/testdata/geolite2_csv/cidr2as_ipv4.csv"
	cfg.InetlookupGeolitecsv.CidrCountry = "../inetlookup/testdata/geolite2_csv/cidr2countryIso_ipv4.csv"
	cfg.InetlookupGeolitecsv.GeonameidCountry = "../inetlookup/testdata/geolite2_csv/geonameId2country_en.csv"
	return cfg
}

func TestWebhostRunnerSummary(t *testing.T) {
	for _, name := range []string{"completed", "canceled", "timeout", "invalid filter"} {
		t.Run(name, func(t *testing.T) {
			cfg := webhostTestConfig(t)
			cfg.Checkers.Webhost.FarmTimeout = time.Hour
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			opt := WebhostGochanRunnerOpt{Ctx: ctx}
			switch name {
			case "canceled":
				cancel()
			case "timeout":
				cfg.Checkers.Webhost.FarmTimeout = time.Nanosecond
			case "invalid filter":
				opt.Targets = []config.WebhostTarget{{Filter: ")"}}
			}
			out := WebhostGochanRunner(opt)
			for range out.Out {
			}
			for range out.Progress {
			}
			select {
			case summary, ok := <-out.Summary:
				if !ok || summary.FarmTimeout != (name == "timeout") {
					t.Fatalf("got %v, open: %v", summary, ok)
				}
			case <-time.After(time.Second):
				t.Fatal("runner did not publish its summary")
			}
			if _, ok := <-out.Summary; ok {
				t.Fatal("runner published more than one summary")
			}
		})
	}
}

func TestWebhostRunnerFilterError(t *testing.T) {
	cfg := webhostTestConfig(t)
	cfg.Checkers.Webhost.FarmTimeout = time.Second
	out := WebhostGochanRunner(WebhostGochanRunnerOpt{
		Ctx: t.Context(),
		Targets: []config.WebhostTarget{
			{Name: "invalid", Filter: "as(true)"},
			{Name: "empty", Filter: "subnet()"},
		},
	})
	for range out.Out {
	}
	for range out.Progress {
	}
	summary := <-out.Summary
	if summary.Err == nil || summary.FarmTimeout {
		t.Fatalf("got %v, want filter error", summary)
	}
}

func TestWebhostRunnerTimeoutWithFullProgress(t *testing.T) {
	cfg := webhostTestConfig(t)
	cfg.Checkers.Webhost.FarmTimeout = time.Second
	cfg.Checkers.Webhost.Workers = 8
	cfg.Subnetfilter.Workers = 8
	cfg.WebhostFarm.Workers = 8
	cfg.InetUtil.Fingerprint = "chrome"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	targets := make([]config.WebhostTarget, 64)
	for i := range targets {
		targets[i] = config.WebhostTarget{
			Name: "local", Filter: `subnet("127.0.0.1/32")`, Port: listener.Addr().(*net.TCPAddr).Port,
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := WebhostGochanRunner(WebhostGochanRunnerOpt{Ctx: ctx, Targets: targets})
	select {
	case summary := <-out.Summary:
		if !summary.FarmTimeout {
			t.Fatal("runner lost its timeout")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner blocked while progress was not consumed")
	}
	if len(out.Progress) != cap(out.Progress) {
		t.Fatalf("progress buffer is not full: %d/%d", len(out.Progress), cap(out.Progress))
	}
	for range out.Out {
	}
	for range out.Progress {
	}
}

func TestFullCheckWebhostTimeout(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cfg := webhostTestConfig(t)
			cfg.Checkers.Webhost.FarmTimeout = time.Nanosecond
			cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "section"}}
			cfg.All.Checkers = []string{"webhost"}
			cfg.All.Format = format
			cfg.All.Prefix = filepath.Join(t.TempDir(), "report")
			cfg.All.TsFormat = "-2006-01-02_15-04-05.000000000"
			var last FullCheckProgress
			for progress := range FullCheckGochan(t.Context()) {
				last = progress
			}
			file, ok := strings.CutPrefix(last.Msg, "done; saved to ")
			if last.State != FullCheckStateOk || !ok {
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
			report, err := FullCheckImport(file)
			if err != nil {
				t.Fatal(err)
			}
			section, ok := report.Webhost["section"]
			if !ok || !section.FarmTimeout || !reflect.DeepEqual(section.Items, []FullCheckWebhostItemDto{}) {
				t.Fatalf("expected a timed-out section with an empty list of items: %s", body)
			}
		})
	}
}

func TestFullCheckWebhostOutputCompatibility(t *testing.T) {
	item := fullCheckWebhostItemDto(WebhostGochanOut[WebhostGochanBag]{
		Bag: WebhostGochanBag{Name: "example"},
		Out: WebhostSingleResult{
			IpInfo: inetlookup.IpInfo{Org: "Example", Asn: 64500, CountryIso: "ZZ", Ip: netip.MustParseAddr("192.0.2.1"), Subnet: netip.MustParsePrefix("192.0.2.0/24")},
			Tls:    &WebhostTls{V: 0x0304, San: []string{"example.test"}, Cn: "example.test"},
		},
	})
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			marshal, unmarshal, key := json.Marshal, json.Unmarshal, "FarmTimeout"
			if format == "yaml" {
				marshal, unmarshal, key = yaml.Marshal, yaml.Unmarshal, "farmtimeout"
			}
			fixture, err := os.ReadFile("testdata/webhost-section." + format)
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := unmarshal(fixture, &want); err != nil {
				t.Fatal(err)
			}
			for _, timeout := range []bool{false, true} {
				body, err := marshal(FullCheckWebhostDto{Items: []FullCheckWebhostItemDto{item}, FarmTimeout: timeout})
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if err := unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got[key] != timeout {
					t.Fatalf("got timeout %v, want %v", got[key], timeout)
				}
				delete(got, key)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("existing report fields changed: %s", body)
				}
			}
		})
	}
}
