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
			CheckWorkers        int               `yaml:"check-workers"`
			FarmWorkers         int               `yaml:"farm-workers"`
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
		TcpConnTimeout      time.Duration `yaml:"tcp-conn-timeout"`
		TlsHandshakeTimeout time.Duration `yaml:"tls-handshake-timeout"`
	} `yaml:"webhostfarm"`

	InetLookup struct {
		RipeApiUrl   string `yaml:"ripe-api-url"`
		YandexApiUrl string `yaml:"yandex-api-url"`
	}

	HttpUtil struct {
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
