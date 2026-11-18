package testutil

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func saveConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	return cfg
}

func WebhostConfig(t *testing.T) *config.Config {
	const geoLiteFixtures = "../../inetlookup/testdata/geolite2_csv"

	t.Helper()
	cfg := saveConfig(t)
	cfg.Checkers.Webhost.Workers = 1
	cfg.Checkers.Webhost.FarmTimeout = 0
	cfg.Subnetfilter.Workers = 1
	cfg.WebhostFarm.Workers = 1
	cfg.InetlookupGeolitecsv.CidrAs = filepath.Join(geoLiteFixtures, "cidr2as_ipv4.csv")
	cfg.InetlookupGeolitecsv.CidrCountry = filepath.Join(geoLiteFixtures, "cidr2countryIso_ipv4.csv")
	cfg.InetlookupGeolitecsv.GeonameidCountry = filepath.Join(geoLiteFixtures, "geonameId2country_en.csv")
	return cfg
}

func DnsConfig(t *testing.T, raw string) *config.Config {
	t.Helper()
	cfg := saveConfig(t)
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(file); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func DnsServer(t *testing.T, nxdomain string) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			var msg dnsmessage.Message
			if err := msg.Unpack(buf[:n]); err != nil || len(msg.Questions) == 0 || msg.Questions[0].Name.String() != nxdomain {
				continue
			}
			msg.Header.Response = true
			msg.Header.RCode = dnsmessage.RCodeNameError
			raw, err := msg.Pack()
			if err == nil {
				conn.WriteTo(raw, addr)
			}
		}
	}()
	t.Cleanup(func() { conn.Close(); <-done })
	return conn.LocalAddr().String()
}
