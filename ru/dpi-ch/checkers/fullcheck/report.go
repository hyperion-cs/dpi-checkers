package fullcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	yaml "go.yaml.in/yaml/v3"
)

const version = 1

var errInvalidOutputFormat = errors.New("fullcheck: invalid output format")
var ErrUnknownReason = errors.New("fullcheck: unknown reason")

type WhoamiDto struct {
	Status   StatusDto
	Ip       string
	Subnet   string
	Asn      string
	Org      string
	Location string
	TtlbMs   int64
}

type CidrWhitelistDto struct {
	Status StatusDto
}

type DnsLeakDto struct {
	Status StatusDto
	Items  []inetlookup.IpInfoStrings
}

type DnsReportDto struct {
	Status    StatusDto
	Providers map[string][]StatusDto
}

type DnsDto struct {
	Leak  *DnsLeakDto
	Plain *DnsReportDto
	Doh   *DnsReportDto
}

type WebhostTlsDto struct {
	V   string
	San []string
	Cn  string
}

type WebhostItemDto struct {
	Group       string
	Org         string
	AS          string
	Location    string
	IP          string
	Prefix      string
	Alive       StatusDto
	Tls         *WebhostTlsDto
	Tcp1620     StatusDto
	Siberian    StatusDto
	BurstTxKbps *float64
	BurstRxKbps *float64
}

type WebhostDto struct {
	Items       []WebhostItemDto
	FarmTimeout bool
}

type StatusDto struct {
	Msg    string
	Code   string
	Reason string
}

type ReportDto struct {
	Version       formatVersion
	Whoami        *WhoamiDto
	CidrWhitelist *CidrWhitelistDto
	Dns           *DnsDto
	Webhost       map[string]WebhostDto
}

type formatVersion int

func (v *formatVersion) UnmarshalYAML(node *yaml.Node) error {
	if node.ShortTag() != "!!int" {
		return errors.New("fullcheck: report version must be an integer")
	}
	return node.Decode((*int)(v))
}

func Import(file string) (*ReportDto, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read ALL report: %w", err)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	var report ReportDto
	if err := json.Unmarshal(raw, &report); err != nil {
		if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
			return nil, fmt.Errorf("decode ALL report: %w", err)
		}
		decoder := yaml.NewDecoder(bytes.NewReader(raw))
		if err := decoder.Decode(&report); err != nil {
			return nil, fmt.Errorf("decode ALL report: %w", err)
		}
		var extra yaml.Node
		if err := decoder.Decode(&extra); err != io.EOF {
			if err != nil {
				return nil, fmt.Errorf("decode trailing ALL report data: %w", err)
			}
			return nil, errors.New("fullcheck: report must contain a single document")
		}
	}
	switch report.Version {
	case 0:
		return nil, errors.New("fullcheck: report uses an unversioned format")
	case version:
	default:
		return nil, fmt.Errorf("unsupported ALL report version %d (expected %d)", report.Version, version)
	}
	for name, section := range report.Webhost {
		if section.Items == nil && !section.FarmTimeout {
			delete(report.Webhost, name)
		}
	}
	if report.Whoami == nil && report.CidrWhitelist == nil && len(report.Webhost) == 0 && !report.Dns.HasResults() {
		return nil, errors.New("fullcheck: report contains no checker results")
	}
	return &report, nil
}

func (r *DnsDto) HasResults() bool {
	return r != nil && (r.Leak != nil || r.Plain != nil || r.Doh != nil)
}

func savePath() (string, error) {
	cfg := config.Get().All
	reportPath := cfg.Prefix + time.Now().Format(cfg.TsFormat) + "." + cfg.Format
	return filepath.Abs(reportPath)
}

func save(reportPath string, dto *ReportDto) error {
	var (
		out []byte
		err error
	)
	switch config.Get().All.Format {
	case "json":
		out, err = json.MarshalIndent(dto, "", "  ")
	case "yaml":
		out, err = yaml.Marshal(dto)
	default:
		return errInvalidOutputFormat
	}

	if err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, out, 0644); err != nil {
		return err
	}

	return nil
}

func whoamiDto(r whoami.Result, err error) WhoamiDto {
	x := WhoamiDto{
		Ip:       r.Ip,
		Subnet:   r.Subnet,
		Asn:      r.Asn,
		Org:      r.Org,
		Location: r.Location,
		TtlbMs:   r.Ttlb.Milliseconds(),
		Status:   statusDto(nil, "OK", "Ok"),
	}
	if err != nil {
		x.Status = statusDto(err, "ERR", err.Error())
	}
	return x
}

func cidrWhitelistDto(err error) CidrWhitelistDto {
	var status StatusDto
	switch err {
	case nil:
		status = statusDto(err, "NOT_DETECTED", "You're NOT under one")
	case cidrwhitelist.ErrDetected:
		status = statusDto(err, "DETECTED", "You're UNDER one")
	case cidrwhitelist.ErrNoInetAccess:
		status = statusDto(err, "NO_INTERNET_ACCESS", "It seems that there is no Internet access (even to resources from the whitelist)")
	default:
		status = statusDto(err, "INTERNAL_ERR", "Internal error")
	}
	return CidrWhitelistDto{Status: status}
}

func dnsLeakDto(outs []dns.LeakResult) DnsLeakDto {
	items := []inetlookup.IpInfoStrings{}
	for _, out := range outs {
		if out.Err != nil {
			return DnsLeakDto{Status: statusDto(out.Err, "ERR", out.Err.Error())}
		}
		items = append(items, out.Items...)
	}

	slices.SortFunc(items, func(a, b inetlookup.IpInfoStrings) int {
		return strings.Compare(a.Ip, b.Ip)
	})
	items = slices.CompactFunc(items, func(a, b inetlookup.IpInfoStrings) bool { return a.Ip == b.Ip })
	return DnsLeakDto{Status: statusDto(nil, "OK", "Ok"), Items: items}
}

func dnsStatuses(verdict error) []StatusDto {
	dnsErrs := dns.Errors(verdict)
	if len(dnsErrs) == 0 {
		return []StatusDto{dnsStatus(verdict)}
	}
	// TODO: disable this when HTTP/2 is supported.
	// It only works as long as we trust providers that use DoH.
	if len(dnsErrs) == 1 && dnsErrs[0] == dns.ErrDohNon2xxResp {
		return []StatusDto{statusDto(dnsErrs[0], "OK", "Ok")}
	}

	statuses := make([]StatusDto, len(dnsErrs))
	for i, err := range dnsErrs {
		statuses[i] = dnsStatus(err)
	}
	return statuses
}

func dnsStatus(err error) StatusDto {
	switch err {
	case nil:
		return statusDto(err, "OK", "Ok")
	case dns.ErrProviderHijacking:
		return statusDto(err, "PROVIDER_HIJACKING", "Provider hijacking")
	case dns.ErrResolveSpoofing:
		return statusDto(err, "RESPONSE_SPOOFING", "Response spoofing")
	case dns.ErrNxdomainSpoofing:
		return statusDto(err, "NXDOMAIN_SPOOFING", "NXDOMAIN spoofing")
	case dns.ErrDohBootstrapSpoofing:
		return statusDto(err, "BOOTSTRAP_SPOOFING", "Bootstrap spoofing")
	case dns.ErrDohBootstrapEmpty:
		return statusDto(err, "EMPTY_BOOTSTRAP", "Empty bootstrap")
	case dns.ErrDohInsecure:
		return statusDto(err, "INVALID_HTTPS_CERT", "Invalid https certificate")
	case dns.ErrDohNon2xxResp:
		return statusDto(err, "NON_2XX_RESP", "Non-2xx response")
	case dns.ErrProviderTimeout:
		return statusDto(err, "PROVIDER_TIMEOUT", "Provider timeout")
	case dns.ErrSkip:
		return statusDto(err, "SKIP", "Skip")
	}
	if inetutil.IsInetutilErr(err) {
		return statusDto(err, "INETUTIL_ERR", err.Error())
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return statusDto(err, "LOOKUP_ERR", "Lookup error")
	}
	return statusDto(err, "INTERNAL_ERR", "Internal error")
}

func dnsReportDto(verdicts []dns.Verdict) DnsReportDto {
	providers := map[string][]StatusDto{}
	for _, x := range verdicts {
		providers[x.Provider] = dnsStatuses(x.Verdict)
	}
	return DnsReportDto{Status: statusDto(nil, "OK", "Ok"), Providers: providers}
}

func webhostItemDto(o webhost.Item) WebhostItemDto {
	info := inetlookup.IpInfoAsStrings(o.Result.IpInfo)
	dto := WebhostItemDto{
		Group:    o.Group,
		Org:      info.Org,
		AS:       info.Asn,
		Location: info.Location,
		IP:       info.Ip,
		Prefix:   info.Subnet,
		Alive:    webhostAliveStatus(o.Result.Alive),
		Tcp1620:  tcp1620Status(o.Result.Tcp1620),
		Siberian: siberianStatus(o.Result.Siberian),
	}

	if o.Result.Tls != nil {
		dto.Tls = &WebhostTlsDto{V: o.Result.Tls.VersionString(), San: o.Result.Tls.San, Cn: o.Result.Tls.Cn}
	}

	dto.BurstTxKbps, dto.BurstRxKbps = o.Result.Throughput.Kbps()

	return dto
}

func webhostAliveStatus(err error) StatusDto {
	switch err {
	case nil:
		return statusDto(err, "OK", "Ok")
	case inetutil.ErrHttpMalformedResponse:
		return statusDto(err, "OK_CUSTOM_HTTP", "Ok, custom HTTP")
	case webhost.ErrBlockedBySni:
		return statusDto(err, "ERR", "TLS blocked by SNI")
	default:
		return statusDto(err, "ERR", err.Error())
	}
}

func tcp1620Status(err error) StatusDto {
	switch err {
	case inetutil.ErrTcpReadTimeout, inetutil.ErrTcpWriteTimeout:
		return statusDto(err, "DETECTED", "Detected")
	case inetutil.ErrTlsWriteBrokenPipe:
		return statusDto(err, "NOT_SUPPORTED_BY_HOST", "Not supported by host")
	default:
		return webhostDetectionStatus(err)
	}
}

func siberianStatus(err error) StatusDto {
	switch err {
	case inetutil.ErrTlsHandshakeTimeout, inetutil.ErrTlsHandshakeFail:
		return statusDto(err, "DETECTED", "Detected")
	default:
		return webhostDetectionStatus(err)
	}
}

func webhostDetectionStatus(err error) StatusDto {
	switch err {
	case nil:
		return statusDto(err, "OK", "No")
	case webhost.ErrSkip:
		return statusDto(err, "SKIP", "Skipped")
	default:
		return statusDto(err, "ERR", err.Error())
	}
}

func statusDto(err error, code, msg string) StatusDto {
	return StatusDto{Msg: msg, Code: code, Reason: reasonFor(err)}
}

// Reason codes are part of the report format and survive Go symbol renames.
var reasons = [...]struct {
	reason string
	err    error
}{
	{"NONE", nil},
	{"CHECKERS_CIDRWHITELIST_ERR_DETECTED", cidrwhitelist.ErrDetected},
	{"CHECKERS_CIDRWHITELIST_ERR_NO_INET_ACCESS", cidrwhitelist.ErrNoInetAccess},
	{"CHECKERS_DNS_ERR_PROVIDER_HIJACKING", dns.ErrProviderHijacking},
	{"CHECKERS_DNS_ERR_RESOLVE_SPOOFING", dns.ErrResolveSpoofing},
	{"CHECKERS_DNS_ERR_NXDOMAIN_SPOOFING", dns.ErrNxdomainSpoofing},
	{"CHECKERS_DNS_ERR_DOH_BOOTSTRAP_SPOOFING", dns.ErrDohBootstrapSpoofing},
	{"CHECKERS_DNS_ERR_DOH_BOOTSTRAP_EMPTY", dns.ErrDohBootstrapEmpty},
	{"CHECKERS_DNS_ERR_DOH_INSECURE", dns.ErrDohInsecure},
	{"CHECKERS_DNS_ERR_DOH_NON_2XX_RESP", dns.ErrDohNon2xxResp},
	{"CHECKERS_DNS_ERR_SKIP", dns.ErrSkip},
	{"CHECKERS_DNS_ERR_PROVIDER_TIMEOUT", dns.ErrProviderTimeout},
	{"CHECKERS_WEBHOST_ERR_BLOCKED_BY_SNI", webhost.ErrBlockedBySni},
	{"CHECKERS_WEBHOST_ERR_INTERNAL", webhost.ErrInternal},
	{"CHECKERS_WEBHOST_ERR_SKIP", webhost.ErrSkip},
	{"INETUTIL_ERR_TCP_CONN_RESET", inetutil.ErrTcpConnReset},
	{"INETUTIL_ERR_TCP_CONN_TIMEOUT", inetutil.ErrTcpConnTimeout},
	{"INETUTIL_ERR_TCP_WRITE_TIMEOUT", inetutil.ErrTcpWriteTimeout},
	{"INETUTIL_ERR_TCP_READ_TIMEOUT", inetutil.ErrTcpReadTimeout},
	{"INETUTIL_ERR_TLS_CERTIFICATE_INVALID", inetutil.ErrTlsCertificateInvalid},
	{"INETUTIL_ERR_TLS_HANDSHAKE_TIMEOUT", inetutil.ErrTlsHandshakeTimeout},
	{"INETUTIL_ERR_TLS_HANDSHAKE_FAIL", inetutil.ErrTlsHandshakeFail},
	{"INETUTIL_ERR_TLS_INTERNAL", inetutil.ErrTlsInternal},
	{"INETUTIL_ERR_TLS_BAD_RECORD_MAC", inetutil.ErrTlsBadRecordMac},
	{"INETUTIL_ERR_TLS_INVALID_KEY_SHARE", inetutil.ErrTlsInvalidKeyShare},
	{"INETUTIL_ERR_TLS_WRITE_BROKEN_PIPE", inetutil.ErrTlsWriteBrokenPipe},
	{"INETUTIL_ERR_HTTP_MALFORMED_RESPONSE", inetutil.ErrHttpMalformedResponse},
	{"INETUTIL_ERR_INTERNAL", inetutil.ErrInternal},
}

func reasonFor(err error) string {
	for _, entry := range reasons {
		if entry.err == err {
			return entry.reason
		}
	}
	return "UNKNOWN"
}

func ErrorFromReason(reason string) error {
	for _, entry := range reasons {
		if entry.reason == reason {
			return entry.err
		}
	}
	return ErrUnknownReason
}
