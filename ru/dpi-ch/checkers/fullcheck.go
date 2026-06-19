package checkers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	yaml "go.yaml.in/yaml/v3"
)

var ErrFullCheckInvalidOutputFormat = errors.New("all: invalid output format")

type FullCheckProgress struct {
	Msg string
}

type FullCheckWhoamiDto struct {
	Status   FullCheckStatusDto
	Ip       string
	Subnet   string
	Asn      string
	Org      string
	Location string
	TtlbMs   int64
}

type FullCheckCidrwhitelistDto struct {
	Status FullCheckStatusDto
}

type FullCheckDnsLeakDto struct {
	Status FullCheckStatusDto
	Items  []inetlookup.IpInfoStrings
}

type FullCheckDnsReportDto struct {
	Status    FullCheckStatusDto
	Providers map[string]FullCheckStatusDto
}

type FullCheckDnsDto struct {
	Leak  *FullCheckDnsLeakDto
	Plain *FullCheckDnsReportDto
	Doh   *FullCheckDnsReportDto
}

type FullCheckWebhostItemDto struct {
	Group       string
	Org         string
	AS          string
	Location    string
	IP          string
	Prefix      string
	Alive       FullCheckStatusDto
	TlsV        string
	Tcp1620     FullCheckStatusDto
	Siberian    FullCheckStatusDto
	BurstTxKbps *float64
	BurstRxKbps *float64
}

type FullCheckWebhostDto struct {
	Items []FullCheckWebhostItemDto
}

type FullCheckStatusDto struct {
	Msg  string
	Code string
}

type FullCheckDto struct {
	Whoami        *FullCheckWhoamiDto
	CidrWhitelist *FullCheckCidrwhitelistDto
	Dns           *FullCheckDnsDto
	Webhost       map[string]FullCheckWebhostDto
}

func FullCheckGochan(ctx context.Context) <-chan FullCheckProgress {
	cfg := config.Get()
	progressCh := make(chan FullCheckProgress, 16)
	r := &FullCheckDto{}

	go func() {
		defer close(progressCh)
		var wg sync.WaitGroup
		fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "started"})

		var whoami *FullCheckWhoamiDto
		if slices.Contains(cfg.All.Checkers, "whoami") {
			wg.Go(func() {
				whoamiRes, whoamiErr := Whoami()
				val := fullCheckWhoamiDto(whoamiRes, whoamiErr)
				whoami = &val
				fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "whoami ready"})
			})
		}

		var cidrwhitelist *FullCheckCidrwhitelistDto
		if slices.Contains(cfg.All.Checkers, "cidrwhitelist") {
			wg.Go(func() {
				val := fullCheckCidrwhitelistDto(CidrWhitelist())
				cidrwhitelist = &val
				fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "cidrwhitelist ready"})
			})
		}

		var dnsLeak *FullCheckDnsLeakDto
		var dnsPlain *FullCheckDnsReportDto
		var dnsDoh *FullCheckDnsReportDto
		if slices.Contains(cfg.All.Checkers, "dns") {
			wg.Go(func() {
				gch := DnsLeakGochan(ctx)
				items := []DnsLeakWithIpinfoOut{}
				for v := range gch {
					items = append(items, v)
				}
				val := fullCheckDnsLeakDto(items)
				dnsLeak = &val
				fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "dns:leak ready"})
			})

			wg.Go(func() {
				items := []DnsVerdict{}
				gch := DnsPlainGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := fullCheckDnsReportDto(items)
				dnsPlain = &val
				fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "dns:plain ready"})
			})

			wg.Go(func() {
				items := []DnsVerdict{}
				gch := DnsDohGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := fullCheckDnsReportDto(items)
				dnsDoh = &val
				fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "dns:doh ready"})
			})
		}

		var webhostMu sync.Mutex
		webhost := map[string]FullCheckWebhostDto{}
		if slices.Contains(cfg.All.Checkers, "webhost") {
			for _, s := range cfg.Checkers.Webhost.Sections {
				wg.Go(func() {
					gch := WebhostGochanRunner(WebhostGochanRunnerOpt{Ctx: ctx, Targets: s.Targets})
					for o := range gch.Out {
						webhostMu.Lock()
						if _, ok := webhost[s.Name]; !ok {
							webhost[s.Name] = FullCheckWebhostDto{Items: []FullCheckWebhostItemDto{}}
						}
						curr := webhost[s.Name]
						curr.Items = append(curr.Items, fullCheckWebhostItemDto(o))
						webhost[s.Name] = curr
						webhostMu.Unlock()
						fullCheckSendProgress(progressCh, FullCheckProgress{Msg: fmt.Sprintf(`webhost[%s]: "%s" ready`, s.Name, o.Bag.Name)})
					}
					fullCheckSendProgress(progressCh, FullCheckProgress{Msg: fmt.Sprintf("webhost[%s] ready", s.Name)})
				})
			}
		}

		wg.Wait()
		r.Whoami = whoami
		r.CidrWhitelist = cidrwhitelist
		r.Webhost = webhost
		if dnsLeak != nil || dnsPlain != nil || dnsDoh != nil {
			r.Dns = &FullCheckDnsDto{
				Leak:  dnsLeak,
				Plain: dnsPlain,
				Doh:   dnsDoh,
			}
		}

		savePath, err := fullCheckSavePath()
		if err != nil {
			log.Println("fullcheck/savepath:", err)
			fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "error with the output path; enable debug and check the logs"})
			return
		}
		if err = fullCheckSave(r); err != nil {
			log.Println("fullcheck/save:", err)
			fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "error when saving to a file; enable debug and check the logs"})
			return
		}
		fullCheckSendProgress(progressCh, FullCheckProgress{Msg: "done; saved to " + savePath})
	}()

	return progressCh

}

func fullCheckSavePath() (string, error) {
	cfg := config.Get().All
	reportPath := cfg.Prefix + time.Now().Format(cfg.TsFormat) + "." + cfg.Format

	if !path.IsAbs(reportPath) {
		binFolder, err := config.BinFolder()
		if err != nil {
			return "", err
		}
		reportPath = path.Join(binFolder, reportPath)
	}

	return path.Clean(reportPath), nil
}

func fullCheckSave(dto *FullCheckDto) error {
	cfg := config.Get().All
	reportPath, err := fullCheckSavePath()
	if err != nil {
		return err
	}

	var out []byte
	switch cfg.Format {
	case "json":
		out, err = json.MarshalIndent(dto, "", "  ")
	case "yaml":
		out, err = yaml.Marshal(dto)
	default:
		return ErrFullCheckInvalidOutputFormat
	}

	if err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, out, 0644); err != nil {
		return err
	}

	return nil
}

func fullCheckWhoamiDto(r WhoamiResult, err error) FullCheckWhoamiDto {
	x := FullCheckWhoamiDto{
		Ip:       r.Ip,
		Subnet:   r.Subnet,
		Asn:      r.Asn,
		Org:      r.Org,
		Location: r.Location,
		TtlbMs:   r.Ttlb.Milliseconds(),
		Status:   FullCheckStatusDto{Msg: "Ok", Code: "OK"},
	}
	if err != nil {
		x.Status = FullCheckStatusDto{Msg: err.Error(), Code: "ERR"}
	}
	return x
}

func fullCheckCidrwhitelistDto(err error) FullCheckCidrwhitelistDto {
	if err == nil {
		return FullCheckCidrwhitelistDto{Status: FullCheckStatusDto{Msg: "You're NOT under one", Code: "NOT_DETECTED"}}
	}

	if err == ErrCidrWhitelistDetected {
		return FullCheckCidrwhitelistDto{Status: FullCheckStatusDto{Msg: "You're UNDER one", Code: "DETECTED"}}
	}

	if err == ErrCidrWhitelistNoInetAccess {
		return FullCheckCidrwhitelistDto{Status: FullCheckStatusDto{Msg: "It seems that there is no Internet access (even to resources from the whitelist)", Code: "NO_INTERNET_ACCESS"}}
	}

	return FullCheckCidrwhitelistDto{Status: FullCheckStatusDto{Msg: "Internal error", Code: "INTERNAL_ERR"}}
}

func fullCheckDnsLeakDto(outs []DnsLeakWithIpinfoOut) FullCheckDnsLeakDto {
	items := []inetlookup.IpInfoStrings{}
	for _, out := range outs {
		if out.Err != nil {
			return FullCheckDnsLeakDto{Status: FullCheckStatusDto{Msg: out.Err.Error(), Code: "ERR"}}
		}
		items = append(items, out.Items...)
	}

	slices.SortFunc(items, func(a, b inetlookup.IpInfoStrings) int {
		return strings.Compare(a.Ip, b.Ip)
	})
	items = slices.CompactFunc(items, func(a, b inetlookup.IpInfoStrings) bool { return a.Ip == b.Ip })
	return FullCheckDnsLeakDto{Status: FullCheckStatusDto{Msg: "Ok", Code: "OK"}, Items: items}
}

func fullCheckPrettyDnsVerdict(v error) (string, string) {
	if inetutil.IsInetutilErr(v) {
		return v.Error(), "INETUTIL_ERR"
	}
	if _, ok := errors.AsType[*net.DNSError](v); ok {
		return "Lookup error", "LOOKUP_ERR"
	}
	switch v {
	case nil:
		return "Ok", "OK"
	case ErrDnsNxdomainSpoofing:
		return "NXDOMAIN spoofing", "NXDOMAIN_SPOOFING"
	case ErrDnsResolveSpoofing:
		return "Response spoofing", "RESPONSE_SPOOFING"
	case ErrDnsDohBootstrapSpoofing:
		return "Bootstrap spoofing", "BOOTSTRAP_SPOOFING"
	case ErrDnsDohBootstrapEmpty:
		return "Empty bootstrap", "EMPTY_BOOTSTRAP"
	case ErrDnsDohInsecure, inetutil.ErrTlsCertificateInvalid:
		return "Invalid https certificate", "INVALID_HTTPS_CERT"
	case ErrDnsDohNon2xxResp:
		return "Non-2xx response", "NON_2XX_RESP"
	case ErrDnsSkip:
		return "Skip", "SKIP"
	default:
		return "Internal error", "INTERNAL_ERR"
	}
}

func fullCheckDnsReportDto(verdicts []DnsVerdict) FullCheckDnsReportDto {
	providers := map[string]FullCheckStatusDto{}
	for _, x := range verdicts {
		verdict, code := fullCheckPrettyDnsVerdict(x.Verdict)
		providers[x.Provider] = FullCheckStatusDto{Msg: verdict, Code: code}
	}
	return FullCheckDnsReportDto{Status: FullCheckStatusDto{Msg: "Ok", Code: "OK"}, Providers: providers}
}

func fullCheckWebhostItemDto(o WebhostGochanOut[WebhostGochanBag]) FullCheckWebhostItemDto {
	dto := FullCheckWebhostItemDto{
		Group:    o.Bag.Name,
		Org:      o.Out.IpInfo.Org,
		AS:       fmt.Sprintf("AS%d", o.Out.IpInfo.Asn),
		Location: o.Out.IpInfo.CountryIso,
		IP:       o.Out.IpInfo.Ip.String(),
		Prefix:   o.Out.IpInfo.Subnet.String(),
		Alive:    webhostPrettyAlive(o.Out.Alive),
		TlsV:     webhostPrettyTlsV(o.Out.TlsV),
		Tcp1620:  webhostPrettyTcp1620(o.Out.Tcp1620),
		Siberian: webhostPrettySiberian(o.Out.Siberian),
	}

	if o.Out.Throughput.TxElapsed > 0 && o.Out.Throughput.RxElapsed > 0 {
		thp := o.Out.Throughput
		const bytesToKilobits = 8.0 / 1_000
		txKbps := float64(o.Out.Throughput.TxBytes) / thp.TxElapsed.Seconds() * bytesToKilobits
		rxKbps := float64(o.Out.Throughput.RxBytes) / thp.RxElapsed.Seconds() * bytesToKilobits
		dto.BurstTxKbps = &txKbps
		dto.BurstRxKbps = &rxKbps
	}

	return dto
}

func webhostPrettyAlive(err error) FullCheckStatusDto {
	if err == nil {
		return FullCheckStatusDto{Msg: "Ok", Code: "OK"}
	}
	return FullCheckStatusDto{Msg: err.Error(), Code: "ERR"}
}

func webhostPrettyTlsV(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return " — "
	}
}

func webhostPrettyTcp1620(err error) FullCheckStatusDto {
	switch err {
	case nil:
		return FullCheckStatusDto{Msg: "No", Code: "OK"}
	case inetutil.ErrTcpWriteTimeout, inetutil.ErrTcpReadTimeout:
		return FullCheckStatusDto{Msg: "Detected", Code: "DETECTED"}
	case ErrWebhostSkip:
		return FullCheckStatusDto{Msg: "Skipped", Code: "SKIP"}
	case inetutil.ErrTlsWriteBrokenPipe:
		return FullCheckStatusDto{Msg: "Not supported by host", Code: "NOT_SUPPORTED_BY_HOST"}
	default:
		return FullCheckStatusDto{Msg: err.Error(), Code: "ERR"}
	}
}

func webhostPrettySiberian(err error) FullCheckStatusDto {
	switch err {
	case nil:
		return FullCheckStatusDto{Msg: "No", Code: "OK"}
	case inetutil.ErrTlsHandshakeTimeout, inetutil.ErrTlsHandshakeFail:
		return FullCheckStatusDto{Msg: "Detected", Code: "DETECTED"}
	case ErrWebhostSkip:
		return FullCheckStatusDto{Msg: "Skipped", Code: "SKIP"}
	default:
		return FullCheckStatusDto{Msg: err.Error(), Code: "ERR"}
	}
}

func fullCheckSendProgress(ch chan<- FullCheckProgress, p FullCheckProgress) {
	debug := config.Get().Debug
	select {
	case ch <- p:
		if debug {
			log.Println(p)
		}
	default:
	}
}
