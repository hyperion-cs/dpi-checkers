package checkers

import (
	"context"
	"dpich/config"
	"dpich/inetlookup"
	"fmt"
)

type WhoamiResult struct {
	Ip       string
	Subnet   string
	Asn      string
	Org      string
	Location string
}

func Whoami() (WhoamiResult, error) {
	cfg := config.Get().Checkers.Whoami
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	ip, err := inetlookup.GetExternalIpViaRipe(ctx)
	if err != nil {
		ip, err = inetlookup.GetExternalIpViaYandex(ctx)
	}
	if err != nil {
		return WhoamiResult{}, err
	}

	il := inetlookup.Default()
	info := il.IpInfo(ip)

	return WhoamiResult{
		Ip:       info.Ip.String(),
		Subnet:   info.Subnet.String(),
		Asn:      fmt.Sprintf("AS%d", info.Asn),
		Org:      info.Org,
		Location: info.CountryIso,
	}, nil
}
