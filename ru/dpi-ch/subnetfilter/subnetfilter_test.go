package subnetfilter

import (
	"dpich/inetlookup"
	"net/netip"
	"os"
	"slices"
	"testing"

	"go4.org/netipx"
)

var _testSubnetfilter *Subnetfilter

func TestMain(m *testing.M) {
	il := inetlookup.NewGeoliteCsv(inetlookup.GeoliteCsvOpt{
		GeonameId2countryIsoPath: "../inetlookup/testdata/geolite2_csv/geonameId2country_en.csv",
		Cidr2countryIsoPath:      "../inetlookup/testdata/geolite2_csv/cidr2countryIso_ipv4.csv",
		Cidr2asPath:              "../inetlookup/testdata/geolite2_csv/cidr2as_ipv4.csv",
	})
	_testSubnetfilter = New(il)

	code := m.Run()
	os.Exit(code)
}

func Test1(t *testing.T) {
	subnets, _ := compileAndRunFilter(`country("ru")`)
	got := subnets.Prefixes()
	want := []netip.Prefix{
		netip.MustParsePrefix("31.44.8.0/21"),
		netip.MustParsePrefix("37.9.64.0/24"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

}
func Test2(t *testing.T) {
	subnets, _ := compileAndRunFilter(`country("us", "au")`)
	got := subnets.Prefixes()
	want := []netip.Prefix{
		netip.MustParsePrefix("1.0.0.0/24"),
		netip.MustParsePrefix("34.0.128.0/19"),
		netip.MustParsePrefix("68.169.48.0/20"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test3(t *testing.T) {
	subnets, _ := compileAndRunFilter(`org("google")`)
	got := subnets.Prefixes()
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
func Test4(t *testing.T) {
	subnets, _ := compileAndRunFilter(`org("yandex")`)
	got := subnets.Prefixes()
	want := []netip.Prefix{
		netip.MustParsePrefix("5.45.192.0/18"),
		netip.MustParsePrefix("31.44.8.0/21"),
		netip.MustParsePrefix("37.9.64.0/24"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
func Test5(t *testing.T) {

	subnets, _ := compileAndRunFilter(`org("yandex") && country("ru")`)
	got := subnets.Prefixes()
	want := []netip.Prefix{
		netip.MustParsePrefix("31.44.8.0/21"),
		netip.MustParsePrefix("37.9.64.0/24"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test6(t *testing.T) {
	cidr := "192.168.0.1/32"
	subnets, err := compileAndRunFilter(`subnet("` + cidr + `")`)
	if err != nil {
		t.Fatal(err)
	}
	p := subnets.Prefixes()

	got1 := len(p)
	want1 := 1
	if got1 != want1 {
		t.Fatalf("got %d prefix, want %d", got1, want1)
	}

	got2 := p[0].String()
	want2 := cidr
	if got2 != want2 {
		t.Fatalf("got %s cidr, want %s", got2, want2)
	}
}

func compileAndRunFilter(filter string) (*netipx.IPSet, error) {
	f, err := _testSubnetfilter.CompileFilter(filter)
	if err != nil {
		return nil, err
	}

	subnets, err := _testSubnetfilter.RunFilter(f)
	if err != nil {
		return nil, err
	}

	return subnets, nil
}
