package subnetfilter

import (
	"dpich/inetlookup"
	"net/netip"
	"slices"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"go4.org/netipx"
)

type Subnetfilter struct {
	inetlookup inetlookup.InetLookup
	env        map[string]any
}

func New(lookup inetlookup.InetLookup) *Subnetfilter {
	var s = &Subnetfilter{inetlookup: lookup}
	var env = map[string]any{
		"host":    s.host,
		"subnet":  s.subnet,
		"as":      s.as,
		"org":     s.org,
		"country": s.country,
		"and":     intersect,
		"or":      union,
	}
	s.env = env
	return s
}

func (s *Subnetfilter) CompileFilter(filter string) (*vm.Program, error) {
	prog, err := expr.Compile(
		filter,
		expr.Env(s.env),
		expr.Operator("&&", "and"),
		expr.Operator("||", "or"),
	)

	return prog, err
}

func (s *Subnetfilter) RunFilter(filter *vm.Program) (*netipx.IPSet, error) {
	v, err := expr.Run(filter, s.env)
	if err != nil {
		return nil, err
	}
	return v.(*netipx.IPSet), nil
}

// hostname to A/AAAA dns records (/32 subnets)
// tricks such as deep search are used
func (s *Subnetfilter) host(hosts ...string) *netipx.IPSet {
	return s.inetlookup.Cidrs(inetlookup.CidrsOpt{Hosts: hosts})
}

// ips or subnets to subnets
func (s *Subnetfilter) subnet(vRaws ...string) *netipx.IPSet {
	ips := []netip.Addr{}
	subnets := []netip.Prefix{}
	for _, vRaw := range vRaws {
		subnet, err := netip.ParsePrefix(vRaw)
		if err == nil {
			subnets = append(subnets, subnet)
			continue
		}
		ip := netip.MustParseAddr(vRaw)
		ips = append(ips, ip)
	}
	ips = slices.Compact(ips)
	subnets = slices.Compact(subnets)

	var b netipx.IPSetBuilder
	for _, cidr := range subnets {
		b.AddPrefix(cidr)
	}

	ips2subnets := s.inetlookup.Cidrs(inetlookup.CidrsOpt{Ips: ips})
	b.AddSet(ips2subnets)
	set, err := b.IPSet()
	if err != nil {
		panic(err)
	}
	return set
}

// asns or (ips => asns) to subnets
func (s *Subnetfilter) as(vRaws ...any) *netipx.IPSet {
	asns := []int32{}
	ips := []netip.Addr{}
	for _, vRaw := range vRaws {
		// int is asn, string is ip addr
		switch v := vRaw.(type) {
		case int:
			asns = append(asns, int32(v))
		case string:
			ip := netip.MustParseAddr(v)
			ips = append(ips, ip)
		default:
			panic("as: expected asn or ip")
		}

	}
	asns = slices.Compact(asns)
	ips = slices.Compact(ips)

	// convert ips to extra asns
	extra := s.inetlookup.Asns(inetlookup.AsnsOpt{Ips: ips})
	asns = append(asns, extra...)

	return s.inetlookup.Cidrs(inetlookup.CidrsOpt{Asns: asns})
}

// org terms, (asn => org term) or (ip => org terms) to subnets
func (s *Subnetfilter) org(vRaws ...any) *netipx.IPSet {
	// v is list of org term, asn, ip
	ips := []netip.Addr{}
	asns := []int32{}
	terms := []string{}
	for _, vRaw := range vRaws {
		switch v := vRaw.(type) {
		case int:
			asns = append(asns, int32(v))
		case string:
			ip, err := netip.ParseAddr(v)
			if err == nil {
				ips = append(ips, ip)
				continue
			}

			// v is probably org term
			org := strings.Trim(v, " ")
			org = strings.ToLower(org)
			terms = append(terms, org)
		default:
			panic("org: expected org term, asn or ip")
		}
	}
	ips = slices.Compact(ips)
	asns = slices.Compact(asns)
	terms = slices.Compact(terms)

	for i, v := range terms {
		terms[i] = strings.ToLower(v)
	}

	// convert ips and asns to extra org terms
	extra := s.inetlookup.OrgTerms(inetlookup.OrgTermsOpt{Ips: ips, Asns: asns})
	terms = append(terms, extra...)

	return s.inetlookup.Cidrs(inetlookup.CidrsOpt{OrgTerms: terms})
}

// country iso codes to subnets
func (s *Subnetfilter) country(isoCodes ...string) *netipx.IPSet {
	return s.inetlookup.Cidrs(inetlookup.CidrsOpt{CountryIsoCodes: isoCodes})
}

func intersect(a, b *netipx.IPSet) *netipx.IPSet {
	var ab netipx.IPSetBuilder
	ab.AddSet(a)
	ab.Intersect(b)
	s, _ := ab.IPSet()
	return s
}

func union(a, b *netipx.IPSet) *netipx.IPSet {
	var ab netipx.IPSetBuilder
	ab.AddSet(a)
	ab.AddSet(b)
	s, _ := ab.IPSet()
	return s
}
