package webhost

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/internal/testutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func TestRunnerSummary(t *testing.T) {
	for _, name := range []string{"completed", "canceled", "timeout", "invalid filter"} {
		t.Run(name, func(t *testing.T) {
			cfg := testutil.WebhostConfig(t)
			cfg.Checkers.Webhost.FarmTimeout = time.Hour
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var targets []config.WebhostTarget
			switch name {
			case "canceled":
				cancel()
			case "timeout":
				cfg.Checkers.Webhost.FarmTimeout = time.Nanosecond
			case "invalid filter":
				targets = []config.WebhostTarget{{Filter: ")"}}
			}
			out := Run(ctx, targets)
			for range out.Out {
			}
			for range out.Progress {
			}
			select {
			case summary, ok := <-out.Summary:
				if !ok || summary.FarmTimeout != (name == "timeout") {
					t.Fatalf("got %v, open: %v", summary, ok)
				}
			case <-time.After(time.Second):
				t.Fatal("runner did not publish its summary")
			}
			if _, ok := <-out.Summary; ok {
				t.Fatal("runner published more than one summary")
			}
		})
	}
}

func TestRunnerFilterError(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.Checkers.Webhost.FarmTimeout = time.Second
	out := Run(t.Context(), []config.WebhostTarget{
		{Name: "invalid", Filter: "as(true)"},
		{Name: "empty", Filter: "subnet()"},
	})
	for range out.Out {
	}
	for range out.Progress {
	}
	summary := <-out.Summary
	if summary.Err == nil || summary.FarmTimeout {
		t.Fatalf("got %v, want filter error", summary)
	}
}

func TestRunnerTimeoutWithFullProgress(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.Checkers.Webhost.FarmTimeout = time.Second
	cfg.Checkers.Webhost.Workers = 8
	cfg.Subnetfilter.Workers = 8
	cfg.WebhostFarm.Workers = 8
	cfg.InetUtil.Fingerprint = "chrome"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	targets := make([]config.WebhostTarget, 64)
	for i := range targets {
		targets[i] = config.WebhostTarget{
			Name: "local", Filter: `subnet("127.0.0.1/32")`, Port: listener.Addr().(*net.TCPAddr).Port,
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := Run(ctx, targets)
	select {
	case summary := <-out.Summary:
		if !summary.FarmTimeout {
			t.Fatal("runner lost its timeout")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner blocked while progress was not consumed")
	}
	if len(out.Progress) != cap(out.Progress) {
		t.Fatalf("progress buffer is not full: %d/%d", len(out.Progress), cap(out.Progress))
	}
	for range out.Out {
	}
	for range out.Progress {
	}
}

func TestTcp1620BodyTimeout(t *testing.T) {
	cfg := testutil.WebhostConfig(t)
	cfg.InetUtil.Fingerprint = "chrome"
	cfg.Checkers.Webhost.TcpReadTimeout = 50 * time.Millisecond
	cfg.Checkers.Webhost.TcpWriteTimeout = time.Second
	cfg.Checkers.Webhost.Tcp1620nBytes = 16
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	defer server.CloseClientConnections()
	host, portRaw, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portRaw)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := checkTcp1620(SingleOpt{Ctx: ctx, Host: host}, inetutil.TlsConnOpt{
			Ctx: ctx, Ip: netip.MustParseAddr(host), Port: port,
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != inetutil.ErrTcpReadTimeout {
			t.Fatalf("got %v, want TCP read timeout", err)
		}
	case <-time.After(time.Second):
		server.CloseClientConnections()
		<-done
		t.Fatal("response body ignored the read timeout")
	}
}
