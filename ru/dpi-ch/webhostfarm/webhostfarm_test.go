package webhostfarm

import (
	"net/netip"
	"testing"

	"go4.org/netipx"
)

func Test1(t *testing.T) {
	p := netip.MustParsePrefix("192.168.0.0/16")
	r := netipx.RangeOfPrefix(p)
	got := iprangeTotal(r)
	want := uint64(1 << 16)
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test2(t *testing.T) {
	var b netipx.IPSetBuilder
	b.AddPrefix(netip.MustParsePrefix("192.168.0.0/16"))
	b.AddPrefix(netip.MustParsePrefix("192.169.1.0/24"))
	s, _ := b.IPSet()

	got := ipsetTotal(s)
	want := uint64((1 << 16) + (1 << 8))
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test3(t *testing.T) {
	// This test does not provide a complete guarantee of correct behavior for randomIpsIter.
	var b netipx.IPSetBuilder
	b.AddPrefix(netip.MustParsePrefix("192.168.0.0/16"))
	b.AddPrefix(netip.MustParsePrefix("192.169.1.0/24"))
	s, _ := b.IPSet()
	const count = 256

	i := 0
	for ip := range randomIpsIter(s) {
		if i >= count {
			break
		}
		i++
		if !s.Contains(ip) {
			t.Fatalf("got %v, which is not included in the test ipset", ip)
		}
	}
}
