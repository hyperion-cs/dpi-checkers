package config

import (
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Debug bool `yaml:"debug"`

	Checkers struct {
		CidrWhitelist struct {
			Timeout       time.Duration `yaml:"timeout"`
			WlEndpoints   []string      `yaml:"wl-endpoints"`
			NormEndpoints []string      `yaml:"norm-endpoints"`
		} `yaml:"cidrwhitelist"`

		DnsServer struct {
			ConnTimeout   time.Duration `yaml:"conn-timeout"`
			LookupTimeout time.Duration `yaml:"lookup-timeout"`
		} `yaml:"dnsserver"`

		Endpoint struct {
			Timeout time.Duration `yaml:"timeout"`
			Urls    []string      `yaml:"urls"`
		} `yaml:"endpoint"`

		Tcp1620 struct {
			Workers             int           `yaml:"workers"`
			NBytes              int           `yaml:"n-bytes"`
			BufSize             int           `yaml:"buf-size"`
			TcpConnTimeout      time.Duration `yaml:"tcp-conn-timeout"`
			TlsHandshakeTimeout time.Duration `yaml:"tls-handshake-timeout"`
			HttpHeadersTimeout  time.Duration `yaml:"http-headers-timeout"`
			TotalTimeout        time.Duration `yaml:"total-timeout"`
			Endpoints           []string      `yaml:"endpoints"`
		} `yaml:"tcp1620"`

		Whoami struct {
			Timeout time.Duration `yaml:"timeout"`
		} `yaml:"whoami"`
	} `yaml:"checkers"`

	Netutils struct {
		RipeApiUrl     string            `yaml:"ripe-api-url"`
		YandexApiUrl   string            `yaml:"yandex-api-url"`
		Timeout        time.Duration     `yaml:"timeout"`
		BrowserHeaders map[string]string `yaml:"browser-headers"`
	} `yaml:"netutils"`
}

var cfg = &Config{}

func Load(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(b, cfg); err != nil {
		return err
	}

	// TODO: add config validator
	return nil
}

func Get() *Config {
	return cfg
}
