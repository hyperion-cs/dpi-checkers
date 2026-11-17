package webhostfarm

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"go4.org/netipx"
)

func farmTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.InetUtil.Fingerprint = "chrome"
	cfg.WebhostFarm.Workers = 2
	cfg.WebhostFarm.TcpConnTimeout = 250 * time.Millisecond
	cfg.WebhostFarm.TlsHandshakeTimeout = 250 * time.Millisecond
	return cfg
}

func farmTestSet(t *testing.T, ips ...string) *netipx.IPSet {
	t.Helper()
	var b netipx.IPSetBuilder
	for _, ip := range ips {
		b.Add(netip.MustParseAddr(ip))
	}
	set, err := b.IPSet()
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func farmTestTLS(t *testing.T, state func(net.Conn, http.ConnState)) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	server.Listener.Close()
	server.Listener = listener
	server.Config.ConnState = state
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

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
		for range Farm(ctx, FarmOpt{Subnets: set, Count: 1, Port: listener.Addr().(*net.TCPAddr).Port}) {
		}
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

func TestFarmStreamsBeforeLastProbe(t *testing.T) {
	cfg := farmTestConfig(t)
	cfg.WebhostFarm.TcpConnTimeout = 5 * time.Second
	cfg.WebhostFarm.TlsHandshakeTimeout = 5 * time.Second
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var accepted atomic.Int32
	server := farmTestTLS(t, func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew && accepted.Add(1) == 2 {
			close(secondStarted)
			<-release
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		unblock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("farm did not stop after cleanup cancellation")
		}
	})
	set := farmTestSet(t, "127.0.0.1", "127.0.0.2")
	items := make(chan struct{}, 2)
	go func() {
		defer close(done)
		for range Farm(ctx, FarmOpt{Subnets: set, Count: 2, Port: server.Listener.Addr().(*net.TCPAddr).Port}) {
			items <- struct{}{}
		}
	}()
	select {
	case <-items:
	case <-time.After(time.Second):
		t.Fatal("first host waited for the last probe")
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("farm did not start the second probe")
	}
	select {
	case <-done:
		t.Fatal("farm finished while the second probe was blocked")
	default:
	}
	unblock()
	select {
	case <-items:
	case <-time.After(time.Second):
		t.Fatal("second host did not arrive after releasing its probe")
	}
}

func TestFarmBoundaries(t *testing.T) {
	farmTestConfig(t)
	server := farmTestTLS(t, nil)
	port := server.Listener.Addr().(*net.TCPAddr).Port
	unused, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := unused.Addr().(*net.TCPAddr).Port
	unused.Close()
	set := farmTestSet(t, "127.0.0.1", "127.0.0.2")
	for _, tt := range []struct {
		name     string
		set      *netipx.IPSet
		count    int
		port     int
		canceled bool
		want     int
	}{
		{"count limit", set, 1, port, false, 1},
		{"exhausted set", set, 3, port, false, 2},
		{"fallback", set, 3, closedPort, false, 1},
		{"empty set", farmTestSet(t), 1, port, false, 0},
		{"zero count", set, 0, port, false, 0},
		{"negative count", set, -1, port, false, 0},
		{"canceled", set, 1, port, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if tt.canceled {
				cancel()
			}
			count := 0
			seen := map[netip.Addr]bool{}
			for item := range Farm(ctx, FarmOpt{Subnets: tt.set, Count: tt.count, Port: tt.port}) {
				count++
				if !item.Ip.IsValid() || !tt.set.Contains(item.Ip) || item.Port != tt.port || seen[item.Ip] {
					t.Errorf("invalid or duplicate result: %v", item)
				}
				seen[item.Ip] = true
			}
			if count != tt.want {
				t.Fatalf("got %d hosts, want %d", count, tt.want)
			}
			if !tt.canceled && ctx.Err() != nil {
				t.Fatal("farm finished only after the context deadline")
			}
		})
	}
}

func TestGochanBagAndFound(t *testing.T) {
	farmTestConfig(t)
	server := farmTestTLS(t, nil)
	port := server.Listener.Addr().(*net.TCPAddr).Port
	set := farmTestSet(t, "127.0.0.1", "127.0.0.2")
	input := make(chan GochanIn[string], 2)
	for _, name := range []string{"first", "second"} {
		input <- GochanIn[string]{Bag: name, In: FarmOpt{Subnets: set, Count: 2, Port: port}}
	}
	close(input)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	seen := map[string]int{}
	for item := range Gochan(GochanOpt[string]{Ctx: ctx, In: input}) {
		seen[item.Bag]++
		if item.Found != seen[item.Bag] || !set.Contains(item.Out.Ip) || item.Out.Port != port {
			t.Errorf("incorrect stream metadata: %v", item)
		}
	}
	if ctx.Err() != nil || seen["first"] != 2 || seen["second"] != 2 || len(seen) != 2 {
		t.Fatalf("got %v, context: %v", seen, ctx.Err())
	}
}

func TestGochanCancelsBlockedOutput(t *testing.T) {
	cfg := farmTestConfig(t)
	cfg.WebhostFarm.Workers = 1
	closed := make(chan struct{})
	var probes atomic.Int32
	server := farmTestTLS(t, func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed && probes.Add(1) == 2 {
			close(closed)
		}
	})
	input := make(chan GochanIn[string], 1)
	input <- GochanIn[string]{Bag: "local", In: FarmOpt{
		Subnets: farmTestSet(t, "127.0.0.1", "127.0.0.2"), Count: 2,
		Port: server.Listener.Addr().(*net.TCPAddr).Port,
	}}
	close(input)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := Gochan(GochanOpt[string]{Ctx: ctx, In: input})
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("farm did not finish both TLS probes")
	}
	cancel()
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("farm workers did not close a blocked output after cancellation")
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
	for ip := range randomIpsIter(t.Context(), s) {
		if i >= count {
			break
		}
		i++
		if !s.Contains(ip) {
			t.Fatalf("got %v, which is not included in the test ipset", ip)
		}
	}
}
