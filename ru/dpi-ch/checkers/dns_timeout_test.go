package checkers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func dnsTestConfig(t *testing.T, raw string) *config.Config {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(file); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func dnsTestServer(t *testing.T, nxdomain string) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			var msg dnsmessage.Message
			if err := msg.Unpack(buf[:n]); err != nil || len(msg.Questions) == 0 || msg.Questions[0].Name.String() != nxdomain {
				continue
			}
			msg.Header.Response = true
			msg.Header.RCode = dnsmessage.RCodeNameError
			raw, err := msg.Pack()
			if err == nil {
				conn.WriteTo(raw, addr)
			}
		}
	}()
	t.Cleanup(func() { conn.Close(); <-done })
	return conn.LocalAddr().String()
}

func TestDnsPlainQueryTimeout(t *testing.T) {
	cfg := dnsTestConfig(t, "{}")
	cfg.Checkers.Dns.Resolve.PlainOpt.QueryTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := dnsPlainA(ctx, dnsTestServer(t, ""), "probe.example.")
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("got %v, parent context: %v", err, ctx.Err())
	}
}

func TestDnsProviderTimeoutDelivery(t *testing.T) {
	for _, mode := range []string{"plain", "doh"} {
		t.Run(mode, func(t *testing.T) {
			addr := dnsTestServer(t, "")
			hosts := fmt.Sprintf("plain: {hosts: [%q]}", addr)
			if mode == "doh" {
				hosts = "doh: {hosts: [127.0.0.1]}"
			}
			cfg := dnsTestConfig(t, fmt.Sprintf(`checkers:
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
			var out <-chan DnsVerdict
			if mode == "doh" {
				out = DnsDohGochan(ctx)
			} else {
				out = DnsPlainGochan(ctx)
			}
			// Leave the first verdict blocked until its provider deadline has passed.
			time.Sleep(100 * time.Millisecond)
			seen := map[string]bool{}
			for item := range out {
				if !errors.Is(item.Verdict, ErrDnsProviderTimeout) {
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
	addr := dnsTestServer(t, "missing.example.")
	cfg := dnsTestConfig(t, fmt.Sprintf(`checkers:
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
	for item := range DnsPlainGochan(ctx) {
		count++
		if !errors.Is(item.Verdict, ErrDnsNxdomainSpoofing) || !errors.Is(item.Verdict, ErrDnsProviderTimeout) {
			t.Errorf("got %v, want NXDOMAIN spoofing and provider timeout", item.Verdict)
		}
	}
	if count != 1 || ctx.Err() != nil {
		t.Fatalf("received %d results, parent context: %v", count, ctx.Err())
	}
}

func TestDnsProviderCancellation(t *testing.T) {
	addr := dnsTestServer(t, "")
	dnsTestConfig(t, fmt.Sprintf(`checkers:
  dns:
    resolve:
      providers:
        - name: local
          plain: {hosts: [%q]}
`, addr))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := DnsPlainGochan(ctx)
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
	cfg := dnsTestConfig(t, "{}")
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
			result := dnsDohRaw(ctx, DnsTarget{Hostname: "probe.example"}, inetutil.TlsConnOpt{
				Ip: addr.Addr(), Port: int(addr.Port()), Sni: "example.com",
			})
			if result.Err != inetutil.ErrTcpReadTimeout || ctx.Err() != nil {
				t.Fatalf("got %v, parent context: %v", result.Err, ctx.Err())
			}
		})
	}
}
