package inetlookup

import (
	"context"
	"dpich/config"
	"dpich/httputil"
	"net"
	"net/http"
	"net/netip"
)

var defaultInetLookup InetLookup

// TODO: lock and startup that
func Default() InetLookup {
	if defaultInetLookup == nil {
		ilOpt := GeoliteCsvOpt{
			Cidr2asPath:              "/Users/hyperion/Repositories/dpi-checkers/lab/geolite2_csv/asn_ipv4.csv",
			Cidr2countryIsoPath:      "/Users/hyperion/Repositories/dpi-checkers/lab/geolite2_csv/country_ipv4.csv",
			GeonameId2countryIsoPath: "/Users/hyperion/Repositories/dpi-checkers/lab/geolite2_csv/countrylocations_en.csv",
		}
		defaultInetLookup = NewGeoliteCsv(ilOpt)
	}

	return defaultInetLookup
}

func LookupIpViaDefault(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

func GetExternalIpViaRipe(ctx context.Context) (netip.Addr, error) {
	cfg := config.Get().Inetlookup

	var ipRaw struct{ Data struct{ Ip string } }
	if err := httputil.GetAndUnmarshal(ctx, http.DefaultClient, cfg.RipeApiUrl+"whats-my-ip/data.json", &ipRaw, true); err != nil {
		return netip.Addr{}, err
	}

	return netip.ParseAddr(ipRaw.Data.Ip)
}

func GetExternalIpViaYandex(ctx context.Context) (netip.Addr, error) {
	cfg := config.Get().Inetlookup

	var ip string
	err := httputil.GetAndUnmarshal(ctx, http.DefaultClient, cfg.YandexApiUrl+"ip", &ip, true)
	if err != nil {
		return netip.Addr{}, err
	}

	return netip.ParseAddr(ip)
}
