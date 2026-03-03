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
			Timeout     time.Duration `yaml:"timeout"`
			Whitelisted []string      `yaml:"whitelisted"`
			Regular     []string      `yaml:"regular"`
		} `yaml:"cidrwhitelist"`

		Webhost struct {
			Popular []WebhostItem `yaml:"popular"`
			Infra   []WebhostItem `yaml:"infra"`

			Workers             int               `yaml:"workers"`
			TcpConnTimeout      time.Duration     `yaml:"tcp-conn-timeout"`
			TlsHandshakeTimeout time.Duration     `yaml:"tls-handshake-timeout"`
			TcpReadTimeout      time.Duration     `yaml:"tcp-read-timeout"`
			TcpWriteTimeout     time.Duration     `yaml:"tcp-write-timeout"`
			TcpWriteBuf         int               `yaml:"tcp-write-buf"`
			TcpReadBuf          int               `yaml:"tcp-read-buf"`
			Tcp1620nBytes       int               `yaml:"tcp1620-n-bytes"`
			KeyLogPath          string            `yaml:"key-log-path"`
			HttpStaticHeaders   map[string]string `yaml:"http-static-headers"`
		} `yaml:"webhost"`

		Whoami struct {
			Timeout time.Duration `yaml:"timeout"`
		} `yaml:"whoami"`
	} `yaml:"checkers"`

	WebhostFarm struct {
		Workers             int           `yaml:"workers"`
		TcpConnTimeout      time.Duration `yaml:"tcp-conn-timeout"`
		TlsHandshakeTimeout time.Duration `yaml:"tls-handshake-timeout"`
	} `yaml:"webhostfarm"`

	Subnetfilter struct {
		Workers int `yaml:"workers"`
	} `yaml:"subnetfilter"`

	InetLookup struct {
		RipeApiUrl   string `yaml:"ripe-api-url"`
		YandexApiUrl string `yaml:"yandex-api-url"`
	}

	HttpUtil struct {
		BrowserHeaders map[string]string `yaml:"browser-headers"`
	} `yaml:"netutils"`
}

type WebhostItem struct {
	Name           string `yaml:"name"`
	Filter         string `yaml:"filter"`
	Count          int    `yaml:"count"`
	Port           int    `yaml:"port"`
	Host           string `yaml:"sni"`
	Sni            string `yaml:"host"`
	Tcp1620skip    bool   `yaml:"tcp1620-skip"`
	RandomHostname bool   `yaml:"random-hostname"`
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
