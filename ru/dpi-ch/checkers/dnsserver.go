// Checks the availability of some dns servers

package checkers

import (
	"context"
	"dpich/config"
	"fmt"
	"net"
)

func DnsServer(server, domain string) ([]net.IP, error) {
	cfg := config.Get().Checkers.DnsServer

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: cfg.ConnTimeout}
			return d.DialContext(ctx, "udp", server)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.LookupTimeout)
	defer cancel()

	ips, err := resolver.LookupIP(ctx, "ip", domain)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no A records")
	}

	return ips, nil
}
