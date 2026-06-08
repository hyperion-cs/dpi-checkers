package checkers

import (
	"time"
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
	const ttlb = 128 * time.Millisecond
	time.Sleep(ttlb)
	return WhoamiResult{
		Ip:       "3.3.3.3",
		Subnet:   "3.3.3.3/24",
		Asn:      "AS14618",
		Org:      "Amazon.com, Inc.",
		Location: "US",
		Ttlb:     ttlb,
	}, nil
}
