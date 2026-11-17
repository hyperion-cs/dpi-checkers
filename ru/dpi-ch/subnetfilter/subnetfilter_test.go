package subnetfilter

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"

	"go4.org/netipx"
)

var _testSubnetfilter *Subnetfilter

func TestMain(m *testing.M) {
	il := inetlookup.NewGeoliteCsv(inetlookup.GeoliteCsvOpt{
		GeonameidCountryPath: "../inetlookup/testdata/geolite2_csv/geonameId2country_en.csv",
		CidrCountryPath:      "../inetlookup/testdata/geolite2_csv/cidr2countryIso_ipv4.csv",
		CidrAsPath:           "../inetlookup/testdata/geolite2_csv/cidr2as_ipv4.csv",
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

	subnets, err := _testSubnetfilter.RunFilter(context.Background(), f)
	if err != nil {
		return nil, err
	}

	return subnets, nil
}

func TestRunFilterCanceled(t *testing.T) {
	filter, err := _testSubnetfilter.CompileFilter(`host("example.invalid")`)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := _testSubnetfilter.RunFilter(ctx, filter); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context canceled", err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		if _, err := _testSubnetfilter.RunFilter(ctx, filter); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want deadline exceeded", err)
		}
	})
}

func TestRunFilterConcurrentContexts(t *testing.T) {
	filter, err := _testSubnetfilter.CompileFilter(`host("192.0.2.1", "2001:db8::1")`)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if _, err := _testSubnetfilter.RunFilter(canceled, filter); !errors.Is(err, context.Canceled) {
				t.Errorf("got %v, want context canceled", err)
			}
		})
		wg.Go(func() {
			ips, err := _testSubnetfilter.RunFilter(t.Context(), filter)
			if err != nil {
				t.Error(err)
				return
			}
			for _, ip := range []string{"192.0.2.1", "2001:db8::1"} {
				if !ips.Contains(netip.MustParseAddr(ip)) {
					t.Errorf("missing %s", ip)
				}
			}
		})
	}
	wg.Wait()
}

func TestExtractHostname(t *testing.T) {
	for _, tt := range []struct {
		filter string
		host   string
	}{
		{`host("example.invalid")`, "example.invalid"},
		{`host("example.invalid", "other.invalid")`, ""},
		{`host("example.invalid") || host("other.invalid")`, ""},
		{`country("ru")`, ""},
	} {
		t.Run(tt.filter, func(t *testing.T) {
			filter, err := _testSubnetfilter.CompileFilter(tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			host, ok := _testSubnetfilter.ExtractHostname(filter)
			if host != tt.host || ok != (tt.host != "") {
				t.Fatalf("got %q, %v, want %q, %v", host, ok, tt.host, tt.host != "")
			}
		})
	}
}
