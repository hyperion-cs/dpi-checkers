package checkers

import (
	"context"
	"dpich/config"
	"dpich/netutil"
)

type WhoamiResult struct {
	Ip       string
	Subnet   string
	Asn      string
	Holder   string
	Location string
}

func Whoami() (WhoamiResult, error) {
	if res, err := viaRipe(); err == nil {
		return res, nil
	}

	return viaYandex()
}

func viaRipe() (WhoamiResult, error) {
	cfg := config.Get().Checkers.Whoami
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	ip, err := netutil.GetExternalIpViaRipe(ctx)
	if err != nil {
		return WhoamiResult{}, nil
	}

	as, err := netutil.GetAsViaRipe(ctx, ip)
	if err != nil {
		as = netutil.As{Asn: "—", Holder: "—", Subnet: "—"}
	}

	loc, err := netutil.GetLocationViaRipe(ctx, ip)
	if err != nil {
		loc = netutil.Location{Country: "—", City: "—"}
	}

	return WhoamiResult{
		Ip:       ip,
		Asn:      as.Asn,
		Holder:   as.Holder,
		Subnet:   as.Subnet,
		Location: loc.Country + ", " + loc.City}, nil
}

// Probably, this will work with restrictions of the "cidr whitelist" type.
// Only IP address available.
func viaYandex() (WhoamiResult, error) {
	// TODO: We can also extract ASN from https://yandex.ru/internet/,
	// but the body is over 100KB and we'll have to use regexp.
	cfg := config.Get().Checkers.Whoami
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	ip, err := netutil.GetExternalIpViaYandex(ctx)
	if err != nil {
		return WhoamiResult{}, err
	}

	return WhoamiResult{Ip: ip, Holder: "—", Subnet: "—", Asn: "—", Location: "—"}, nil
}
