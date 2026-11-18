package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
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
			Sections               []WebhostSection  `mapstructure:"sections"`
			Workers                int               `mapstructure:"workers"`
			FarmTimeout            time.Duration     `mapstructure:"farm-timeout"`
			TcpConnTimeout         time.Duration     `mapstructure:"tcp-conn-timeout"`
			TlsHandshakeTimeout    time.Duration     `mapstructure:"tls-handshake-timeout"`
			TcpReadTimeout         time.Duration     `mapstructure:"tcp-read-timeout"`
			TcpWriteTimeout        time.Duration     `mapstructure:"tcp-write-timeout"`
			TcpWriteBuf            int               `mapstructure:"tcp-write-buf"`
			TcpReadBuf             int               `mapstructure:"tcp-read-buf"`
			Tcp1620nBytes          int               `mapstructure:"tcp1620-n-bytes"`
			SiberianConnCount      int               `mapstructure:"siberian-conn-count"`
			SiberianFingerprint    string            `mapstructure:"siberian-fingerprint"`
			TableMaxVisibleRows    int               `mapstructure:"table-max-visible-rows"`
			HttpStaticHeaders      map[string]string `mapstructure:"http-static-headers"`
			ExcludeDefaultSections bool              `mapstructure:"exclude-default-sections"`
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
					QueryTimeout    time.Duration `mapstructure:"query-timeout"`
					ProviderTimeout time.Duration `mapstructure:"provider-timeout"`
					Workers         int           `mapstructure:"workers"`
				} `mapstructure:"plain-opt"`

				DohOpt struct {
					QueryTimeout      time.Duration     `mapstructure:"query-timeout"`
					ProviderTimeout   time.Duration     `mapstructure:"provider-timeout"`
					Workers           int               `mapstructure:"workers"`
					Path              string            `mapstructure:"path"`
					HttpStaticHeaders map[string]string `mapstructure:"http-static-headers"`
				} `mapstructure:"doh-opt"`

				Whoami struct {
					Host string `mapstructure:"host"`
				} `mapstructure:"whoami"`

				Targets []struct {
					Host   string `mapstructure:"host"`
					Filter string `mapstructure:"filter"`
				} `mapstructure:"targets"`

				Providers []struct {
					Name string `mapstructure:"name"`

					Plain struct {
						Filter string   `mapstructure:"filter"`
						Hosts  []string `mapstructure:"hosts"`
					} `mapstructure:"plain"`

					DoH struct {
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

	All struct {
		Format   string   `mapstructure:"format"`
		Checkers []string `mapstructure:"checkers"`
		TsFormat string   `mapstructure:"ts-format"`
		Prefix   string   `mapstructure:"prefix"`
		AutoExit bool     `mapstructure:"auto-exit"`
		Flag     bool     `mapstructure:"flag"`
	} `mapstructure:"all"`

	WebhostFarm struct {
		Workers             int           `mapstructure:"workers"`
		TcpConnTimeout      time.Duration `mapstructure:"tcp-conn-timeout"`
		TlsHandshakeTimeout time.Duration `mapstructure:"tls-handshake-timeout"`
	} `mapstructure:"webhostfarm"`

	Subnetfilter struct {
		Workers int `mapstructure:"workers"`
	} `mapstructure:"subnetfilter"`

	InetLookup struct {
		MailApiUrl   string `mapstructure:"mail-api-url"`
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
	Default bool            `mapstructure:"default"`
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

var _cfg = &Config{}
var _path string
var _createErr error

//go:embed default.yaml
var _defRaw []byte

//go:embed template.yaml
var userTemplate []byte

func Load(file string) error {
	useDefault := file == ""
	if useDefault {
		var err error
		file, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}
	file, err := filepath.Abs(file)
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(_defRaw)); err != nil {
		return err
	}

	var defSec []WebhostSection
	if err := v.UnmarshalKey("checkers.webhost.sections", &defSec); err != nil {
		return err
	}

	userRaw, err := os.ReadFile(file)
	var createErr error
	if useDefault && (errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)) {
		var created bool
		created, createErr = createConfig(file)
		err = nil
		if createErr == nil && !created {
			userRaw, err = os.ReadFile(file)
			if errors.Is(err, os.ErrNotExist) {
				createErr, err = err, nil
			}
		}
	}
	if err != nil {
		return err
	}
	if userRaw != nil {
		if err := v.MergeConfig(bytes.NewReader(userRaw)); err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return err
	}
	if err := cfg.resolvePaths(filepath.Dir(file)); err != nil {
		return err
	}

	userSec := cfg.Checkers.Webhost.Sections
	if !cfg.Checkers.Webhost.ExcludeDefaultSections && !reflect.DeepEqual(defSec, userSec) {
		cfg.Checkers.Webhost.Sections = append(defSec, userSec...)
	}

	// TODO: add config validator
	*_cfg = cfg
	_path = file
	_createErr = createErr
	return nil
}

func createConfig(file string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, writeErr := f.Write(userTemplate)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return false, errors.Join(err, os.Remove(file))
	}
	return true, nil
}

func Get() *Config {
	return _cfg
}

func ForceInetlookupUpdateFlag() {
	_cfg.Updater.ForceInetlookupUpdate = true
}

func ForceUpdateFlag() {
	_cfg.Updater.ForceUpdate = true
}

func AllFlag() {
	_cfg.All.Flag = true
}
