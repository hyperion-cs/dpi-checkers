package netutil

import (
	"context"
	"dpich/config"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type As struct {
	Asn    string
	Holder string
	Subnet string
}

type Location struct {
	Country string
	City    string
}

func GetExternalIpViaRipe(ctx context.Context) (string, error) {
	cfg := config.Get().Netutils
	ctx, cancel := ctxOrDefault(ctx)
	defer cancel()

	var ipRaw struct{ Data struct{ Ip string } }
	if err := GetAndUnmarshal(ctx, http.DefaultClient, cfg.RipeApiUrl+"whats-my-ip/data.json", &ipRaw, true); err != nil {
		return "", err
	}

	return ipRaw.Data.Ip, nil
}

func GetExternalIpViaYandex(ctx context.Context) (string, error) {
	cfg := config.Get().Netutils
	ctx, cancel := ctxOrDefault(ctx)
	defer cancel()

	var ip string
	err := GetAndUnmarshal(ctx, http.DefaultClient, cfg.YandexApiUrl+"ip", &ip, true)
	if err == nil {
		return ip, nil
	}

	return "", err
}

func LookupIpViaDefault(ctx context.Context, host string) ([]net.IP, error) {
	ctx, cancel := ctxOrDefault(ctx)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

func GetAsViaRipe(ctx context.Context, ip string) (As, error) {
	cfg := config.Get().Netutils
	ctx, cancel := ctxOrDefault(ctx)
	defer cancel()

	var asRaw struct {
		Data struct {
			Asns []struct {
				Asn    int
				Holder string
			}
			Resource string
		}
	}
	if err := GetAndUnmarshal(ctx, http.DefaultClient, cfg.RipeApiUrl+"prefix-overview/data.json?resource="+ip, &asRaw, true); err != nil {
		return As{}, err
	}

	as := As{}
	if len(asRaw.Data.Asns) > 0 {
		first := asRaw.Data.Asns[0]
		as.Asn = strconv.Itoa(first.Asn)

		// We believe that the holder is passed in the format "X — Y"
		holderParts := strings.Split(first.Holder, " - ")
		as.Holder = holderParts[len(holderParts)-1]
	}

	as.Subnet = asRaw.Data.Resource
	return as, nil
}

func GetLocationViaRipe(ctx context.Context, ip string) (Location, error) {
	cfg := config.Get().Netutils
	ctx, cancel := ctxOrDefault(ctx)
	defer cancel()

	var locRaw struct {
		Data struct {
			LocatedResources []struct {
				Locations []struct {
					Country string
					City    string
				}
			} `json:"located_resources"`
		}
	}
	if err := GetAndUnmarshal(ctx, http.DefaultClient, cfg.RipeApiUrl+"maxmind-geo-lite/data.json?resource="+ip, &locRaw, true); err != nil {
		return Location{}, err
	}

	loc := Location{}
	if len(locRaw.Data.LocatedResources) > 0 && len(locRaw.Data.LocatedResources[0].Locations) > 0 {
		first := locRaw.Data.LocatedResources[0].Locations[0]
		loc.Country = first.Country

		if first.City == "" {
			loc.City = "—"
		} else {
			loc.City = first.City
		}
	}

	return loc, nil
}
