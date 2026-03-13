package config

import (
	"bytes"
	_ "embed"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Debug bool `mapstructure:"debug"`

	Checkers struct {
		CidrWhitelist struct {
			Timeout     time.Duration `mapstructure:"timeout"`
			Whitelisted []string      `mapstructure:"whitelisted"`
			Regular     []string      `mapstructure:"regular"`
		} `mapstructure:"cidrwhitelist"`

		Webhost struct {
			Popular []WebhostItem `mapstructure:"popular"`
			Infra   []WebhostItem `mapstructure:"infra"`

			Workers             int               `mapstructure:"workers"`
			TcpConnTimeout      time.Duration     `mapstructure:"tcp-conn-timeout"`
			TlsHandshakeTimeout time.Duration     `mapstructure:"tls-handshake-timeout"`
			TcpReadTimeout      time.Duration     `mapstructure:"tcp-read-timeout"`
			TcpWriteTimeout     time.Duration     `mapstructure:"tcp-write-timeout"`
			TcpWriteBuf         int               `mapstructure:"tcp-write-buf"`
			TcpReadBuf          int               `mapstructure:"tcp-read-buf"`
			Tcp1620nBytes       int               `mapstructure:"tcp1620-n-bytes"`
			KeyLogPath          string            `mapstructure:"key-log-path"`
			TableMaxVisibleRows int               `mapstructure:"table-max-visible-rows"`
			HttpStaticHeaders   map[string]string `mapstructure:"http-static-headers"`
		} `mapstructure:"webhost"`

		Whoami struct {
			Timeout time.Duration `mapstructure:"timeout"`
		} `mapstructure:"whoami"`
	} `mapstructure:"checkers"`

	WebhostFarm struct {
		Workers             int           `mapstructure:"workers"`
		TcpConnTimeout      time.Duration `mapstructure:"tcp-conn-timeout"`
		TlsHandshakeTimeout time.Duration `mapstructure:"tls-handshake-timeout"`
	} `mapstructure:"webhostfarm"`

	Subnetfilter struct {
		Workers int `mapstructure:"workers"`
	} `mapstructure:"subnetfilter"`

	InetLookup struct {
		RipeApiUrl   string `mapstructure:"ripe-api-url"`
		YandexApiUrl string `mapstructure:"yandex-api-url"`
	} `mapstructure:"inetlookup"`

	InetlookupGeolitecsv struct {
		CidrAs           string `mapstructure:"cidr-as"`
		CidrCountry      string `mapstructure:"cidr-country"`
		GeonameidCountry string `mapstructure:"geonameid-country"`
	} `mapstructure:"inetlookup-geolitecsv"`

	HttpUtil struct {
		BrowserHeaders map[string]string `mapstructure:"browser-headers"`
	} `mapstructure:"httputil"`

	Updater struct {
		Enabled               bool          `mapstructure:"enabled"`
		Period                time.Duration `mapstructure:"period"`
		Timeout               time.Duration `mapstructure:"timeout"`
		RootDir               string        `mapstructure:"root-dir"`
		UpdateTsFile          string        `mapstructure:"update-ts-file"`
		ForceInetlookupUpdate bool          `mapstructure:"force-inetlookup-update"`

		Self struct {
			Dir       string `mapstructure:"dir"`
			Bin       string `mapstructure:"bin"`
			Owner     string `mapstructure:"owner"`
			Repo      string `mapstructure:"repo"`
			TagPrefix string `mapstructure:"tag-prefix"`
		} `mapstructure:"self"`

		Geolite struct {
			Dir    string `mapstructure:"dir"`
			Owner  string `mapstructure:"owner"`
			Repo   string `mapstructure:"repo"`
			Branch string `mapstructure:"branch"`

			CidrAs struct {
				From string `mapstructure:"from"`
				To   string `mapstructure:"to"`
			} `mapstructure:"cidr-as"`

			CidrCountry struct {
				From string `mapstructure:"from"`
				To   string `mapstructure:"to"`
			} `mapstructure:"cidr-country"`

			GeonameidCountry struct {
				From string `mapstructure:"from"`
				To   string `mapstructure:"to"`
			} `mapstructure:"geonameid-country"`
		} `mapstructure:"geolite"`
	} `mapstructure:"updater"`
}

type WebhostItem struct {
	Name           string `mapstructure:"name"`
	Filter         string `mapstructure:"filter"`
	Count          int    `mapstructure:"count"`
	Port           int    `mapstructure:"port"`
	Host           string `mapstructure:"host"`
	Sni            string `mapstructure:"sni"`
	Tcp1620skip    bool   `mapstructure:"tcp1620-skip"`
	RandomHostname bool   `mapstructure:"random-hostname"`
}

const CfgDefPath = "config.yaml"

var cfg = &Config{}

//go:embed default.yaml
var defRaw []byte

func Load(path string) error {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(defRaw)); err != nil {
		return err
	}

	_, err := os.Stat(path)
	if path != CfgDefPath && err != nil {
		return err
	}

	if err == nil {
		if userRaw, err := os.Open(path); err == nil {
			defer userRaw.Close()
			if err := v.MergeConfig(userRaw); err != nil {
				return err
			}
		}
	}

	if err := v.Unmarshal(cfg); err != nil {
		return err
	}

	// TODO: add config validator
	return nil
}

func Get() *Config {
	return cfg
}

func ForceInetlookupUpdate() {
	cfg.Updater.ForceInetlookupUpdate = true
}
