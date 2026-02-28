// Checks if a censor restricts using the tcp 16-20 method

package checkers

import (
	"context"
	"crypto/tls"
	"dpich/config"
	"dpich/netutil"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
)

var (
	ErrTcp1620Conn = errors.New("connection error")
	ErrTcp1620Read = errors.New("read error")
)

type Tcp1620Opt struct {
	Url     string
	Resolve string
}

type Tcp1620ResultItem struct {
	Attrs EndpointAttrs
	Err   error
}

func Tcp1620Start(ctx context.Context) <-chan Tcp1620ResultItem {
	jobs := make(chan Tcp1620Opt)
	out := make(chan Tcp1620ResultItem)
	cfg := config.Get().Checkers.Tcp1620
	var wg sync.WaitGroup

	for range cfg.Workers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case opt, ok := <-jobs:
					if !ok {
						return
					}
					a, e := tcp1620Single(opt)
					select {
					case <-ctx.Done():
						return
					case out <- Tcp1620ResultItem{Attrs: a, Err: e}:
					}
				}
			}
		})
	}

	go func() {
		defer close(jobs)
		for _, u := range cfg.Endpoints {
			select {
			case <-ctx.Done():
				return
			case jobs <- Tcp1620Opt{Url: u}:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func tcp1620Single(opt Tcp1620Opt) (EndpointAttrs, error) {
	// TODO: Probably, we should try all IPs here,
	// exception ones from the same subnet (currently only ONE)
	// as well as different versions of TLS (1.2/1.3)

	attrs, err := getEndpointAttrs(opt.Url)
	if err != nil {
		return attrs, err
	}

	cfg := config.Get().Checkers.Tcp1620
	dialer := &net.Dialer{Timeout: cfg.TcpConnTimeout}

	tr := &http.Transport{
		DisableCompression:    true, // No decompression data is required
		TLSHandshakeTimeout:   cfg.TlsHandshakeTimeout,
		ResponseHeaderTimeout: cfg.HttpHeadersTimeout,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives:     true,
		Proxy:                 http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, _ := net.SplitHostPort(addr)
			if opt.Resolve == "" {
				// We want the IP to be the same as in the attributes.
				addr = net.JoinHostPort(attrs.IpAddr, port)
			} else {
				// TODO: If the IP is set manually, do we need to update the attributes?
				addr = net.JoinHostPort(opt.Resolve, port)
			}

			return dialer.DialContext(ctx, network, addr)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TotalTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opt.Url, http.NoBody)
	if err != nil {
		return attrs, err
	}

	netutil.SetBrowserHeaders(&req.Header)
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return attrs, ErrTcp1620Conn
	}
	defer resp.Body.Close()

	buf := make([]byte, cfg.BufSize)
	read := 0

	for read < cfg.NBytes {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			read += n
		}
		if err != nil {
			return attrs, ErrTcp1620Read
		}
	}

	return attrs, nil
}

type EndpointAttrs struct {
	Id           string
	Url          string
	Host         string
	IpAddr       string
	Subnet       string
	Asn          string
	Holder       string
	Country      string
	CountryEmoji string
	City         string
}

func getEndpointAttrs(endpointUrl string) (EndpointAttrs, error) {
	attrs := EndpointAttrs{Url: endpointUrl}
	u, err := url.Parse(endpointUrl)
	if err != nil {
		return attrs, err
	}

	attrs.Host = u.Hostname()
	// TODO: We should remove the ability to pass a nil context and split it into several functions
	ips, err := netutil.LookupIpViaDefault(nil, attrs.Host)
	if err != nil || len(ips) == 0 {
		return attrs, err
	}

	// TODO: Obviously, there may be several (v4/v6).
	attrs.IpAddr = ips[0].String()
	as, err := netutil.GetAsViaRipe(nil, attrs.IpAddr)
	if err != nil {
		return attrs, err
	}

	attrs.Asn = as.Asn
	attrs.Holder = stripHolder(as.Holder)
	attrs.Subnet = as.Subnet

	loc, err := netutil.GetLocationViaRipe(nil, attrs.IpAddr)
	if err != nil {
		return attrs, err
	}

	attrs.City = loc.City
	attrs.Country = loc.Country
	if len(loc.Country) < 2 {
		attrs.Country = "XX"
	} else {
		attrs.CountryEmoji = countryIsoToFlagEmoji(attrs.Country)
	}
	attrs = setEndpointId(attrs)

	return attrs, nil
}

func setEndpointId(e EndpointAttrs) EndpointAttrs {
	// The country and IP are not constant parts, so the prefix and suffix may change
	h := stripHostToN(e.Host, 5) + "-" + hashData(e.Url+e.IpAddr, 2)
	e.Id = e.Country + "." + h
	return e
}
