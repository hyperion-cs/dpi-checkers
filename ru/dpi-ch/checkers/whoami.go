package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
)

type WhoamiResult struct {
	Ip       string
	Subnet   string
	Asn      string
	Org      string
	Location string
	Ttlb     time.Duration
}

func Whoami() (WhoamiResult, error) {
	cfg := config.Get().Checkers.Whoami
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	ttlbStart := time.Now()
	ip, err := inetlookup.GetExternalIpViaYandex(ctx)
	if err != nil {
		ttlbStart = time.Now()
		ip, err = inetlookup.GetExternalIpViaRipe(ctx)
	}
	if err != nil {
		return WhoamiResult{}, err
	}
	ttlb := time.Since(ttlbStart)

	il := inetlookup.Default()
	info := il.IpInfo(ip)

	return WhoamiResult{
		Ip:       info.Ip.String(),
		Subnet:   info.Subnet.String(),
		Asn:      fmt.Sprintf("AS%d", info.Asn),
		Org:      info.Org,
		Location: info.CountryIso,
		Ttlb:     ttlb,
	}, nil
}
