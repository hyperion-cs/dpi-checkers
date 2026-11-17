package inetlookup

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path"
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
	mu.Lock()
	defer mu.Unlock()

	geolitecsvCfg := config.Get().InetlookupGeolitecsv
	if def == nil {
		ilOpt := GeoliteCsvOpt{
			CidrAsPath:           geolitecsvCfg.CidrAs,
			CidrCountryPath:      geolitecsvCfg.CidrCountry,
			GeonameidCountryPath: geolitecsvCfg.GeonameidCountry,
		}
		if !fileExists(ilOpt.CidrAsPath) || !fileExists(ilOpt.CidrCountryPath) || !fileExists(ilOpt.GeonameidCountryPath) {
			panic(
				fmt.Sprintf("inetlookup/geolite: some .csv files are missing in ./%s; try running the utility with the --force-inetlookup-update flag",
					path.Dir(ilOpt.CidrAsPath)),
			)
		}
		def = NewGeoliteCsv(ilOpt)
		inited.Store(true)
	}

	return def
}

func LookupIpViaDefault(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
