package fullcheck

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	yaml "go.yaml.in/yaml/v3"
)

func TestImportFormats(t *testing.T) {
	want := ReportDto{
		Version: 1,
		Whoami: &WhoamiDto{
			Status: StatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
			Ip:     "192.0.2.17", Subnet: "192.0.2.0/24", Asn: "AS64496",
			Org: "Example Network", Location: "ZZ", TtlbMs: 42,
		},
		Dns: &DnsDto{
			Plain: &DnsReportDto{
				Status:    StatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
				Providers: map[string][]StatusDto{"saved resolver": {{Code: "OK", Msg: "Ok", Reason: "NONE"}}},
			},
			Doh: &DnsReportDto{
				Providers: map[string][]StatusDto{"future resolver": {{Code: "FUTURE_CODE", Msg: "saved explanation", Reason: "FUTURE_REASON"}}},
			},
		},
		Webhost: map[string]WebhostDto{"saved section": {
			FarmTimeout: true,
			Items: []WebhostItemDto{{
				Group: "example", IP: "198.51.100.7", Prefix: "198.51.100.0/24",
				Alive: StatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
			}},
		}},
	}
	jsonRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	yamlRaw, err := yaml.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		raw  []byte
	}{
		{"json.yml", jsonRaw},
		{"yaml.json", yamlRaw},
		{"json-without-extension", jsonRaw},
		{"yaml-without-extension", yamlRaw},
		{"json-bom", append([]byte{0xef, 0xbb, 0xbf}, jsonRaw...)},
		{"yaml-bom", append([]byte{0xef, 0xbb, 0xbf}, yamlRaw...)},
		{"json-future", append(append([]byte{}, jsonRaw[:len(jsonRaw)-1]...), []byte(",\"FutureField\":{\"value\":42}}")...)},
		{"yaml-future", append(append([]byte{}, yamlRaw...), []byte("\nfuturefield:\n  value: 42\n")...)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tt.name)
			if err := os.WriteFile(file, tt.raw, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Import(file)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Fatalf("got %#v, want %#v", *got, want)
			}
		})
	}
}

func TestStatusConversions(t *testing.T) {
	for _, pair := range []struct {
		tcp, tls error
	}{
		{inetutil.ErrTcpReadTimeout, inetutil.ErrTlsHandshakeTimeout},
		{inetutil.ErrTcpWriteTimeout, inetutil.ErrTlsHandshakeFail},
	} {
		item := webhostItemDto(webhost.Item{Result: webhost.SingleResult{
			Tcp1620: pair.tcp, Siberian: pair.tls,
		}})
		if item.Tcp1620.Code != "DETECTED" || item.Siberian.Code != "DETECTED" ||
			ErrorFromReason(item.Tcp1620.Reason) != pair.tcp || ErrorFromReason(item.Siberian.Reason) != pair.tls {
			t.Fatalf("lost Webhost causes: %+v, %+v", item.Tcp1620, item.Siberian)
		}
	}
	for _, tt := range []struct {
		status StatusDto
		want   StatusDto
		err    error
	}{
		{whoamiDto(whoami.Result{}, nil).Status, StatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"}, nil},
		{whoamiDto(whoami.Result{}, errors.New("fullcheck: local failure")).Status,
			StatusDto{Code: "ERR", Msg: "fullcheck: local failure", Reason: "UNKNOWN"}, ErrUnknownReason},
		{webhostAliveStatus(webhost.ErrBlockedBySni),
			StatusDto{Code: "ERR", Msg: "TLS blocked by SNI", Reason: "CHECKERS_WEBHOST_ERR_BLOCKED_BY_SNI"}, webhost.ErrBlockedBySni},
		{cidrWhitelistDto(cidrwhitelist.ErrNoInetAccess).Status, StatusDto{
			Code: "NO_INTERNET_ACCESS", Msg: "It seems that there is no Internet access (even to resources from the whitelist)",
			Reason: "CHECKERS_CIDRWHITELIST_ERR_NO_INET_ACCESS",
		}, cidrwhitelist.ErrNoInetAccess},
	} {
		if tt.status != tt.want || ErrorFromReason(tt.status.Reason) != tt.err {
			t.Errorf("got status %+v, want %+v and reason %v", tt.status, tt.want, tt.err)
		}
	}
}

func TestDnsNon2xxReason(t *testing.T) {
	single := dnsStatuses(errors.Join(dns.ErrDohNon2xxResp))
	want := []StatusDto{{Code: "OK", Msg: "Ok", Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"}}
	if !reflect.DeepEqual(single, want) || ErrorFromReason(single[0].Reason) != dns.ErrDohNon2xxResp {
		t.Fatalf("got %+v, want %+v and the original DoH reason", single, want)
	}
	combined := dnsStatuses(errors.Join(dns.ErrDohNon2xxResp, dns.ErrProviderTimeout))
	want = []StatusDto{
		{Code: "NON_2XX_RESP", Msg: "Non-2xx response", Reason: "CHECKERS_DNS_ERR_DOH_NON_2XX_RESP"},
		{Code: "PROVIDER_TIMEOUT", Msg: "Provider timeout", Reason: "CHECKERS_DNS_ERR_PROVIDER_TIMEOUT"},
	}
	if !reflect.DeepEqual(combined, want) {
		t.Fatalf("got %+v, want %+v", combined, want)
	}
}

func TestImportPartial(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want ReportDto
	}{
		{"dns-only", `{"Version":1,"Dns":{"Doh":{"Providers":{}}}}`, ReportDto{
			Dns: &DnsDto{Doh: &DnsReportDto{Providers: map[string][]StatusDto{}}},
		}},
		{"empty-timeout", "version: 1\nwebhost:\n  saved:\n    farmtimeout: true\n    items: []\n", ReportDto{
			Webhost: map[string]WebhostDto{"saved": {Items: []WebhostItemDto{}, FarmTimeout: true}},
		}},
		{"webhost-with-null-section", `{"Version":1,"Webhost":{"missing":null,"timed out":{"FarmTimeout":true}}}`, ReportDto{
			Webhost: map[string]WebhostDto{"timed out": {FarmTimeout: true}},
		}},
		{"empty-webhost", `{"Version":1,"Webhost":{"saved":{"Items":[]}}}`, ReportDto{
			Webhost: map[string]WebhostDto{"saved": {Items: []WebhostItemDto{}}},
		}},
		{"missing-reason", `{"Version":1,"CidrWhitelist":{"Status":{"Code":"NOT_DETECTED"}}}`, ReportDto{
			CidrWhitelist: &CidrWhitelistDto{Status: StatusDto{Code: "NOT_DETECTED"}},
		}},
		{"yaml-trailing-newlines", "version: 1\nwhoami:\n  status:\n    code: ERR\n    reason: UNKNOWN\n    msg: |+\n      saved error\n\n", ReportDto{
			Whoami: &WhoamiDto{Status: StatusDto{Code: "ERR", Msg: "saved error\n\n", Reason: "UNKNOWN"}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "report")
			if err := os.WriteFile(file, []byte(tt.raw), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Import(file)
			if err != nil {
				t.Fatal(err)
			}
			tt.want.Version = 1
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("got %#v, want %#v", *got, tt.want)
			}
		})
	}
}

func TestImportVersion(t *testing.T) {
	for _, tt := range []struct {
		raw, want string
	}{
		{`{"Whoami":{"Status":{"Code":"OK"}}}`, "unversioned"},
		{"whoami: {}\n", "unversioned"},
		{`{"Version":2,"Whoami":{}}`, "unsupported ALL report version 2"},
		{"version: 2\nwhoami: {}\n", "unsupported ALL report version 2"},
		{"version: 1.5\nwhoami: {}\n", "version must be an integer"},
	} {
		file := filepath.Join(t.TempDir(), "report")
		if err := os.WriteFile(file, []byte(tt.raw), 0600); err != nil {
			t.Fatal(err)
		}
		if report, err := Import(file); report != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("import %q: got %v, want %q", tt.raw, err, tt.want)
		}
	}
}

func TestImportRejectsInvalid(t *testing.T) {
	for _, raw := range []string{
		"", " \n", "null", "[]", "42", "text", "{}", "unknown: value",
		`{"Version":1,"Unknown":true}`, `{"Version":1,"Dns":{}}`, `{"Version":1,"Dns":{"Plain":null,"Doh":null,"Leak":null}}`, `{"Version":1,"Webhost":{}}`,
		"version: 1\nwebhost: {missing: null}",
		`{"Whoami":`, "whoami:\n  status: [", `{"Whoami":[]}`, `{"Dns":{"Plain":true}}`,
		`{"Webhost":{"saved":[]}}`, `{"Whoami":{}} {"Whoami":{}}`,
		"version: 1\nwhoami: {}\n---\nwhoami: {}\n", "version: 1\nwhoami: {}\n---\n",
	} {
		t.Run(raw, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "report")
			if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if got, err := Import(file); err == nil || got != nil {
				t.Fatalf("accepted invalid report: %#v, error: %v", got, err)
			}
		})
	}
	if _, err := Import(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want missing file error", err)
	}
}

func TestWebhostOutputCompatibility(t *testing.T) {
	item := webhostItemDto(webhost.Item{
		Group: "example",
		Result: webhost.SingleResult{
			IpInfo: inetlookup.IpInfo{Org: "Example", Asn: 64500, CountryIso: "ZZ", Ip: netip.MustParseAddr("192.0.2.1"), Subnet: netip.MustParsePrefix("192.0.2.0/24")},
			Tls:    &webhost.Tls{V: 0x0304, San: []string{"example.test"}, Cn: "example.test"},
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
				body, err := marshal(WebhostDto{Items: []WebhostItemDto{item}, FarmTimeout: timeout})
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
