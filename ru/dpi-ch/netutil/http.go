package netutil

import (
	"context"
	"dpich/config"
	"encoding/json"
	"io"
	"net/http"
)

func Head(ctx context.Context, client *http.Client, url string, browserHeaders bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return err
	}

	if browserHeaders {
		SetBrowserHeaders(&req.Header)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}

func Get(ctx context.Context, client *http.Client, url string, browserHeaders bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	if browserHeaders {
		SetBrowserHeaders(&req.Header)
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

func GetAndUnmarshal[T any](ctx context.Context, client *http.Client, url string, v *T, browserHeaders bool) error {
	body, err := Get(ctx, client, url, browserHeaders)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(body, v); err != nil {
		return err
	}

	return nil
}

func SetBrowserHeaders(header *http.Header) {
	cfg := config.Get().Netutils
	for k, v := range cfg.BrowserHeaders {
		header.Set(k, v)
	}
}
