package checkers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

func TestWebhostTcp1620BodyTimeout(t *testing.T) {
	cfg := webhostTestConfig(t)
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
		_, err := webhostTcp1620check(WebhostSingleOpt{Ctx: ctx, Host: host}, inetutil.TlsConnOpt{
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
