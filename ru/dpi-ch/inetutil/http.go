package inetutil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func Head(ctx context.Context, client *http.Client, url string, browserHeaders bool, close bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	req.Close = close
	if err != nil {
		return err
	}

	if browserHeaders {
		setBrowserHeaders(&req.Header)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}

func Get(ctx context.Context, client *http.Client, url string, browserHeaders bool, close bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	req.Close = close
	if err != nil {
		return nil, err
	}

	if browserHeaders {
		setBrowserHeaders(&req.Header)
	}

	resp, err := client.Do(req)
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

func GetAndUnmarshal[T any](ctx context.Context, client *http.Client, url string, v *T, browserHeaders bool, close bool) error {
	body, err := Get(ctx, client, url, browserHeaders, close)
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
