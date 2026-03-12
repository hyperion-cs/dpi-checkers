package inetlookup

import (
	"net/netip"

	"go4.org/netipx"
)

type CidrsOpt struct {
	Hosts           []string
	Ips             []netip.Addr
	Asns            []int32
	OrgTerms        []string
	CountryIsoCodes []string
}

type AsnsOpt struct {
	Ips []netip.Addr
}

type OrgTermsOpt struct {
	Ips  []netip.Addr
	Asns []int32
}

type IpInfo struct {
	Ip         netip.Addr
	Asn        int32
	Subnet     netip.Prefix
	Org        string
	CountryIso string
}

type InetLookup interface {
	// Returns set of cidrs that satisfy at least one condition from opt.
	// All CidrsOpt fields are optional.
	Cidrs(opt CidrsOpt) *netipx.IPSet

	// Returns unique list of asns that satisfy at least one condition from opt.
	// All AsnsOpt fields are optional.
	Asns(opt AsnsOpt) []int32

	// Returns unique list of org terms that satisfy at least one condition from opt.
	// All OrgTermsOpt fields are optional.
	OrgTerms(opt OrgTermsOpt) []string

	// Returns asn, subnet (in cidr notation), org name and country iso of smallest subnet that contains ip.
	IpInfo(ip netip.Addr) IpInfo
}
