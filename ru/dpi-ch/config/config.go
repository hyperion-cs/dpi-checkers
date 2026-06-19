package config

import (
	"bytes"
	_ "embed"
	"os"
	"path"
	"path/filepath"
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
			Sections            []WebhostSection  `mapstructure:"sections"`
			Workers             int               `mapstructure:"workers"`
			TcpConnTimeout      time.Duration     `mapstructure:"tcp-conn-timeout"`
			TlsHandshakeTimeout time.Duration     `mapstructure:"tls-handshake-timeout"`
			TcpReadTimeout      time.Duration     `mapstructure:"tcp-read-timeout"`
			TcpWriteTimeout     time.Duration     `mapstructure:"tcp-write-timeout"`
			TcpWriteBuf         int               `mapstructure:"tcp-write-buf"`
			TcpReadBuf          int               `mapstructure:"tcp-read-buf"`
			Tcp1620nBytes       int               `mapstructure:"tcp1620-n-bytes"`
			SiberianConnCount   int               `mapstructure:"siberian-conn-count"`
			SiberianFingerprint string            `mapstructure:"siberian-fingerprint"`
			TableMaxVisibleRows int               `mapstructure:"table-max-visible-rows"`
			HttpStaticHeaders   map[string]string `mapstructure:"http-static-headers"`
		} `mapstructure:"webhost"`

		Dns struct {
			TableMaxVisibleRows int `mapstructure:"table-max-visible-rows"`

			Leak struct {
				Timeout      time.Duration `mapstructure:"timeout"`
				Times        int           `mapstructure:"times"`
				Workers      int           `mapstructure:"workers"`
				ParentDomain string        `mapstructure:"parent-domain"`
				LabelLen     int           `mapstructure:"label-len"`
				LabelAlpha   string        `mapstructure:"label-alpha"`
			} `mapstructure:"leak"`

			Resolve struct {
				PlainOpt struct {
					Timeout time.Duration `mapstructure:"timeout"`
					Workers int           `mapstructure:"workers"`
				} `mapstructure:"plain-opt"`

				DohOpt struct {
					Timeout           time.Duration     `mapstructure:"timeout"`
					Workers           int               `mapstructure:"workers"`
					Path              string            `mapstructure:"path"`
					HttpStaticHeaders map[string]string `mapstructure:"http-static-headers"`
				} `mapstructure:"doh-opt"`

				Targets []struct {
					Host   string `mapstructure:"host"`
					Filter string `mapstructure:"filter"`
				} `mapstructure:"targets"`

				Providers []struct {
					Name  string   `mapstructure:"name"`
					Plain []string `mapstructure:"plain"`
					DoH   struct {
						Filter string   `mapstructure:"filter"`
						Hosts  []string `mapstructure:"hosts"`
					} `mapstructure:"doh"`
				} `mapstructure:"providers"`
			} `mapstructure:"resolve"`
		} `mapstructure:"dns"`

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

	InetUtil struct {
		Iface          string            `mapstructure:"iface"`
		Fingerprint    string            `mapstructure:"fingerprint"`
		KeyLogPath     string            `mapstructure:"key-log-path"`
		BrowserHeaders map[string]string `mapstructure:"browser-headers"`
	} `mapstructure:"inetutil"`

	Updater struct {
		Enabled               bool          `mapstructure:"enabled"`
		Period                time.Duration `mapstructure:"period"`
		Timeout               time.Duration `mapstructure:"timeout"`
		RootDir               string        `mapstructure:"root-dir"`
		SelfTsFile            string        `mapstructure:"self-ts-file"`
		InetlookupTsFile      string        `mapstructure:"inetlookup-ts-file"`
		ForceUpdate           bool          `mapstructure:"force-update"`
		ForceInetlookupUpdate bool          `mapstructure:"force-inetlookup-update"`

		Self struct {
			Owner string `mapstructure:"owner"`
			Repo  string `mapstructure:"repo"`
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

type WebhostSection struct {
	Name    string          `mapstructure:"name"`
	Desc    string          `mapstructure:"desc"`
	Targets []WebhostTarget `mapstructure:"targets"`
}

type WebhostTarget struct {
	Name           string `mapstructure:"name"`
	Filter         string `mapstructure:"filter"`
	Count          int    `mapstructure:"count"`
	Port           int    `mapstructure:"port"`
	Host           string `mapstructure:"host"`
	Sni            string `mapstructure:"sni"`
	Tcp1620skip    bool   `mapstructure:"tcp1620-skip"`
	SiberianSkip   bool   `mapstructure:"siberian-skip"`
	RandomHostname bool   `mapstructure:"random-hostname"`
}

const CfgDefPath = "config.yaml"

var _cfg = &Config{}
var _path = CfgDefPath // absolute path

//go:embed default.yaml
var _defRaw []byte

func Load(path string) error {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(_defRaw)); err != nil {
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

	if err := v.Unmarshal(_cfg); err != nil {
		return err
	}

	// TODO: add config validator
	_path = path
	return nil
}

func Get() *Config {
	return _cfg
}

// Returns the absolute path to the folder with the dpi-ch binary file
func BinFolder() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}

	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Dir(realPath), nil
}

// Returns the absolute path to the current configuration
func ConfigPath() (string, error) {
	if path.IsAbs(_path) {
		return filepath.Clean(_path), nil
	}

	binFolder, err := BinFolder()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(binFolder, _path)), nil
}

func ForceInetlookupUpdate() {
	_cfg.Updater.ForceInetlookupUpdate = true
}

func ForceUpdate() {
	_cfg.Updater.ForceUpdate = true
}
