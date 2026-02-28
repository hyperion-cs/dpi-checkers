package webhostfarm

import (
	"iter"
	"math/rand/v2"
	"net"
	"net/netip"
	"time"

	"go4.org/netipx"
)

type FarmOpt struct {
	Subnets   *netipx.IPSet
	HttpsOnly bool
	Count     int
}

type FarmItem struct {
	Ip    netip.Addr
	Port  int
	Https bool
}

// Randomly scans ip addresses from the specified subnets until
// it finds count hosts accessible as a web service.
func Farm(opt FarmOpt) []FarmItem {
	if !opt.HttpsOnly {
		panic("plain http farming is not yet supported")
	}
	items := []FarmItem{}
	found := 0
	for ip := range randomIpsIter(opt.Subnets) {
		if found >= opt.Count {
			break
		}
		if isPortOpen(ip.String(), "443", time.Second) {
			found++
			items = append(items, FarmItem{Ip: ip, Port: 443, Https: true})
			continue
		}
	}
	return items
}

func isPortOpen(ip string, port string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Returns an infinite sequence of ip addresses from a set of subnets, considering their size.
// Currently, only ipv4 is available and no guarantee of uniqueness.
func randomIpsIter(subnets *netipx.IPSet) iter.Seq[netip.Addr] {
	total := ipsetTotal(subnets)
	if total == 0 {
		return func(func(netip.Addr) bool) {}
	}

	return func(yield func(netip.Addr) bool) {
		for {
			k := rand.Uint64N(total)
			for _, r := range subnets.Ranges() {
				if !r.From().Is4() {
					continue
				}
				a := ip4u32(r.From())
				n := iprangeTotal(r)
				if k < n {
					v := u32ip4(a + uint32(k))
					if !yield(v) {
						return
					}
					break
				}
				k -= n
			}
		}
	}
}

// Returns the total number of ip from a set of subnets.
// Currently, only ipv4 is available.
func ipsetTotal(subnets *netipx.IPSet) (total uint64) {
	for _, r := range subnets.Ranges() {
		if r.From().Is4() {
			total += iprangeTotal(r)
		}
	}
	return
}

// Returns the total number of ip from a subnet.
// Currently, only ipv4 is available.
func iprangeTotal(r netipx.IPRange) uint64 {
	return uint64(ip4u32(r.To())-ip4u32(r.From())) + 1
}

// Returns ipv4 as uint32
func ip4u32(ip netip.Addr) uint32 {
	p := ip.As4()
	return uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
}

// Returns uint32 as ipv4
func u32ip4(ip uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(ip >> 24), byte(ip >> 16), byte(ip >> 8), byte(ip)})
}
