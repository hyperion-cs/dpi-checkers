package checkers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	yaml "go.yaml.in/yaml/v3"
)

func TestFullCheckImportFormats(t *testing.T) {
	want := FullCheckDto{
		Version: 1,
		Whoami: &FullCheckWhoamiDto{
			Status: FullCheckStatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
			Ip:     "192.0.2.17", Subnet: "192.0.2.0/24", Asn: "AS64496",
			Org: "Example Network", Location: "ZZ", TtlbMs: 42,
		},
		Dns: &FullCheckDnsDto{
			Plain: &FullCheckDnsReportDto{
				Status:    FullCheckStatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
				Providers: map[string][]FullCheckStatusDto{"saved resolver": {{Code: "OK", Msg: "Ok", Reason: "NONE"}}},
			},
			Doh: &FullCheckDnsReportDto{
				Providers: map[string][]FullCheckStatusDto{"future resolver": {{Code: "FUTURE_CODE", Msg: "saved explanation", Reason: "FUTURE_REASON"}}},
			},
		},
		Webhost: map[string]FullCheckWebhostDto{"saved section": {
			FarmTimeout: true,
			Items: []FullCheckWebhostItemDto{{
				Group: "example", IP: "198.51.100.7", Prefix: "198.51.100.0/24",
				Alive: FullCheckStatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"},
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
			got, err := FullCheckImport(file)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Fatalf("got %#v, want %#v", *got, want)
			}
		})
	}
}

func TestFullCheckStatusConversions(t *testing.T) {
	for _, pair := range []struct {
		tcp, tls error
	}{
		{inetutil.ErrTcpReadTimeout, inetutil.ErrTlsHandshakeTimeout},
		{inetutil.ErrTcpWriteTimeout, inetutil.ErrTlsHandshakeFail},
	} {
		item := fullCheckWebhostItemDto(WebhostGochanOut[WebhostGochanBag]{Out: WebhostSingleResult{
			Tcp1620: pair.tcp, Siberian: pair.tls,
		}})
		if item.Tcp1620.Code != "DETECTED" || item.Siberian.Code != "DETECTED" ||
			FullCheckErrorFromReason(item.Tcp1620.Reason) != pair.tcp || FullCheckErrorFromReason(item.Siberian.Reason) != pair.tls {
			t.Fatalf("lost Webhost causes: %+v, %+v", item.Tcp1620, item.Siberian)
		}
	}
	for _, tt := range []struct {
		status FullCheckStatusDto
		want   FullCheckStatusDto
		err    error
	}{
		{fullCheckWhoamiDto(WhoamiResult{}, nil).Status, FullCheckStatusDto{Code: "OK", Msg: "Ok", Reason: "NONE"}, nil},
		{fullCheckWhoamiDto(WhoamiResult{}, errors.New("local failure")).Status, FullCheckStatusDto{Code: "ERR", Msg: "local failure", Reason: "UNKNOWN"}, ErrFullCheckUnknownReason},
		{fullCheckCidrwhitelistDto(ErrCidrWhitelistNoInetAccess).Status, FullCheckStatusDto{
			Code: "NO_INTERNET_ACCESS", Msg: "It seems that there is no Internet access (even to resources from the whitelist)",
			Reason: "CHECKERS_ERR_CIDR_WHITELIST_NO_INET_ACCESS",
		}, ErrCidrWhitelistNoInetAccess},
	} {
		if tt.status != tt.want || FullCheckErrorFromReason(tt.status.Reason) != tt.err {
			t.Errorf("got status %+v, want %+v and reason %v", tt.status, tt.want, tt.err)
		}
	}
}

func TestFullCheckDnsNon2xxReason(t *testing.T) {
	single := fullCheckPrettyDnsVerdict(errors.Join(ErrDnsDohNon2xxResp))
	want := []FullCheckStatusDto{{Code: "OK", Msg: "Ok", Reason: "CHECKERS_ERR_DNS_DOH_NON_2XX_RESP"}}
	if !reflect.DeepEqual(single, want) || FullCheckErrorFromReason(single[0].Reason) != ErrDnsDohNon2xxResp {
		t.Fatalf("got %+v, want %+v and the original DoH reason", single, want)
	}
	combined := fullCheckPrettyDnsVerdict(errors.Join(ErrDnsDohNon2xxResp, ErrDnsProviderTimeout))
	want = []FullCheckStatusDto{
		{Code: "NON_2XX_RESP", Msg: "Non-2xx response", Reason: "CHECKERS_ERR_DNS_DOH_NON_2XX_RESP"},
		{Code: "PROVIDER_TIMEOUT", Msg: "Provider timeout", Reason: "CHECKERS_ERR_DNS_PROVIDER_TIMEOUT"},
	}
	if !reflect.DeepEqual(combined, want) {
		t.Fatalf("got %+v, want %+v", combined, want)
	}
}

func TestFullCheckImportPartial(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want FullCheckDto
	}{
		{"dns-only", `{"Version":1,"Dns":{"Doh":{"Providers":{}}}}`, FullCheckDto{
			Dns: &FullCheckDnsDto{Doh: &FullCheckDnsReportDto{Providers: map[string][]FullCheckStatusDto{}}},
		}},
		{"empty-timeout", "version: 1\nwebhost:\n  saved:\n    farmtimeout: true\n    items: []\n", FullCheckDto{
			Webhost: map[string]FullCheckWebhostDto{"saved": {Items: []FullCheckWebhostItemDto{}, FarmTimeout: true}},
		}},
		{"webhost-with-null-section", `{"Version":1,"Webhost":{"missing":null,"timed out":{"FarmTimeout":true}}}`, FullCheckDto{
			Webhost: map[string]FullCheckWebhostDto{"timed out": {FarmTimeout: true}},
		}},
		{"empty-webhost", `{"Version":1,"Webhost":{"saved":{"Items":[]}}}`, FullCheckDto{
			Webhost: map[string]FullCheckWebhostDto{"saved": {Items: []FullCheckWebhostItemDto{}}},
		}},
		{"missing-reason", `{"Version":1,"CidrWhitelist":{"Status":{"Code":"NOT_DETECTED"}}}`, FullCheckDto{
			CidrWhitelist: &FullCheckCidrwhitelistDto{Status: FullCheckStatusDto{Code: "NOT_DETECTED"}},
		}},
		{"yaml-trailing-newlines", "version: 1\nwhoami:\n  status:\n    code: ERR\n    reason: UNKNOWN\n    msg: |+\n      saved error\n\n", FullCheckDto{
			Whoami: &FullCheckWhoamiDto{Status: FullCheckStatusDto{Code: "ERR", Msg: "saved error\n\n", Reason: "UNKNOWN"}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "report")
			if err := os.WriteFile(file, []byte(tt.raw), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := FullCheckImport(file)
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

func TestFullCheckImportVersion(t *testing.T) {
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
		if report, err := FullCheckImport(file); report != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("import %q: got %v, want %q", tt.raw, err, tt.want)
		}
	}
}

func TestFullCheckImportRejectsInvalid(t *testing.T) {
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
			if got, err := FullCheckImport(file); err == nil || got != nil {
				t.Fatalf("accepted invalid report: %#v, error: %v", got, err)
			}
		})
	}
	if _, err := FullCheckImport(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want missing file error", err)
	}
}
