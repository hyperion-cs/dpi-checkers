package inetlookup

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

var mu sync.Mutex
var def InetLookup

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
		if !fileExists(ilOpt.CidrAsPath) || !fileExists(ilOpt.CidrCountryPath) || !fileExists(ilOpt.CidrCountryPath) {
			panic(
				fmt.Sprintf("inetlookup/geolite: some .csv files are missing in ./%s; try running the utility with the --force-inetlookup-update flag",
					path.Dir(ilOpt.CidrAsPath)),
			)
		}
		def = NewGeoliteCsv(ilOpt)
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
	cfg := config.Get().InetLookup

	var ipRaw struct{ Data struct{ Ip string } }
	if err := inetutil.GetAndUnmarshal(ctx, cfg.RipeApiUrl+"whats-my-ip/data.json", &ipRaw, true, true); err != nil {
		return netip.Addr{}, err
	}

	return netip.ParseAddr(ipRaw.Data.Ip)
}

func GetExternalIpViaYandex(ctx context.Context) (netip.Addr, error) {
	cfg := config.Get().InetLookup

	var ip string
	err := inetutil.GetAndUnmarshal(ctx, cfg.YandexApiUrl+"ip", &ip, true, true)
	if err != nil {
		return netip.Addr{}, err
	}

	return netip.ParseAddr(ip)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
