package webhostfarm

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"

	"go4.org/netipx"
)

func TestFarmCancelsHandshake(t *testing.T) {
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.InetUtil.Fingerprint = "chrome"
	cfg.WebhostFarm.TcpConnTimeout = 0
	cfg.WebhostFarm.TlsHandshakeTimeout = 0
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	peerCh := make(chan net.Conn, 1)
	go func() {
		peer, _ := listener.Accept()
		peerCh <- peer
	}()
	var subnets netipx.IPSetBuilder
	subnets.AddPrefix(netip.MustParsePrefix("127.0.0.1/32"))
	set, _ := subnets.IPSet()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		Farm(ctx, FarmOpt{Subnets: set, Count: 1, Port: listener.Addr().(*net.TCPAddr).Port})
		close(done)
	}()
	select {
	case peer := <-peerCh:
		defer peer.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			peer.Close()
			<-done
			t.Fatal("farm ignored cancellation during TLS handshake")
		}
	case <-time.After(time.Second):
		t.Fatal("farm did not connect to the local peer")
	}
}

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
