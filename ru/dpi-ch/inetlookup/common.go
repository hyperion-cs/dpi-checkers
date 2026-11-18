package inetlookup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

var (
	mu     sync.Mutex
	def    InetLookup
	inited atomic.Bool

	cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")
)

func Inited() bool {
	return inited.Load()
}

func Default() InetLookup {
	lookup, err := initDefault()
	if err != nil {
		panic(err)
	}
	return lookup
}

// InitDefault loads the configured GeoLite files for subsequent lookup calls.
func InitDefault() error {
	_, err := initDefault()
	return err
}

func initDefault() (InetLookup, error) {
	mu.Lock()
	defer mu.Unlock()

	if def != nil {
		return def, nil
	}

	cfg := config.Get().InetlookupGeolitecsv
	lookup, err := loadGeoliteCsv(GeoliteCsvOpt{
		CidrAsPath:           cfg.CidrAs,
		CidrCountryPath:      cfg.CidrCountry,
		GeonameidCountryPath: cfg.GeonameidCountry,
	})
	if err != nil {
		return nil, fmt.Errorf("inetlookup/geolite: %w", err)
	}
	def = lookup
	inited.Store(true)
	return def, nil
}

func LookupIpViaDefault(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

func GetExternalIpViaMail(ctx context.Context) (netip.Addr, error) {
	const logPrefix = "inetlookup/mail"
	cfg := config.Get().InetLookup

	body, err := inetutil.Get(ctx, cfg.MailApiUrl+"ip.html", true, true)
	if err != nil {
		log.Println(logPrefix, err)
		return netip.Addr{}, err
	}

	ip, err := parseMailExternalIp(body)
	if err != nil {
		log.Println(logPrefix, err)
	}
	return ip, err
}

func GetExternalIpViaRipe(ctx context.Context) (netip.Addr, error) {
	const logPrefix = "inetlookup/ripe"
	cfg := config.Get().InetLookup

	var ipRaw struct{ Data struct{ Ip string } }
	if err := inetutil.GetAndUnmarshal(ctx, cfg.RipeApiUrl+"whats-my-ip/data.json", &ipRaw, true, true); err != nil {
		log.Println(logPrefix, err)
		return netip.Addr{}, err
	}

	ip, err := parseExternalIp(ipRaw.Data.Ip)
	if err != nil {
		log.Println(logPrefix, err)
	}
	return ip, err
}

func GetExternalIpViaYandex(ctx context.Context) (netip.Addr, error) {
	const logPrefix = "inetlookup/yandex"
	cfg := config.Get().InetLookup

	var ip string
	err := inetutil.GetAndUnmarshal(ctx, cfg.YandexApiUrl+"ip", &ip, true, true)
	if err != nil {
		log.Println(logPrefix, err)
		return netip.Addr{}, err
	}

	addr, err := parseExternalIp(ip)
	if err != nil {
		log.Println(logPrefix, err)
	}
	return addr, err
}

func parseMailExternalIp(body []byte) (netip.Addr, error) {
	const prefix, suffix = "(none)(", ")"
	body = bytes.TrimSpace(body)
	if !bytes.HasPrefix(body, []byte(prefix)) || !bytes.HasSuffix(body, []byte(suffix)) {
		return netip.Addr{}, errors.New("invalid mail IP response format")
	}

	var ipRaw struct{ IpAddress string }
	if err := json.Unmarshal(body[len(prefix):len(body)-len(suffix)], &ipRaw); err != nil {
		return netip.Addr{}, err
	}
	return parseExternalIp(ipRaw.IpAddress)
}

func parseExternalIp(raw string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, err
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || cgnatPrefix.Contains(ip.Unmap()) {
		return netip.Addr{}, fmt.Errorf("invalid external IP address: %s", ip)
	}
	return ip, nil
}
