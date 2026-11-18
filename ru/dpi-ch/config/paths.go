package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const configFilename = "config.yaml"

// The Docker build sets this linker variable.
var containerBuild = "false"

func Container() bool {
	return containerBuild == "true"
}

func ConfigPath() (string, error) {
	if _path == "" {
		return defaultConfigPath()
	}
	return _path, nil
}

func ConfigCreateError() error {
	return _createErr
}

func DataDir() string {
	return _cfg.Updater.RootDir
}

func (cfg *Config) DataPath(name string) string {
	return resolvePath(cfg.Updater.RootDir, name)
}

func defaultConfigPath() (string, error) {
	var root string
	var err error
	switch runtime.GOOS {
	case "darwin", "windows":
		root, err = os.UserConfigDir()
	default:
		root, err = xdgDir("XDG_CONFIG_HOME", ".config")
	}
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(root, "dpich", configFilename))
}

func defaultDataDir() (string, error) {
	var root, suffix string
	var err error
	switch runtime.GOOS {
	case "darwin":
		root, err = os.UserConfigDir()
		suffix = "data"
	case "windows":
		root, err = os.UserCacheDir()
		suffix = "data"
	default:
		root, err = xdgDir("XDG_DATA_HOME", ".local/share")
	}
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(root, "dpich", suffix))
}

func xdgDir(variable, fallback string) (string, error) {
	if dir := os.Getenv(variable); filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback), nil
}

func (cfg *Config) resolvePaths(configDir string) error {
	upd := &cfg.Updater
	if upd.RootDir == "" {
		var err error
		upd.RootDir, err = defaultDataDir()
		if err != nil {
			return err
		}
	} else {
		upd.RootDir = resolvePath(configDir, upd.RootDir)
	}
	cidrAs, cidrCountry, geonameidCountry := cfg.GeoliteOutputPaths()
	geo := &cfg.InetlookupGeolitecsv
	geo.CidrAs = lookupPath(configDir, geo.CidrAs, cidrAs)
	geo.CidrCountry = lookupPath(configDir, geo.CidrCountry, cidrCountry)
	geo.GeonameidCountry = lookupPath(configDir, geo.GeonameidCountry, geonameidCountry)
	if cfg.InetUtil.KeyLogPath != "" {
		cfg.InetUtil.KeyLogPath = resolvePath(configDir, cfg.InetUtil.KeyLogPath)
	}
	return nil
}

// GeoliteOutputPaths returns the updater destinations and the default CSV read paths.
func (cfg *Config) GeoliteOutputPaths() (cidrAs, cidrCountry, geonameidCountry string) {
	upd := &cfg.Updater
	dir := cfg.DataPath(upd.Geolite.Dir)
	return resolvePath(dir, upd.Geolite.CidrAs.To),
		resolvePath(dir, upd.Geolite.CidrCountry.To),
		resolvePath(dir, upd.Geolite.GeonameidCountry.To)
}

func lookupPath(configDir, path, defaultPath string) string {
	if path == "" {
		return defaultPath
	}
	return resolvePath(configDir, path)
}

func resolvePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}
