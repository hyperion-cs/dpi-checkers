package inetlookup

import (
	"encoding/csv"
	"io"
	"iter"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"go4.org/netipx"
)

type GeoliteCsvOpt struct {
	GeonameId2countryIsoPath string
	Cidr2countryIsoPath      string
	Cidr2asPath              string
}

type cidr2CountryIso struct {
	cidr       netip.Prefix
	countryIso string
}

type cidr2As struct {
	cidr netip.Prefix
	asn  int32
	org  string
}

type geoliteCsv struct {
	cidr2as         []cidr2As
	cidr2countryIso []cidr2CountryIso
}

// TODO: we need indexes instead of direct scans through csv iterators
func NewGeoliteCsv(opt GeoliteCsvOpt) InetLookup {
	lkpr := &geoliteCsv{cidr2as: []cidr2As{}, cidr2countryIso: []cidr2CountryIso{}}
	gnId2Iso := getGeonameId2CountryIso(opt.GeonameId2countryIsoPath)

	for v := range cidr2AsCsvIter(opt.Cidr2asPath) {
		lkpr.cidr2as = append(lkpr.cidr2as, v)
	}
	for v := range cidr2CountryIsoCsvIter(opt.Cidr2countryIsoPath, gnId2Iso) {
		lkpr.cidr2countryIso = append(lkpr.cidr2countryIso, v)
	}

	return lkpr
}

func (l *geoliteCsv) Cidrs(opt CidrsOpt) *netipx.IPSet {
	var b netipx.IPSetBuilder

	if opt.Hosts != nil {
		panic("not implemented yet")
	}

	// ips, asns, orgTerms => cidrs via cidr2as
	if len(opt.Ips) > 0 || len(opt.Asns) > 0 || len(opt.OrgTerms) > 0 {
		if len(opt.OrgTerms) > 0 {
			for i, v := range opt.OrgTerms {
				opt.OrgTerms[i] = strings.ToLower(v)
			}
		}

		for _, cidr2as := range l.cidr2as {
			orgTermContainsFunc := func(term string) bool {
				org := strings.ToLower(cidr2as.org)
				return strings.Contains(org, term)
			}

			if slices.ContainsFunc(opt.Ips, cidr2as.cidr.Contains) ||
				slices.Contains(opt.Asns, cidr2as.asn) ||
				slices.ContainsFunc(opt.OrgTerms, orgTermContainsFunc) {
				b.AddPrefix(cidr2as.cidr)
			}
		}
	}

	// countries => cidrs via cidr2CountryIso
	if len(opt.CountryIsoCodes) > 0 {
		for i, v := range opt.CountryIsoCodes {
			opt.CountryIsoCodes[i] = strings.ToUpper(v)
		}
		for _, cidr2iso := range l.cidr2countryIso {
			if slices.Contains(opt.CountryIsoCodes, cidr2iso.countryIso) {
				b.AddPrefix(cidr2iso.cidr)
			}
		}
	}

	s, err := b.IPSet()
	if err != nil {
		panic(err)
	}
	return s
}

func (l *geoliteCsv) Asns(opt AsnsOpt) []int32 {
	asns := []int32{}
	if len(opt.Ips) > 0 {
		for _, cidr2as := range l.cidr2as {
			if slices.ContainsFunc(opt.Ips, cidr2as.cidr.Contains) {
				asns = append(asns, cidr2as.asn)
			}
		}
	}
	asns = slices.Compact(asns)
	return asns
}

func (l *geoliteCsv) OrgTerms(opt OrgTermsOpt) []string {
	var terms []string
	if len(opt.Ips) > 0 || len(opt.Asns) > 0 {
		for _, cidr2as := range l.cidr2as {
			if slices.ContainsFunc(opt.Ips, cidr2as.cidr.Contains) || slices.Contains(opt.Asns, cidr2as.asn) {
				terms = append(terms, cidr2as.org)
			}
		}
		terms = slices.Compact(terms)
	}
	return terms
}

func (l *geoliteCsv) IpInfo(ip netip.Addr) IpInfo {
	defCidr := netip.MustParsePrefix("0.0.0.0/0")

	info := IpInfo{Ip: ip, Subnet: defCidr}
	for _, cidr2as := range l.cidr2as {
		if cidr2as.cidr.Contains(ip) && info.Subnet.Bits() < cidr2as.cidr.Bits() {
			info.Subnet = cidr2as.cidr
			info.Asn = cidr2as.asn
			info.Org = cidr2as.org
		}
	}

	isoMinSubnet := defCidr
	for _, cidr2iso := range l.cidr2countryIso {
		if cidr2iso.cidr.Contains(ip) && isoMinSubnet.Bits() < cidr2iso.cidr.Bits() {
			isoMinSubnet = cidr2iso.cidr
			info.CountryIso = cidr2iso.countryIso
		}
	}

	return info
}

func cidr2CountryIsoCsvIter(path string, geonameId2countryIso map[int32]string) iter.Seq[cidr2CountryIso] {
	f, _ := os.Open(path)
	r := csv.NewReader(f)

	// header skip
	_, err := r.Read()
	if err != nil {
		panic(err)
	}

	return func(yield func(cidr2CountryIso) bool) {
		for {
			row, err := r.Read()
			if err == io.EOF {
				f.Close()
				return
			}
			if err != nil {
				panic(err)
			}
			if len(row) < 4 {
				panic("cidr2CountryIsoIter: unexpected number of columns in csv")
			}

			geonameId := mustInt32(row[1])
			if geonameId == 0 {
				geonameId = mustInt32(row[2])
			}
			if geonameId == 0 {
				geonameId = mustInt32(row[3])
			}

			v := cidr2CountryIso{
				cidr:       netip.MustParsePrefix(row[0]),
				countryIso: geonameId2countryIso[geonameId],
			}
			if !yield(v) {
				f.Close()
				return
			}
		}
	}
}

func cidr2AsCsvIter(path string) iter.Seq[cidr2As] {
	f, _ := os.Open(path)
	r := csv.NewReader(f)

	// header skip
	_, err := r.Read()
	if err != nil {
		panic(err)
	}

	return func(yield func(cidr2As) bool) {
		for {
			row, err := r.Read()
			if err == io.EOF {
				f.Close()
				return
			}
			if err != nil {
				panic(err)
			}
			if len(row) < 3 {
				panic("cidr2AsIter: unexpected number of columns in csv")
			}

			v := cidr2As{
				cidr: netip.MustParsePrefix(row[0]),
				asn:  mustInt32(row[1]),
				org:  row[2],
			}
			if !yield(v) {
				f.Close()
				return
			}
		}

	}
}

func getGeonameId2CountryIso(path string) map[int32]string {
	f, _ := os.Open(path)
	defer f.Close()
	r := csv.NewReader(f)
	m := make(map[int32]string)

	// header skip
	_, err := r.Read()
	if err != nil {
		panic(err)
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		if len(row) < 4 {
			panic("getGeonameId2CountryIso: unexpected number of columns in csv")
		}

		geonameId := mustInt32(row[0])
		iso := row[4]
		m[geonameId] = iso
	}

	return m
}

func mustInt32(s string) int32 {
	if s == "" {
		return 0
	}
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return int32(i)
}
