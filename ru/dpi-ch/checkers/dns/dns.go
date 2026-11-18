package dns

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"

	rand "math/rand/v2"
	"net/http"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/subnetfilter"
)

type plainProvider struct {
	Addrs  []string // ip:port (udp)
	Filter string   // subnetfilter for provider hijacking check
}

type dohProvider struct {
	Hosts  []string // RFC 8484: DoH + /dns-query + wire format
	Filter string   // subnetfilter for DoH bootstrap spoofing check
}

type target struct {
	Hostname string // target for receiving an A record
	Filter   string // subnetfilter for response spoofing check (relevant for plain mode)
}

type plainAnswer struct {
	Target       target
	ResolverAddr string // ip:port (udp)
	Items        []netip.Addr
	Err          error
}

type dohAnswer struct {
	Target           target
	ResolverHostname string
	Items            []dohAnswerItem
	BootstrapErr     error
}

type dohAnswerItem struct {
	ResolverIp netip.Addr
	Err        error
}

type Verdict struct {
	Provider string
	Verdict  error
}

type LeakResult struct {
	Items []inetlookup.IpInfoStrings
	Err   error
}

type leakAnswer struct {
	Addrs []netip.Addr
	Err   error
}

var (
	// There may also be network errors
	ErrSkip                 = errors.New("dns: skip")
	ErrProviderHijacking    = errors.New("dns: provider hijacking")
	ErrResolveSpoofing      = errors.New("dns: response spoofing")
	ErrNxdomainSpoofing     = errors.New("dns: nxdomain spoofing")
	ErrDohBootstrapSpoofing = errors.New("dns: doh bootstrap spoofing")
	ErrDohBootstrapEmpty    = errors.New("dns: doh bootstrap empty")
	ErrDohInsecure          = errors.New("dns: doh insecure")
	ErrDohNon2xxResp        = errors.New("dns: doh non-2xx response")
	ErrProviderTimeout      = errors.New("dns: provider timeout")
)

var dnsErrors = []error{
	// In descending order of importance
	ErrProviderHijacking,
	ErrResolveSpoofing,
	ErrNxdomainSpoofing,
	ErrDohBootstrapSpoofing,
	ErrDohBootstrapEmpty,
	ErrDohInsecure,
	ErrDohNon2xxResp,
	ErrProviderTimeout,
	ErrSkip,
}

func Errors(err error) []error {
	out := []error{}
	for _, x := range dnsErrors {
		if errors.Is(err, x) {
			out = append(out, x)
		}
	}
	return out
}

// Resolve in DoH mode + spoofing check.
func dohMatrix(ctx context.Context, dohProvider dohProvider, targets []target) []dohAnswer {
	res := []dohAnswer{}

	bootstraps := map[string][]netip.Addr{}
	bootstrapErrs := map[string]error{}
	for _, host := range dohProvider.Hosts {
		if err := ctx.Err(); err != nil {
			bootstrapErrs[host] = err
			continue
		}
		bootstraps[host], bootstrapErrs[host] = dohBootstrap(ctx, host, dohProvider.Filter)
	}

	for _, target := range targets {
		for _, host := range dohProvider.Hosts {
			ans := dohAnswer{Target: target, ResolverHostname: host, Items: []dohAnswerItem{}}

			if err := bootstrapErrs[host]; err != nil {
				ans.BootstrapErr = err
			} else {
				for _, ip := range bootstraps[host] {
					if ctx.Err() != nil {
						break
					}
					ans.Items = append(ans.Items, dohRaw(ctx, target, inetutil.TlsConnOpt{
						Ip: ip, Port: 443, Sni: host, InsecureVerify: true,
					}))
				}
			}

			res = append(res, ans)
		}
	}

	return res
}

// The DoH host is resolved via the system resolver: that is the path a real client
// takes, so an A record spoofed on the way is exactly what has to be caught here.
func dohBootstrap(ctx context.Context, host, filter string) ([]netip.Addr, error) {
	cfg := config.Get().Checkers.Dns.Resolve

	ctx, cancel := context.WithTimeout(ctx, cfg.PlainOpt.QueryTimeout)
	defer cancel()

	raw, err := inetlookup.LookupIpViaDefault(ctx, host)
	if err != nil {
		return nil, err
	}

	ips := make([]netip.Addr, 0, len(raw))
	for _, ip := range raw {
		x := netip.MustParseAddr(ip.String())
		if x.Is4() {
			ips = append(ips, x)
		}
	}

	if len(ips) == 0 {
		return nil, ErrDohBootstrapEmpty
	}

	if filter == "" {
		return ips, nil
	}

	orig, err := subnetfilterMatchAll(ctx, ips, filter)
	if err != nil {
		return nil, err
	}
	if !orig {
		log.Println("dnsDohBootstrap", "bootstrap spoofing", host, ips)
		return nil, ErrDohBootstrapSpoofing
	}

	return ips, nil
}

func dohRaw(ctx context.Context, target target, tlsConnOpt inetutil.TlsConnOpt) dohAnswerItem {
	cfg := config.Get().Checkers.Dns.Resolve
	res := dohAnswerItem{ResolverIp: tlsConnOpt.Ip}

	innerCtx, cancel := context.WithTimeout(ctx, cfg.DohOpt.QueryTimeout)
	defer cancel()

	tlsConnOpt.Ctx = innerCtx

	tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		res.Err = err
		if err == inetutil.ErrTlsCertificateInvalid {
			res.Err = ErrDohInsecure
		}

		return res
	}
	defer tlsConn.Close()
	stopClose := context.AfterFunc(innerCtx, func() { tlsConn.Close() })
	defer stopClose()

	preparedA, err := prepareA(target.Hostname)
	if err != nil {
		res.Err = err
		return res
	}

	req, err := http.NewRequest("POST", "https://"+tlsConnOpt.Sni+cfg.DohOpt.Path, bytes.NewReader(preparedA))
	if err != nil {
		res.Err = err
		return res
	}
	req.Close = true // TODO: it is better to keep one connection open to each resolver
	req.Header.Set("Content-Type", "application/dns-message")

	inetutil.SetHeaders(&req.Header, cfg.DohOpt.HttpStaticHeaders)

	if _, err := inetutil.TlsWriteHttpRequest(innerCtx, tlsConn, req); err != nil {
		res.Err = err
		return res
	}

	resp, err := inetutil.TlsReadHttpResponse(innerCtx, tlsConn, bufio.NewReader(tlsConn))
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		res.Err = err
		if errors.Is(innerCtx.Err(), context.DeadlineExceeded) {
			res.Err = inetutil.ErrTcpReadTimeout
		}
		return res
	}
	if err := innerCtx.Err(); err != nil {
		res.Err = err
		return res
	}

	if resp.StatusCode != 200 {
		res.Err = ErrDohNon2xxResp
	}

	return res
}

func verdict(errs []error) error {
	found := []error{}

	for _, sentinel := range dnsErrors {
		for _, err := range errs {
			if errors.Is(err, sentinel) {
				found = append(found, sentinel)
				break
			}
		}
	}

	if len(found) > 0 {
		return errors.Join(found...)
	}

	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

func plainVerdict(matrix []plainAnswer, hijacking error) error {
	errs := make([]error, 0, len(matrix)+1)
	for _, m := range matrix {
		errs = append(errs, m.Err)
	}
	errs = append(errs, hijacking)

	err := verdict(errs)
	if err != nil {
		log.Println("dnsPlainVerdict", matrix)
	}
	return err
}

func dohVerdict(matrix []dohAnswer) error {
	errs := []error{}
	for _, m := range matrix {
		errs = append(errs, m.BootstrapErr)
		for _, item := range m.Items {
			errs = append(errs, item.Err)
		}
	}

	err := verdict(errs)
	if err != nil {
		log.Println("dnsDohVerdict", matrix)
	}
	return err
}

// Resolve in plain mode + spoofing check.
func plainMatrix(ctx context.Context, provider plainProvider, targets []target) []plainAnswer {
	res := []plainAnswer{}

	for _, target := range targets {
		for _, addr := range provider.Addrs {
			if ctx.Err() != nil {
				return res
			}
			item := plainAnswer{Target: target, ResolverAddr: addr}

			func() {
				ips, err := plainA(ctx, addr, target.Hostname)

				if err != nil {
					if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
						if dnsErr.IsNotFound {
							item.Err = errors.Join(item.Err, ErrNxdomainSpoofing)
							return
						}
					}

					if item.Err == nil {
						item.Err = err
					}
					return
				}

				orig, err := subnetfilterMatchAll(ctx, ips, target.Filter)
				if err != nil {
					if item.Err == nil {
						item.Err = err
					}
					return
				}

				item.Items = ips
				if !orig {
					item.Err = errors.Join(item.Err, ErrResolveSpoofing)
					log.Println("dnsPlainMatrix", "response spoofing", item)
				}
			}()

			res = append(res, item)
		}
	}

	return res
}

// Whoami-based hijacking check (plain mode only)
func plainHijacking(ctx context.Context, provider plainProvider) error {
	cfg := config.Get().Checkers.Dns.Resolve
	if cfg.Whoami.Host == "" || provider.Filter == "" {
		return nil
	}

	probe := target{Hostname: cfg.Whoami.Host, Filter: provider.Filter}
	for _, ans := range plainMatrix(ctx, provider, []target{probe}) {
		if errors.Is(ans.Err, ErrResolveSpoofing) {
			log.Println("dnsPlainHijacking", "provider hijacking", ans)
			return ErrProviderHijacking
		}
	}

	return ctx.Err()
}

// DNS servers that are actually used. The answer may not be comprehensive.
func leakSingle(ctx context.Context) leakAnswer {
	cfg := config.Get().Checkers.Dns

	ctx, cancel := context.WithTimeout(ctx, cfg.Leak.Timeout)
	defer cancel()

	url := "https://" + randString(cfg.Leak.LabelAlpha, cfg.Leak.LabelLen) + "." + cfg.Leak.ParentDomain
	var respRaw map[string][]string
	if err := inetutil.GetAndUnmarshal(ctx, url, &respRaw, true, true); err != nil {
		return leakAnswer{nil, err}
	}

	out := make([]netip.Addr, 0, len(respRaw))
	for k := range respRaw {
		out = append(out, netip.MustParseAddr(k))
	}
	return leakAnswer{out, nil}
}

func leakWithIpInfo(ctx context.Context) LeakResult {
	leak := leakSingle(ctx)
	if leak.Err != nil {
		log.Println("dnsLeakSingle", leak.Err)
		return LeakResult{Err: leak.Err}
	}

	il := inetlookup.Default()
	items := []inetlookup.IpInfoStrings{}
	for _, ip := range leak.Addrs {
		info := il.IpInfo(ip)
		items = append(items, inetlookup.IpInfoAsStrings(info))
	}

	return LeakResult{Items: items}
}

// Checks if subfilter matches specified ip addresses.
func subnetfilterMatchAll(ctx context.Context, ips []netip.Addr, filter string) (bool, error) {
	sf := subnetfilter.Default()
	compiled, err := sf.CompileFilter(filter)
	if err != nil {
		return false, err
	}

	ipset, err := sf.RunFilter(ctx, compiled)
	if err != nil {
		return false, err
	}

	for _, ip := range ips {
		if !ipset.Contains(ip) {
			return false, nil
		}
	}

	return true, nil
}

func prepareA(target string) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		RecursionDesired: true,
	})
	b.EnableCompression()

	err := b.StartQuestions()
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(target, ".") {
		target = target + "."
	}

	err = b.Question(dnsmessage.Question{
		Name:  dnsmessage.MustNewName(target),
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.ClassINET,
	})
	if err != nil {
		return nil, err
	}

	return b.Finish()
}

// Resolves A records for the specified hostname using the specified DNS server.
func plainA(ctx context.Context, addr, target string) ([]netip.Addr, error) {
	cfg := config.Get().Checkers.Dns.Resolve
	ctx, cancel := context.WithTimeout(ctx, cfg.PlainOpt.QueryTimeout)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(_ctx context.Context, _network, _ string) (net.Conn, error) {
			d := &net.Dialer{}
			return d.DialContext(_ctx, _network, addr)
		},
	}

	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil {
		return nil, err
	}

	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		x := netip.MustParseAddr(ip.String())
		if x.Is4() {
			out = append(out, x)
		}
	}
	return out, nil
}

func randString(alpha string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[rand.IntN(len(alpha))]
	}
	return string(b)
}
