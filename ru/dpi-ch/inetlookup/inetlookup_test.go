package inetlookup

import (
	"net/netip"
	"os"
	"slices"
	"testing"
)

var inetlookup InetLookup

func TestMain(m *testing.M) {
	inetlookup = NewGeoliteCsv(GeoliteCsvOpt{
		GeonameidCountryPath: "./testdata/geolite2_csv/geonameId2country_en.csv",
		CidrCountryPath:      "./testdata/geolite2_csv/cidr2countryIso_ipv4.csv",
		CidrAsPath:           "./testdata/geolite2_csv/cidr2as_ipv4.csv",
	})

	code := m.Run()
	os.Exit(code)
}

func Test1(t *testing.T) {
	ip := netip.MustParseAddr("31.44.8.1")
	got := inetlookup.IpInfo(ip)
	want := IpInfo{
		Ip:         ip,
		Asn:        200351,
		Org:        "Yandex.Cloud LLC",
		Subnet:     netip.MustParsePrefix("31.44.8.0/24"),
		CountryIso: "RU",
	}

	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test2(t *testing.T) {
	got := inetlookup.Asns(
		AsnsOpt{
			Ips: []netip.Addr{
				netip.MustParseAddr("31.44.8.24"),
				netip.MustParseAddr("193.186.4.17"),
			},
		})
	want := []int32{15169, 200350, 200351}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test3(t *testing.T) {
	got := inetlookup.Cidrs(CidrsOpt{
		Ips: []netip.Addr{
			netip.MustParseAddr("31.44.8.24"),
			netip.MustParseAddr("193.186.4.17"),
		},
	}).Prefixes()

	want := []netip.Prefix{
		netip.MustParsePrefix("31.44.8.0/21"),
		netip.MustParsePrefix("193.186.4.0/24"),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test4(t *testing.T) {
	got := inetlookup.Cidrs(CidrsOpt{OrgTerms: []string{"google"}}).Prefixes()
	want := []netip.Prefix{
		netip.MustParsePrefix("1.179.112.0/20"),
		netip.MustParsePrefix("2.56.250.0/24"),
		netip.MustParsePrefix("34.0.128.0/19"),
		netip.MustParsePrefix("193.186.4.0/24"),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseMailExternalIp(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{`(none)({"ipAddress": "31.44.8.1", "xForwardedFor": "(none)"})`, "31.44.8.1"},
		{" \n(none)({\"ipAddress\":\"31.44.8.1\",\"xForwardedFor\":\"127.0.0.1\"})\n", "31.44.8.1"},
		{"", ""},
		{`{"ipAddress":"31.44.8.1"}`, ""},
		{`callback({"ipAddress":"31.44.8.1"})`, ""},
		{`(none)({"ipAddress":"31.44.8.1"}`, ""},
		{`(none)()`, ""},
		{`(none)({)`, ""},
		{`(none)({"xForwardedFor":"31.44.8.1"})`, ""},
		{`(none)({"ipAddress":"127.0.0.1","xForwardedFor":"31.44.8.1"})`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			got, err := parseMailExternalIp([]byte(tt.body))
			if tt.want == "" {
				if err == nil || got.IsValid() {
					t.Fatalf("got %v, %v; want zero address and error", got, err)
				}
				return
			}
			if err != nil || got.String() != tt.want {
				t.Fatalf("got %v, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestParseExternalIp(t *testing.T) {
	tests := []struct {
		ip      string
		wantErr bool
	}{
		{"", true},
		{"not an ip", true},
		{"127.0.0.1", true},
		{"127.42.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.0", true},
		{"100.127.255.255", true},
		{"0.0.0.0", true},
		{"224.0.0.1", true},
		{"255.255.255.255", true},
		{"::1", true},
		{"::", true},
		{"fc00::1", true},
		{"fd00::1", true},
		{"fe80::1", true},
		{"ff02::1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:192.168.1.1", true},
		{"::ffff:100.64.0.1", true},
		{"31.44.8.1", false},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"172.15.255.255", false},
		{"172.32.0.0", false},
		{"2001:4860:4860::8888", false},
		{"::ffff:31.44.8.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got, err := parseExternalIp(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got %v, %v; want error: %v", got, err, tt.wantErr)
			}
			if tt.wantErr {
				if got.IsValid() {
					t.Fatalf("got %v on error, want zero address", got)
				}
				return
			}
			if want := netip.MustParseAddr(tt.ip); got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}
