package webhostfarm

import (
	"iter"
	"math/rand/v2"
	"net/netip"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"

	"go4.org/netipx"
)

type FarmOpt struct {
	Subnets *netipx.IPSet
	Count   int
	Port    int
	Sni     string
}

type FarmItem struct {
	Ip   netip.Addr
	Port int
}

// Randomly scans ip addresses from a specified set of subnets for web service availability.
// No more than opt.Count items will be returned.
// Currently, only https with forced tls handshake verification is supported.
func Farm(opt FarmOpt) []FarmItem {
	items := []FarmItem{}
	last := FarmItem{}

	for ip := range randomIpsIter(opt.Subnets) {
		if len(items) >= opt.Count {
			break
		}
		last = FarmItem{Ip: ip, Port: opt.Port}
		if tryConnect(ip, opt.Port, opt.Sni) {
			items = append(items, last)
		}
	}

	if len(items) == 0 {
		// Try to find hosts with a successful tls handshake,
		// but in the worst case, return at least one any.
		items = append(items, last)
	}

	return items
}

func tryConnect(ip netip.Addr, port int, sni string) bool {
	cfg := config.Get().WebhostFarm
	conn, err := httputil.GetHandshakedUTlsConn(httputil.TlsConnOpt{
		Ip:                  ip,
		Port:                port,
		Sni:                 sni,
		TcpConnTimeout:      cfg.TcpConnTimeout,
		TlsHandshakeTimeout: cfg.TlsHandshakeTimeout,
	})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Returns a random sequence of ip addresses from a set of subnets (considering their size).
// It is guaranteed that addresses will not be repeated. Currently, only ipv4 is supported.
func randomIpsIter(subnets *netipx.IPSet) iter.Seq[netip.Addr] {
	total := ipsetTotal(subnets)
	if total == 0 {
		return func(func(netip.Addr) bool) {}
	}

	processed := map[uint64]struct{}{}
	return func(yield func(netip.Addr) bool) {
		for {
			if len(processed) >= int(total) {
				return
			}
			// TODO: impl random pick with blacklist
			k := rand.Uint64N(total)
			if _, has := processed[k]; has {
				continue
			}
			processed[k] = struct{}{}

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
// Currently, only ipv4 is supported.
func ipsetTotal(subnets *netipx.IPSet) (total uint64) {
	for _, r := range subnets.Ranges() {
		if r.From().Is4() {
			total += iprangeTotal(r)
		}
	}
	return
}

// Returns the total number of ip from a subnet.
// Currently, only ipv4 is supported.
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
