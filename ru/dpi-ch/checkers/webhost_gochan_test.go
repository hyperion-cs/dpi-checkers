package checkers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
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

func TestWebhostRunnerTimeoutWithFullProgress(t *testing.T) {
	cfg := webhostTestConfig(t)
	cfg.Checkers.Webhost.FarmTimeout = time.Second
	cfg.Checkers.Webhost.Workers = 8
	cfg.Subnetfilter.Workers = 8
	cfg.WebhostFarm.Workers = 8
	targets := make([]config.WebhostTarget, 64)
	for i := range targets {
		targets[i] = config.WebhostTarget{Name: "empty", Filter: "subnet()"}
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
			cfg.All.TsFormat = ""
			for range FullCheckGochan(t.Context()) {
			}
			body, err := os.ReadFile(cfg.All.Prefix + "." + format)
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			key, timeoutKey, itemsKey := "Webhost", "FarmTimeout", "Items"
			if format == "json" {
				err = json.Unmarshal(body, &report)
			} else {
				err = yaml.Unmarshal(body, &report)
				key, timeoutKey, itemsKey = strings.ToLower(key), strings.ToLower(timeoutKey), strings.ToLower(itemsKey)
			}
			if err != nil {
				t.Fatal(err)
			}
			sections := report[key].(map[string]any)
			section, ok := sections["section"].(map[string]any)
			if !ok || section[timeoutKey] != true {
				t.Fatalf("missing timeout in report: %s", body)
			}
			if items, ok := section[itemsKey].([]any); !ok || len(items) != 0 {
				t.Fatalf("got items %v, want empty array", section[itemsKey])
			}
		})
	}
}

func TestFullCheckWebhostOutputCompatibility(t *testing.T) {
	item := FullCheckWebhostItemDto{
		Group: "example", Org: "Example", AS: "AS64500", Location: "ZZ",
		IP: "192.0.2.1", Prefix: "192.0.2.0/24",
		Alive:    FullCheckStatusDto{Msg: "Ok", Code: "OK"},
		Tls:      &FullCheckWebhostTls{V: "1.3", San: []string{"example.test"}, Cn: "example.test"},
		Tcp1620:  FullCheckStatusDto{Msg: "No", Code: "OK"},
		Siberian: FullCheckStatusDto{Msg: "No", Code: "OK"},
	}
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			marshal, unmarshal, key := json.Marshal, json.Unmarshal, "FarmTimeout"
			if format == "yaml" {
				marshal, unmarshal, key = yaml.Marshal, yaml.Unmarshal, "farmtimeout"
			}
			legacy, err := os.ReadFile("testdata/webhost-section." + format)
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := unmarshal(legacy, &want); err != nil {
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
