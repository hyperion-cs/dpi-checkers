package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDefaultXDGPaths(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("XDG paths apply to Linux and Termux")
	}
	home := t.TempDir()
	configHome := filepath.Join(home, "custom config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", "relative-is-ignored")
	wantConfig := filepath.Join(configHome, "dpich", "config.yaml")
	wantData := filepath.Join(home, ".local", "share", "dpich")

	if err := Load(""); err != nil {
		t.Fatal(err)
	}
	if path, err := ConfigPath(); err != nil || path != wantConfig || ConfigCreateError() != nil {
		t.Fatalf("default config = %q (%v), creation error = %v", path, err, ConfigCreateError())
	}
	if raw, err := os.ReadFile(wantConfig); err != nil || string(raw) != string(userTemplate) {
		t.Fatalf("initialized config = %q (%v)", raw, err)
	}
	if DataDir() != wantData || Get().InetlookupGeolitecsv.CidrAs != filepath.Join(wantData, "geolite", "cidr-as.csv") {
		t.Fatalf("unexpected data paths: %q, %+v", DataDir(), Get().InetlookupGeolitecsv)
	}
	const override = "debug: true\n"
	if err := os.WriteFile(wantConfig, []byte(override), 0o600); err != nil {
		t.Fatal(err)
	}
	wantData = filepath.Join(home, "custom data", "dpich")
	t.Setenv("XDG_DATA_HOME", filepath.Dir(wantData))
	if err := Load(""); err != nil {
		t.Fatal(err)
	}
	if !Get().Debug || DataDir() != wantData {
		t.Fatalf("existing config or XDG data override was ignored: debug=%v, data=%q", Get().Debug, DataDir())
	}
}

func TestLoadExplicitRelativePaths(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := Load("config.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing explicit config: %v", err)
	}
	dir := filepath.Join(cwd, "custom")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "config.yaml")
	const yaml = `
all:
  prefix: exports/results_
updater:
  root-dir: ./data
  geolite:
    cidr-as:
      to: as.csv
inetlookup-geolitecsv:
  cidr-country: ./manual/countries.csv
inetutil:
  key-log-path: keys.log
`
	if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load(filepath.Join("custom", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if path, err := ConfigPath(); err != nil || path != file {
		t.Fatalf("explicit config = %q (%v)", path, err)
	}
	cfg := Get()
	if DataDir() != filepath.Join(dir, "data") ||
		cfg.InetlookupGeolitecsv.CidrAs != filepath.Join(dir, "data", "geolite", "as.csv") ||
		cfg.InetlookupGeolitecsv.CidrCountry != filepath.Join(dir, "manual", "countries.csv") ||
		cfg.InetUtil.KeyLogPath != filepath.Join(dir, "keys.log") ||
		cfg.All.Prefix != "exports/results_" {
		t.Fatalf("unexpected paths: data=%q lookup=%+v key-log=%q prefix=%q",
			DataDir(), cfg.InetlookupGeolitecsv, cfg.InetUtil.KeyLogPath, cfg.All.Prefix)
	}
}

func TestLoadDefaultConfigCreationFailure(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("uses the XDG directories")
	}
	home := t.TempDir()
	blocked := filepath.Join(home, "blocked")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	t.Setenv("XDG_DATA_HOME", home)
	if err := Load(""); err != nil {
		t.Fatal(err)
	}
	if path, err := ConfigPath(); err != nil || path != filepath.Join(blocked, "dpich", "config.yaml") {
		t.Fatalf("attempted config = %q (%v)", path, err)
	}
	if ConfigCreateError() == nil || Get().Debug || Get().All.Format != "json" {
		t.Fatalf("expected creation failure with built-in settings: error=%v, debug=%v, format=%q",
			ConfigCreateError(), Get().Debug, Get().All.Format)
	}
}
