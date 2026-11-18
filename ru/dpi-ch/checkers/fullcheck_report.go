package checkers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	yaml "go.yaml.in/yaml/v3"
)

const fullCheckVersion = 1

var ErrFullCheckInvalidOutputFormat = errors.New("all: invalid output format")
var ErrFullCheckUnknownReason = errors.New("Unknown")

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
	Providers map[string][]FullCheckStatusDto
}

type FullCheckDnsDto struct {
	Leak  *FullCheckDnsLeakDto
	Plain *FullCheckDnsReportDto
	Doh   *FullCheckDnsReportDto
}

type FullCheckWebhostTls struct {
	V   string
	San []string
	Cn  string
}

type FullCheckWebhostItemDto struct {
	Group       string
	Org         string
	AS          string
	Location    string
	IP          string
	Prefix      string
	Alive       FullCheckStatusDto
	Tls         *FullCheckWebhostTls
	Tcp1620     FullCheckStatusDto
	Siberian    FullCheckStatusDto
	BurstTxKbps *float64
	BurstRxKbps *float64
}

type FullCheckWebhostDto struct {
	Items       []FullCheckWebhostItemDto
	FarmTimeout bool
}

type FullCheckStatusDto struct {
	Msg    string
	Code   string
	Reason string
}

type FullCheckDto struct {
	Version       fullCheckFormatVersion
	Whoami        *FullCheckWhoamiDto
	CidrWhitelist *FullCheckCidrwhitelistDto
	Dns           *FullCheckDnsDto
	Webhost       map[string]FullCheckWebhostDto
}

type fullCheckFormatVersion int

func (v *fullCheckFormatVersion) UnmarshalYAML(node *yaml.Node) error {
	if node.ShortTag() != "!!int" {
		return errors.New("ALL report version must be an integer")
	}
	return node.Decode((*int)(v))
}

func FullCheckImport(file string) (*FullCheckDto, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read ALL report: %w", err)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	var report FullCheckDto
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
			return nil, errors.New("ALL report must contain a single document")
		}
	}
	switch report.Version {
	case 0:
		return nil, errors.New("ALL report uses an unversioned format")
	case fullCheckVersion:
	default:
		return nil, fmt.Errorf("unsupported ALL report version %d (expected %d)", report.Version, fullCheckVersion)
	}
	for name, section := range report.Webhost {
		if section.Items == nil && !section.FarmTimeout {
			delete(report.Webhost, name)
		}
	}
	if report.Whoami == nil && report.CidrWhitelist == nil && len(report.Webhost) == 0 && !report.Dns.HasResults() {
		return nil, errors.New("ALL report contains no checker results")
	}
	return &report, nil
}

func (r *FullCheckDnsDto) HasResults() bool {
	return r != nil && (r.Leak != nil || r.Plain != nil || r.Doh != nil)
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

func fullCheckSave(reportPath string, dto *FullCheckDto) error {
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
		Status:   fullCheckStatus(nil, "OK", "Ok"),
	}
	if err != nil {
		x.Status = fullCheckStatus(err, "ERR", err.Error())
	}
	return x
}

func fullCheckCidrwhitelistDto(err error) FullCheckCidrwhitelistDto {
	var status FullCheckStatusDto
	switch err {
	case nil:
		status = fullCheckStatus(err, "NOT_DETECTED", "You're NOT under one")
	case ErrCidrWhitelistDetected:
		status = fullCheckStatus(err, "DETECTED", "You're UNDER one")
	case ErrCidrWhitelistNoInetAccess:
		status = fullCheckStatus(err, "NO_INTERNET_ACCESS", "It seems that there is no Internet access (even to resources from the whitelist)")
	default:
		status = fullCheckStatus(err, "INTERNAL_ERR", "Internal error")
	}
	return FullCheckCidrwhitelistDto{Status: status}
}

func fullCheckDnsLeakDto(outs []DnsLeakWithIpinfoOut) FullCheckDnsLeakDto {
	items := []inetlookup.IpInfoStrings{}
	for _, out := range outs {
		if out.Err != nil {
			return FullCheckDnsLeakDto{Status: fullCheckStatus(out.Err, "ERR", out.Err.Error())}
		}
		items = append(items, out.Items...)
	}

	slices.SortFunc(items, func(a, b inetlookup.IpInfoStrings) int {
		return strings.Compare(a.Ip, b.Ip)
	})
	items = slices.CompactFunc(items, func(a, b inetlookup.IpInfoStrings) bool { return a.Ip == b.Ip })
	return FullCheckDnsLeakDto{Status: fullCheckStatus(nil, "OK", "Ok"), Items: items}
}

func fullCheckPrettyDnsVerdict(verdict error) []FullCheckStatusDto {
	dnsErrs := DnsErrors(verdict)
	if len(dnsErrs) == 0 {
		return []FullCheckStatusDto{fullCheckDnsStatus(verdict)}
	}
	// TODO: disable this when HTTP/2 is supported.
	// It only works as long as we trust providers that use DoH.
	if len(dnsErrs) == 1 && dnsErrs[0] == ErrDnsDohNon2xxResp {
		return []FullCheckStatusDto{fullCheckStatus(dnsErrs[0], "OK", "Ok")}
	}

	statuses := make([]FullCheckStatusDto, len(dnsErrs))
	for i, err := range dnsErrs {
		statuses[i] = fullCheckDnsStatus(err)
	}
	return statuses
}

func fullCheckDnsStatus(err error) FullCheckStatusDto {
	switch err {
	case nil:
		return fullCheckStatus(err, "OK", "Ok")
	case ErrDnsProviderHijacking:
		return fullCheckStatus(err, "PROVIDER_HIJACKING", "Provider hijacking")
	case ErrDnsResolveSpoofing:
		return fullCheckStatus(err, "RESPONSE_SPOOFING", "Response spoofing")
	case ErrDnsNxdomainSpoofing:
		return fullCheckStatus(err, "NXDOMAIN_SPOOFING", "NXDOMAIN spoofing")
	case ErrDnsDohBootstrapSpoofing:
		return fullCheckStatus(err, "BOOTSTRAP_SPOOFING", "Bootstrap spoofing")
	case ErrDnsDohBootstrapEmpty:
		return fullCheckStatus(err, "EMPTY_BOOTSTRAP", "Empty bootstrap")
	case ErrDnsDohInsecure:
		return fullCheckStatus(err, "INVALID_HTTPS_CERT", "Invalid https certificate")
	case ErrDnsDohNon2xxResp:
		return fullCheckStatus(err, "NON_2XX_RESP", "Non-2xx response")
	case ErrDnsProviderTimeout:
		return fullCheckStatus(err, "PROVIDER_TIMEOUT", "Provider timeout")
	case ErrDnsSkip:
		return fullCheckStatus(err, "SKIP", "Skip")
	}
	if inetutil.IsInetutilErr(err) {
		return fullCheckStatus(err, "INETUTIL_ERR", err.Error())
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return fullCheckStatus(err, "LOOKUP_ERR", "Lookup error")
	}
	return fullCheckStatus(err, "INTERNAL_ERR", "Internal error")
}

func fullCheckDnsReportDto(verdicts []DnsVerdict) FullCheckDnsReportDto {
	providers := map[string][]FullCheckStatusDto{}
	for _, x := range verdicts {
		providers[x.Provider] = fullCheckPrettyDnsVerdict(x.Verdict)
	}
	return FullCheckDnsReportDto{Status: fullCheckStatus(nil, "OK", "Ok"), Providers: providers}
}

func fullCheckWebhostItemDto(o WebhostGochanOut[WebhostGochanBag]) FullCheckWebhostItemDto {
	info := inetlookup.IpInfoAsStrings(o.Out.IpInfo)
	dto := FullCheckWebhostItemDto{
		Group:    o.Bag.Name,
		Org:      info.Org,
		AS:       info.Asn,
		Location: info.Location,
		IP:       info.Ip,
		Prefix:   info.Subnet,
		Alive:    fullCheckWebhostAliveStatus(o.Out.Alive),
		Tcp1620:  fullCheckWebhostTcp1620Status(o.Out.Tcp1620),
		Siberian: fullCheckWebhostSiberianStatus(o.Out.Siberian),
	}

	if o.Out.Tls != nil {
		dto.Tls = &FullCheckWebhostTls{V: o.Out.Tls.VersionString(), San: o.Out.Tls.San, Cn: o.Out.Tls.Cn}
	}

	dto.BurstTxKbps, dto.BurstRxKbps = o.Out.Throughput.Kbps()

	return dto
}

func fullCheckWebhostAliveStatus(err error) FullCheckStatusDto {
	switch err {
	case nil:
		return fullCheckStatus(err, "OK", "Ok")
	case inetutil.ErrHttpMalformedResponse:
		return fullCheckStatus(err, "OK_CUSTOM_HTTP", "Ok, custom HTTP")
	default:
		return fullCheckStatus(err, "ERR", err.Error())
	}
}

func fullCheckWebhostTcp1620Status(err error) FullCheckStatusDto {
	switch err {
	case inetutil.ErrTcpReadTimeout, inetutil.ErrTcpWriteTimeout:
		return fullCheckStatus(err, "DETECTED", "Detected")
	case inetutil.ErrTlsWriteBrokenPipe:
		return fullCheckStatus(err, "NOT_SUPPORTED_BY_HOST", "Not supported by host")
	default:
		return fullCheckWebhostDetectionStatus(err)
	}
}

func fullCheckWebhostSiberianStatus(err error) FullCheckStatusDto {
	switch err {
	case inetutil.ErrTlsHandshakeTimeout, inetutil.ErrTlsHandshakeFail:
		return fullCheckStatus(err, "DETECTED", "Detected")
	default:
		return fullCheckWebhostDetectionStatus(err)
	}
}

func fullCheckWebhostDetectionStatus(err error) FullCheckStatusDto {
	switch err {
	case nil:
		return fullCheckStatus(err, "OK", "No")
	case ErrWebhostSkip:
		return fullCheckStatus(err, "SKIP", "Skipped")
	default:
		return fullCheckStatus(err, "ERR", err.Error())
	}
}

func fullCheckStatus(err error, code, msg string) FullCheckStatusDto {
	return FullCheckStatusDto{Msg: msg, Code: code, Reason: fullCheckReason(err)}
}

// Reason codes are part of the report format and survive Go symbol renames.
var fullCheckReasons = [...]struct {
	err    error
	reason string
}{
	{nil, "NONE"},
	{ErrCidrWhitelistDetected, "CHECKERS_ERR_CIDR_WHITELIST_DETECTED"},
	{ErrCidrWhitelistNoInetAccess, "CHECKERS_ERR_CIDR_WHITELIST_NO_INET_ACCESS"},
	{ErrDnsProviderHijacking, "CHECKERS_ERR_DNS_PROVIDER_HIJACKING"},
	{ErrDnsResolveSpoofing, "CHECKERS_ERR_DNS_RESOLVE_SPOOFING"},
	{ErrDnsNxdomainSpoofing, "CHECKERS_ERR_DNS_NXDOMAIN_SPOOFING"},
	{ErrDnsDohBootstrapSpoofing, "CHECKERS_ERR_DNS_DOH_BOOTSTRAP_SPOOFING"},
	{ErrDnsDohBootstrapEmpty, "CHECKERS_ERR_DNS_DOH_BOOTSTRAP_EMPTY"},
	{ErrDnsDohInsecure, "CHECKERS_ERR_DNS_DOH_INSECURE"},
	{ErrDnsDohNon2xxResp, "CHECKERS_ERR_DNS_DOH_NON_2XX_RESP"},
	{ErrDnsSkip, "CHECKERS_ERR_DNS_SKIP"},
	{ErrDnsProviderTimeout, "CHECKERS_ERR_DNS_PROVIDER_TIMEOUT"},
	{ErrWebhostBlockedBySni, "CHECKERS_ERR_WEBHOST_BLOCKED_BY_SNI"},
	{ErrWebhostInternal, "CHECKERS_ERR_WEBHOST_INTERNAL"},
	{ErrWebhostSkip, "CHECKERS_ERR_WEBHOST_SKIP"},
	{inetutil.ErrTcpConnReset, "INETUTIL_ERR_TCP_CONN_RESET"},
	{inetutil.ErrTcpConnTimeout, "INETUTIL_ERR_TCP_CONN_TIMEOUT"},
	{inetutil.ErrTcpWriteTimeout, "INETUTIL_ERR_TCP_WRITE_TIMEOUT"},
	{inetutil.ErrTcpReadTimeout, "INETUTIL_ERR_TCP_READ_TIMEOUT"},
	{inetutil.ErrTlsCertificateInvalid, "INETUTIL_ERR_TLS_CERTIFICATE_INVALID"},
	{inetutil.ErrTlsHandshakeTimeout, "INETUTIL_ERR_TLS_HANDSHAKE_TIMEOUT"},
	{inetutil.ErrTlsHandshakeFail, "INETUTIL_ERR_TLS_HANDSHAKE_FAIL"},
	{inetutil.ErrTlsInternal, "INETUTIL_ERR_TLS_INTERNAL"},
	{inetutil.ErrTlsBadRecordMac, "INETUTIL_ERR_TLS_BAD_RECORD_MAC"},
	{inetutil.ErrTlsInvalidKeyShare, "INETUTIL_ERR_TLS_INVALID_KEY_SHARE"},
	{inetutil.ErrTlsWriteBrokenPipe, "INETUTIL_ERR_TLS_WRITE_BROKEN_PIPE"},
	{inetutil.ErrHttpMalformedResponse, "INETUTIL_ERR_HTTP_MALFORMED_RESPONSE"},
	{inetutil.ErrInternal, "INETUTIL_ERR_INTERNAL"},
}

func fullCheckReason(err error) string {
	for _, entry := range fullCheckReasons {
		if entry.err == err {
			return entry.reason
		}
	}
	return "UNKNOWN"
}

func FullCheckErrorFromReason(reason string) error {
	for _, entry := range fullCheckReasons {
		if entry.reason == reason {
			return entry.err
		}
	}
	return ErrFullCheckUnknownReason
}
