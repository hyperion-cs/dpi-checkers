package inetutil

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

var (
	httpMu     sync.Mutex
	httpClient *http.Client
)

func Head(ctx context.Context, url string, browserHeaders bool, close bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	req.Close = close
	if err != nil {
		return err
	}

	if browserHeaders {
		setBrowserHeaders(&req.Header)
	}

	resp, err := httpDefaultClient().Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}

func Get(ctx context.Context, url string, browserHeaders bool, close bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	req.Close = close
	if err != nil {
		return nil, err
	}

	if browserHeaders {
		setBrowserHeaders(&req.Header)
	}

	resp, err := httpDefaultClient().Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func GetAndUnmarshal[T any](ctx context.Context, url string, v *T, browserHeaders bool, close bool) error {
	body, err := Get(ctx, url, browserHeaders, close)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(body, v); err != nil {
		return err
	}

	return nil
}

func SetHeaders(out *http.Header, headers map[string]string) {
	for k, v := range headers {
		out.Set(k, v)
	}
}

func setBrowserHeaders(out *http.Header) {
	cfg := config.Get().InetUtil
	SetHeaders(out, cfg.BrowserHeaders)
}

// Returns default http client for inetutil package, considering network interface options in config.
func httpDefaultClient() *http.Client {
	httpMu.Lock()
	defer httpMu.Unlock()

	if httpClient == nil {
		tcpDialer := net.Dialer{}
		if ifaceAddr, err := Iface4(); err == nil {
			tcpDialer.LocalAddr = &net.TCPAddr{
				IP:   net.IP(ifaceAddr.AsSlice()),
				Port: 0,
			}
			log.Println("inetutil/http", "network interface specified", ifaceAddr)
		}

		httpClient = &http.Client{
			Transport: &http.Transport{DialContext: tcpDialer.DialContext},
		}
	}

	return httpClient
}
