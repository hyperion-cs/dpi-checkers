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
		GeonameId2countryIsoPath: "./testdata/geolite2_csv/geonameId2country_en.csv",
		Cidr2countryIsoPath:      "./testdata/geolite2_csv/cidr2countryIso_ipv4.csv",
		Cidr2asPath:              "./testdata/geolite2_csv/cidr2as_ipv4.csv",
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
