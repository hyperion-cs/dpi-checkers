package inetutil

import (
	"errors"
	"log"
	"net"
	"net/netip"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

var (
	ErrIfaceNoSpecified       = errors.New("no network interface specified")
	ErrIfaceIp4Only           = errors.New("only ipv4 is supported")
	ErrIfaceIp4NotFoundByName = errors.New("no ipv4 address found in network interface with specified name")
)

// Returns ipv4 of network interface (specified in config),
// or ErrIfaceNoSpecified error if it is not specified.
func Iface4() (netip.Addr, error) {
	cfg := config.Get().InetUtil
	if cfg.Iface == "" {
		log.Println("inetutil/iface/4", ErrIfaceNoSpecified, cfg.Iface)
		return netip.Addr{}, ErrIfaceNoSpecified
	}

	if addr, err := netip.ParseAddr(cfg.Iface); err == nil {
		if !addr.Is4() {
			log.Println("inetutil/iface/4", ErrIfaceIp4Only, cfg.Iface)
			return netip.Addr{}, ErrIfaceIp4Only
		}
		return addr, nil
	}

	addr, err := IfaceNameToIp4(cfg.Iface)
	if err != nil {
		log.Println("inetutil/iface/4", err, cfg.Iface)
		return netip.Addr{}, err
	}

	return addr, nil
}

// Returns first ipv4 address found for network interface by name.
func IfaceNameToIp4(name string) (netip.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return netip.Addr{}, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Addr{}, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ipv4 := ipnet.IP.To4()
			if ipv4 == nil {
				continue
			}

			x, _ := netip.AddrFromSlice(ipv4)
			return x, nil
		}
	}

	return netip.Addr{}, ErrIfaceIp4NotFoundByName
}
