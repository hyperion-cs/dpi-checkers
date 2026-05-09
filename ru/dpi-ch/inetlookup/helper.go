package inetlookup

import "fmt"

type IpInfoStrings struct {
	Ip       string
	Subnet   string
	Asn      string
	Org      string
	Location string
}

func IpInfoAsStrings(info IpInfo) IpInfoStrings {
	return IpInfoStrings{
		Ip:       info.Ip.String(),
		Subnet:   info.Subnet.String(),
		Asn:      fmt.Sprintf("AS%d", info.Asn),
		Org:      info.Org,
		Location: info.CountryIso,
	}
}
