package checkers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"strings"

	rand "math/rand/v2"
	"net/http"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/subnetfilter"
)

type DnsPlainProvider struct {
	Addrs []string // ip:port (udp)
}

type DnsDohProvider struct {
	Hosts  []string // RFC 8484: DoH + /dns-query + wire format
	Filter string   // subnetfilter for DoH bootstrap spoofing check
}

type DnsTarget struct {
	Hostname string // target for receiving an A record
	Filter   string // subnetfilter for response spoofing check (relevant for plain mode)
}

type DnsPlainAnswer struct {
	Target       DnsTarget
	ResolverAddr string // ip:port (udp)
	Items        []netip.Addr
	Err          error
}

type DnsDohAnswer struct {
	Target           DnsTarget
	ResolverHostname string
	Items            []DnsDohAnswerItem
	BootstrapErr     error
}

type DnsDohAnswerItem struct {
	ResolverIp netip.Addr
	Err        error
}

type DnsVerdict struct {
	Provider string
	Verdict  error
}

type DnsLeakWithIpinfoOut struct {
	Items []inetlookup.IpInfoStrings
	Err   error
}

type dnsLeakOut struct {
	Addrs []netip.Addr
	Err   error
}

var (
	// There may also be network errors
	ErrDnsSkip                 = errors.New("dns: skip")
	ErrDnsResolveSpoofing      = errors.New("dns: response spoofing")
	ErrDnsDohBootstrapSpoofing = errors.New("dns: doh bootstrap spoofing")
	ErrDnsDohBootstrapEmpty    = errors.New("dns: doh bootstrap empty")
	ErrDnsDohInsecure          = errors.New("dns: doh insecure")
	ErrDnsDohNon2xxResp        = errors.New("dns: doh non-2xx response")
)

// Resolve in DoH mode + spoofing check; bsProvider is used for the DoH bootstrap.
func dnsDohMatrix(ctx context.Context, bsProvider DnsPlainProvider, dohProvider DnsDohProvider, targets []DnsTarget) []DnsDohAnswer {
	res := []DnsDohAnswer{}

	bootstraps := map[string][]DnsPlainAnswer{}
	for _, host := range dohProvider.Hosts {
		target := []DnsTarget{{Hostname: host, Filter: dohProvider.Filter}}
		bootstraps[host] = dnsPlainMatrix(ctx, bsProvider, target)
	}

	for _, target := range targets {
		for _, host := range dohProvider.Hosts {
			ans := DnsDohAnswer{Target: target, ResolverHostname: host, Items: []DnsDohAnswerItem{}}
			hostBootstrap := bootstraps[host]

			func() {
				hostIps := map[netip.Addr]struct{}{}

				for _, bs := range hostBootstrap {
					if bs.Err == ErrDnsResolveSpoofing {
						ans.BootstrapErr = ErrDnsDohBootstrapSpoofing
						return
					}
					for _, ip := range bs.Items {
						hostIps[ip] = struct{}{}
					}
				}

				if len(hostIps) == 0 {
					ans.BootstrapErr = ErrDnsDohBootstrapEmpty
					return
				}

				for ip := range hostIps {
					item := dnsDohRaw(ctx, target, host, ip)
					ans.Items = append(ans.Items, item)
				}
			}()

			res = append(res, ans)
		}
	}

	return res
}

func dnsDohRaw(ctx context.Context, target DnsTarget, resolverHostname string, resolverIp netip.Addr) DnsDohAnswerItem {
	cfg := config.Get().Checkers.Dns.Resolve
	res := DnsDohAnswerItem{ResolverIp: resolverIp}

	innerCtx, cancel := context.WithTimeout(ctx, cfg.DohOpt.Timeout)
	defer cancel()

	tlsConnOpt := httputil.TlsConnOpt{
		Ctx:            innerCtx,
		Ip:             resolverIp,
		Port:           443, // TODO: config that
		Sni:            resolverHostname,
		InsecureVerify: true,
	}

	tlsConn, err := httputil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		res.Err = err
		if err == httputil.ErrTlsCertificateInvalid {
			res.Err = ErrDnsDohInsecure
		}

		return res
	}
	defer tlsConn.Close()

	preparedA, err := dnsDohPrepareA(target.Hostname)
	if err != nil {
		res.Err = err
		return res
	}

	req, err := http.NewRequest("POST", "https://"+resolverHostname+cfg.DohOpt.Path, bytes.NewReader(preparedA))
	if err != nil {
		res.Err = err
		return res
	}
	req.Close = true // TODO: it is better to keep one connection open to each resolver
	req.Header.Set("Content-Type", "application/dns-message")

	httputil.SetHeaders(&req.Header, cfg.DohOpt.HttpStaticHeaders)

	if _, err := httputil.TlsWriteHttpRequest(innerCtx, tlsConn, req); err != nil {
		res.Err = err
		return res
	}

	resp, err := httputil.TlsReadHttpResponse(innerCtx, tlsConn, bufio.NewReader(tlsConn))
	if err != nil {
		res.Err = err
		return res
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		res.Err = ErrDnsDohNon2xxResp
	}

	return res
}

func dnsPlainVerdict(matrix []DnsPlainAnswer) error {
	// We need to make a single verdict on DNS providers,
	// so choose the most dangerous case.
	var err error
	for _, m := range matrix {
		if plainErrImportance(m.Err) > plainErrImportance(err) {
			err = m.Err
		}
	}
	return err
}

func dnsDohVerdict(matrix []DnsDohAnswer) error {
	// We need to make a single verdict on DNS providers,
	// so choose the most dangerous case.
	var err error
	log.Println("dnsDohVerdict", matrix)
	for _, m := range matrix {
		if dohErrImportance(m.BootstrapErr) > dohErrImportance(err) {
			err = m.BootstrapErr
		}
		if len(m.Items) > 0 {
			for _, item := range m.Items {
				if dohErrImportance(item.Err) > dohErrImportance(err) {
					err = item.Err
				}
			}
		}
	}
	return err
}

func plainErrImportance(err error) int {
	switch err {
	case ErrDnsResolveSpoofing:
		return 2
	case nil:
		return 0
	default:
		return 1
	}
}

func dohErrImportance(err error) int {
	if httputil.IsHttputilErr(err) {
		return 3
	}
	switch err {
	case ErrDnsDohBootstrapSpoofing:
		return 5
	case ErrDnsDohInsecure:
		return 4
	case ErrDnsDohNon2xxResp:
		return 2
	case nil:
		return 0
	default:
		return 1
	}
}

// Resolve in plain mode + spoofing check.
func dnsPlainMatrix(ctx context.Context, provider DnsPlainProvider, targets []DnsTarget) []DnsPlainAnswer {
	res := []DnsPlainAnswer{}

	for _, target := range targets {
		for _, addr := range provider.Addrs {
			item := DnsPlainAnswer{Target: target, ResolverAddr: addr}

			func() {
				ips, err := dnsPlainA(ctx, addr, target.Hostname)
				if err != nil {
					item.Err = err
					return
				}

				orig, err := subnetfilterMatchAll(ips, target.Filter)
				if err != nil {
					item.Err = err
					return
				}

				item.Items = ips
				if !orig {
					item.Err = ErrDnsResolveSpoofing
					log.Println("dnsPlainMatrix", "response spoofing", item)
				}
			}()

			res = append(res, item)
		}
	}

	return res
}

// DNS servers that are actually used. The answer may not be comprehensive.
func dnsLeakSingle() dnsLeakOut {
	cfg := config.Get().Checkers.Dns

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Leak.Timeout)
	defer cancel()

	url := "https://" + randString(cfg.Leak.LabelAlpha, cfg.Leak.LabelLen) + "." + cfg.Leak.ParentDomain
	var respRaw map[string][]string
	if err := httputil.GetAndUnmarshal(ctx, http.DefaultClient, url, &respRaw, true, true); err != nil {
		return dnsLeakOut{nil, err}
	}

	out := make([]netip.Addr, 0, len(respRaw))
	for k := range respRaw {
		out = append(out, netip.MustParseAddr(k))
	}
	return dnsLeakOut{out, nil}
}

func dnsLeakWithIpinfoSingle() DnsLeakWithIpinfoOut {
	leak := dnsLeakSingle()
	if leak.Err != nil {
		log.Println("dnsLeakSingle", leak.Err)
		return DnsLeakWithIpinfoOut{Err: leak.Err}
	}

	il := inetlookup.Default()
	items := []inetlookup.IpInfoStrings{}
	for _, ip := range leak.Addrs {
		info := il.IpInfo(ip)
		items = append(items, inetlookup.IpInfoAsStrings(info))
	}

	return DnsLeakWithIpinfoOut{Items: items}
}

// Checks if subfilter matches specified ip addresses.
func subnetfilterMatchAll(ips []netip.Addr, filter string) (bool, error) {
	sf := subnetfilter.Default()
	compiled, err := sf.CompileFilter(filter)
	if err != nil {
		return false, err
	}

	ipset, err := sf.RunFilter(compiled)
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

func dnsDohPrepareA(target string) ([]byte, error) {
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
func dnsPlainA(ctx context.Context, addr, target string) ([]netip.Addr, error) {
	cfg := config.Get().Checkers.Dns.Resolve
	ctx, cancel := context.WithTimeout(ctx, cfg.PlainOpt.Timeout)
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
