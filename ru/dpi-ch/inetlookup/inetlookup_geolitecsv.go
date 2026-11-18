package inetlookup

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"go4.org/netipx"
)

type GeoliteCsvOpt struct {
	GeonameidCountryPath string
	CidrCountryPath      string
	CidrAsPath           string
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
	cidrAs      []cidr2As
	cidrCountry []cidr2CountryIso
}

// TODO: index the loaded CSV records for faster lookups.
func NewGeoliteCsv(opt GeoliteCsvOpt) InetLookup {
	lookup, err := loadGeoliteCsv(opt)
	if err != nil {
		prettyPanic("NewGeoliteCsv", err)
	}
	return lookup
}

func loadGeoliteCsv(opt GeoliteCsvOpt) (InetLookup, error) {
	lkpr := &geoliteCsv{cidrAs: []cidr2As{}, cidrCountry: []cidr2CountryIso{}}
	gnId2Iso := make(map[int32]string)
	if err := readGeoliteCSV(opt.GeonameidCountryPath, 5, func(row []string) error {
		id, err := parseInt32(row[0])
		if err != nil {
			return err
		}
		gnId2Iso[id] = row[4]
		return nil
	}); err != nil {
		return nil, err
	}
	if err := readGeoliteCSV(opt.CidrAsPath, 3, func(row []string) error {
		cidr, err := netip.ParsePrefix(row[0])
		if err != nil {
			return err
		}
		asn, err := parseInt32(row[1])
		if err != nil {
			return err
		}
		lkpr.cidrAs = append(lkpr.cidrAs, cidr2As{cidr: cidr, asn: asn, org: row[2]})
		return nil
	}); err != nil {
		return nil, err
	}
	if err := readGeoliteCSV(opt.CidrCountryPath, 4, func(row []string) error {
		cidr, err := netip.ParsePrefix(row[0])
		if err != nil {
			return err
		}
		var id int32
		for _, value := range row[1:4] {
			id, err = parseInt32(value)
			if err != nil {
				return err
			}
			if id != 0 {
				break
			}
		}
		lkpr.cidrCountry = append(lkpr.cidrCountry, cidr2CountryIso{cidr: cidr, countryIso: gnId2Iso[id]})
		return nil
	}); err != nil {
		return nil, err
	}

	return lkpr, nil
}

func (l *geoliteCsv) Cidrs(opt CidrsOpt) *netipx.IPSet {
	var b netipx.IPSetBuilder

	if opt.Hosts != nil {
		prettyPanic("Cidrs", fmt.Errorf("not implemented yet"))
	}

	// ips, asns, orgTerms => cidrs via cidr2as
	if len(opt.Ips) > 0 || len(opt.Asns) > 0 || len(opt.OrgTerms) > 0 {
		if len(opt.OrgTerms) > 0 {
			for i, v := range opt.OrgTerms {
				opt.OrgTerms[i] = strings.ToLower(v)
			}
		}

		for _, cidr2as := range l.cidrAs {
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
		for _, cidr2iso := range l.cidrCountry {
			if slices.Contains(opt.CountryIsoCodes, cidr2iso.countryIso) {
				b.AddPrefix(cidr2iso.cidr)
			}
		}
	}

	s, err := b.IPSet()
	if err != nil {
		prettyPanic("Cidrs", err)
	}
	return s
}

func (l *geoliteCsv) Asns(opt AsnsOpt) []int32 {
	asns := []int32{}
	if len(opt.Ips) > 0 {
		for _, cidr2as := range l.cidrAs {
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
		for _, cidr2as := range l.cidrAs {
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
	for _, cidr2as := range l.cidrAs {
		if cidr2as.cidr.Contains(ip) && info.Subnet.Bits() < cidr2as.cidr.Bits() {
			info.Subnet = cidr2as.cidr
			info.Asn = cidr2as.asn
			info.Org = cidr2as.org
		}
	}

	isoMinSubnet := defCidr
	for _, cidr2iso := range l.cidrCountry {
		if cidr2iso.cidr.Contains(ip) && isoMinSubnet.Bits() < cidr2iso.cidr.Bits() {
			isoMinSubnet = cidr2iso.cidr
			info.CountryIso = cidr2iso.countryIso
		}
	}

	return info
}

func readGeoliteCSV(path string, minFields int, readRow func([]string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("%s: CSV header: %w", path, err)
	}
	if len(header) < minFields {
		return fmt.Errorf("%s: expected at least %d CSV columns, got %d", path, minFields, len(header))
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := readRow(row); err != nil {
			line, _ := r.FieldPos(0)
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
}

func parseInt32(s string) (int32, error) {
	if s == "" {
		return 0, nil
	}
	i, err := strconv.ParseInt(s, 10, 32)
	return int32(i), err
}

func prettyPanic(loc string, err error) string {
	panic(fmt.Sprintf("❗️ An unexpected problem when reading geoip files. You can try updating them using the --force-inetlookup-update flag when running the utility.\n\n Details [%s]: %v", loc, err))
}
