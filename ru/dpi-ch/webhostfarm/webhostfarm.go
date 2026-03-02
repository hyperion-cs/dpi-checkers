package webhostfarm

import (
	"dpich/config"
	"iter"
	"math/rand/v2"
	"net"
	"net/netip"
	"strconv"
	"time"

	tls "github.com/refraction-networking/utls"
	"go4.org/netipx"
)

type FarmOpt struct {
	Subnets *netipx.IPSet
	Count   int
}

type FarmItem struct {
	Ip   netip.Addr
	Port int
}

// Randomly scans ip addresses from the specified subnets until
// it finds count hosts accessible as a web service.
// Currently, only https with forced tls handshake verification is supported.
func Farm(opt FarmOpt) []FarmItem {
	const port = 443 // TODO: make param?
	items := []FarmItem{}
	found := 0
	for ip := range randomIpsIter(opt.Subnets) {
		if found >= opt.Count {
			break
		}
		if tryUTlsHandshake(ip, port) {
			found++
			items = append(items, FarmItem{Ip: ip, Port: port})
			continue
		}
	}
	return items
}

func tryUTlsHandshake(ip netip.Addr, port int) bool {
	cfg := config.Get().WebhostFarm
	d := net.Dialer{Timeout: cfg.TcpConnTimeout}
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))

	tcpConn, err := d.Dial("tcp", addr)
	if err != nil {
		return false
	}

	// without sni
	tlsConn := tls.UClient(tcpConn, &tls.Config{
		InsecureSkipVerify: true,
	}, tls.HelloChrome_Auto)
	defer tlsConn.Close()

	tlsConn.SetDeadline(time.Now().Add(cfg.TlsHandshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return false
	}

	return true
}

// Returns an infinite sequence of ip addresses from a set of subnets, considering their size.
// Currently, only ipv4 is supported and no guarantee of uniqueness.
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
