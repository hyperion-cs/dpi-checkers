package dns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/internal/testutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func TestDnsPlainQueryTimeout(t *testing.T) {
	cfg := testutil.DnsConfig(t, "{}")
	cfg.Checkers.Dns.Resolve.PlainOpt.QueryTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := plainA(ctx, testutil.DnsServer(t, ""), "probe.example.")
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("got %v, parent context: %v", err, ctx.Err())
	}
}

func TestDnsProviderTimeoutDelivery(t *testing.T) {
	for _, mode := range []string{"plain", "doh"} {
		t.Run(mode, func(t *testing.T) {
			addr := testutil.DnsServer(t, "")
			hosts := fmt.Sprintf("plain: {hosts: [%q]}", addr)
			if mode == "doh" {
				hosts = "doh: {hosts: [127.0.0.1]}"
			}
			cfg := testutil.DnsConfig(t, fmt.Sprintf(`checkers:
  dns:
    resolve:
      targets: [{host: probe.example}]
      providers:
        - name: first
          %s
        - name: second
          %s
`, hosts, hosts))
			cfg.Checkers.Dns.Resolve.PlainOpt.QueryTimeout = time.Second
			cfg.Checkers.Dns.Resolve.PlainOpt.ProviderTimeout = 50 * time.Millisecond
			cfg.Checkers.Dns.Resolve.PlainOpt.Workers = 1
			cfg.Checkers.Dns.Resolve.DohOpt.ProviderTimeout = time.Nanosecond
			cfg.Checkers.Dns.Resolve.DohOpt.Workers = 1
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var out <-chan Verdict
			if mode == "doh" {
				out = DohGochan(ctx)
			} else {
				out = PlainGochan(ctx)
			}
			// Leave the first verdict blocked until its provider deadline has passed.
			time.Sleep(100 * time.Millisecond)
			seen := map[string]bool{}
			for item := range out {
				if !errors.Is(item.Verdict, ErrProviderTimeout) {
					t.Errorf("%s: got %v, want provider timeout", item.Provider, item.Verdict)
				}
				seen[item.Provider] = true
			}
			if ctx.Err() != nil || len(seen) != 2 {
				t.Fatalf("got %v, parent context: %v", seen, ctx.Err())
			}
		})
	}
}

func TestDnsProviderTimeoutKeepsFindings(t *testing.T) {
	addr := testutil.DnsServer(t, "missing.example.")
	cfg := testutil.DnsConfig(t, fmt.Sprintf(`checkers:
  dns:
    resolve:
      targets: [{host: missing.example.}, {host: silent.example.}]
      providers:
        - name: local
          plain: {hosts: [%q]}
`, addr))
	cfg.Checkers.Dns.Resolve.PlainOpt.QueryTimeout = time.Second
	cfg.Checkers.Dns.Resolve.PlainOpt.ProviderTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	count := 0
	for item := range PlainGochan(ctx) {
		count++
		if !errors.Is(item.Verdict, ErrNxdomainSpoofing) || !errors.Is(item.Verdict, ErrProviderTimeout) {
			t.Errorf("got %v, want NXDOMAIN spoofing and provider timeout", item.Verdict)
		}
	}
	if count != 1 || ctx.Err() != nil {
		t.Fatalf("received %d results, parent context: %v", count, ctx.Err())
	}
}

func TestDnsProviderCancellation(t *testing.T) {
	addr := testutil.DnsServer(t, "")
	testutil.DnsConfig(t, fmt.Sprintf(`checkers:
  dns:
    resolve:
      providers:
        - name: local
          plain: {hosts: [%q]}
`, addr))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := PlainGochan(ctx)
	time.AfterFunc(30*time.Millisecond, cancel)
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("DNS workers did not stop after cancellation")
	}
}

func TestDnsDohBodyTimeout(t *testing.T) {
	cfg := testutil.DnsConfig(t, "{}")
	cfg.InetUtil.Fingerprint = "chrome"
	cfg.Checkers.Dns.Resolve.DohOpt.QueryTimeout = 50 * time.Millisecond
	for _, connectionClose := range []bool{false, true} {
		t.Run(fmt.Sprintf("connection-close=%v", connectionClose), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				closeHeader := ""
				if connectionClose {
					closeHeader = "Connection: close\r\n"
				}
				fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n%s\r\n", closeHeader)
				io.Copy(io.Discard, conn)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			addr := netip.MustParseAddrPort(server.Listener.Addr().String())
			result := dohRaw(ctx, target{Hostname: "probe.example"}, inetutil.TlsConnOpt{
				Ip: addr.Addr(), Port: int(addr.Port()), Sni: "example.com",
			})
			if result.Err != inetutil.ErrTcpReadTimeout || ctx.Err() != nil {
				t.Fatalf("got %v, parent context: %v", result.Err, ctx.Err())
			}
		})
	}
}
