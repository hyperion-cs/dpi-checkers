package checkers

import (
	"context"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
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

	ip, err := inetlookup.GetExternalIpViaYandex(ctx)
	if err != nil {
		ip, err = inetlookup.GetExternalIpViaRipe(ctx)
	}
	if err != nil {
		return WhoamiResult{}, err
	}

	il := inetlookup.Default()
	il.IpInfo(ip)

	return WhoamiResult{
		Ip:       "3.3.3.3",
		Subnet:   "3.3.3.3/24",
		Asn:      "AS14618",
		Org:      "Amazon.com, Inc.",
		Location: "US",
	}, nil
}
