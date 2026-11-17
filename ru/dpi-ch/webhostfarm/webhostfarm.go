package webhostfarm

import (
	"context"
	"iter"
	"math/rand/v2"
	"net/netip"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"

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

// Randomly scans IP addresses from a set of subnets for HTTPS availability.
// Streams at most opt.Count items, falling back to the last attempted address if none succeed.
// Cancel ctx when stopping consumption before the channel closes.
func Farm(ctx context.Context, opt FarmOpt) <-chan FarmItem {
	out := make(chan FarmItem)
	go func() {
		defer close(out)
		if opt.Count <= 0 {
			return
		}
		found := 0
		last := FarmItem{}
		for ip := range randomIpsIter(ctx, opt.Subnets) {
			if ctx.Err() != nil {
				return
			}
			last = FarmItem{Ip: ip, Port: opt.Port}
			if !tryConnect(ctx, ip, opt.Port, opt.Sni) {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- last:
				found++
			}
			if found >= opt.Count {
				return
			}
		}
		if found == 0 && last.Ip.IsValid() && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case out <- last:
			}
		}
	}()
	return out
}

func tryConnect(ctx context.Context, ip netip.Addr, port int, sni string) bool {
	cfg := config.Get().WebhostFarm
	conn, err := inetutil.GetHandshakedUTlsConn(inetutil.TlsConnOpt{
		Ctx:                 ctx,
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
func randomIpsIter(ctx context.Context, subnets *netipx.IPSet) iter.Seq[netip.Addr] {
	total := ipsetTotal(subnets)
	if total == 0 {
		return func(func(netip.Addr) bool) {}
	}

	processed := map[uint64]struct{}{}
	ranges := subnets.Ranges()
	return func(yield func(netip.Addr) bool) {
		for {
			if ctx.Err() != nil || uint64(len(processed)) >= total {
				return
			}
			// TODO: impl random pick with blacklist
			k := rand.Uint64N(total)
			if _, has := processed[k]; has {
				continue
			}
			processed[k] = struct{}{}

			for _, r := range ranges {
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
