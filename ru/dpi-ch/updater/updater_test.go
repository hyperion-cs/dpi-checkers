package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func TestSelfCheckUpdatesContext(t *testing.T) {
	cfg := &config.Get().Updater
	previous := *cfg
	t.Cleanup(func() { *cfg = previous })
	cfg.Self.Owner, cfg.Self.Repo = "hyperion-cs", "dpi-checkers"
	cfg.Timeout = time.Minute

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := SelfCheckUpdates(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled check returned %v", err)
	}

	cfg.Timeout = 0
	if _, err := SelfCheckUpdates(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired check returned %v", err)
	}
}

func TestGeoliteDownload(t *testing.T) {
	owner, repo, fn, branch := "Loyalsoldier", "geoip", "GeoLite2-ASN-Blocks-IPv4.csv", "release"
	dst := filepath.Join(t.TempDir(), fn)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	remoteHash, err := remoteHash(ctx, owner, repo, fn, branch)
	if err != nil {
		t.Fatalf("remote hash: %v", err)
	}

	writeTestFile(t, dst+HASH_POSTFIX, []byte(remoteHash))

	localHash, err := readLocalHash(dst)
	if err != nil {
		t.Fatalf("read local hash: %v", err)
	}
	if localHash != remoteHash {
		t.Fatalf("ReadLocalHash: got %v, want %v", localHash, remoteHash)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := download(ctx, contentUrl(owner, repo, fn, branch), dst); err != nil {
		t.Fatalf("download: %v", err)
	}
}

func TestTermuxExecutable(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dpich := filepath.Join(dir, "dpich")
	linker := filepath.Join(dir, "linker64")
	writeTestFile(t, dpich, nil)
	writeTestFile(t, linker, nil)
	for _, test := range []struct{ name, env, argv0, want string }{
		{"termux", dpich, "dpich", dpich},
		{"argv fallback", "dpich", dpich, dpich},
		{"reject linker", linker, linker, ""},
		{"reject relative path", "dpich", "dpich", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := termuxExecutable(test.env, test.argv0)
			if got != test.want || (err == nil) != (test.want != "") {
				t.Fatalf("target = %q, error = %v, want %q", got, err, test.want)
			}
		})
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "installed")
		if err := os.Symlink(dpich, link); err != nil {
			t.Fatal(err)
		}
		if got, err := termuxExecutable(link, ""); err != nil || got != dpich {
			t.Fatalf("symlink target = %q, error = %v", got, err)
		}
	}
}

func TestApplyRelease(t *testing.T) {
	const previous = "previous executable"
	const replacement = "new executable"
	name := "dpich"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, test := range []struct {
		name    string
		archive []byte
		want    string
	}{
		{"renamed target", releaseZip(t, name, replacement), replacement},
		{"empty executable", releaseZip(t, name, ""), previous},
		{"broken archive", []byte("invalid zip"), previous},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write(test.archive)
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "renamed-"+name)
			writeTestFile(t, target, []byte(previous))
			if err := applyRelease(t.Context(), server.URL, "release.zip", target); (err == nil) != (test.want == replacement) {
				t.Fatalf("apply error = %v", err)
			}
			if got, err := os.ReadFile(target); err != nil || string(got) != test.want {
				t.Fatalf("installed executable = %q, error = %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestGeoliteDueWithMissingCSVAndFreshTimestamp(t *testing.T) {
	cfg := config.Get()
	previous := *cfg
	t.Cleanup(func() { *cfg = previous })
	cfg.Updater.RootDir = t.TempDir()
	cfg.Updater.Period = time.Hour
	cfg.Updater.InetlookupTsFile = "inetlookup-ts"
	cfg.Updater.Geolite.Dir = "geolite"
	cfg.Updater.Geolite.CidrAs.To = "cidr-as.csv"
	cfg.Updater.Geolite.CidrCountry.To = "cidr-country.csv"
	cfg.Updater.Geolite.GeonameidCountry.To = "geonameid-country.csv"
	parts := geoliteParts()
	cfg.InetlookupGeolitecsv.CidrAs = parts[0].dest
	cfg.InetlookupGeolitecsv.CidrCountry = parts[1].dest
	cfg.InetlookupGeolitecsv.GeonameidCountry = parts[2].dest

	if err := writeUpdateTimestamp(cfg.DataPath(cfg.Updater.InetlookupTsFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(parts[0].dest), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtures := [...]string{"cidr2as_ipv4.csv", "cidr2countryIso_ipv4.csv", "geonameId2country_en.csv"}
	for i, part := range parts {
		csv, err := os.ReadFile(filepath.Join("..", "inetlookup", "testdata", "geolite2_csv", fixtures[i]))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, part.dest, csv)
	}
	if GeoliteDue() {
		t.Fatal("complete GeoLite with a fresh timestamp should be ready without an update")
	}
	missing := parts[1].dest
	writeTestFile(t, missing+HASH_POSTFIX, []byte("existing-hash"))
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if !GeoliteDue() {
		t.Fatal("missing CSV must require an update even with its hash and a fresh timestamp")
	}
}

func TestGeolitePublishRestoresPreviousFiles(t *testing.T) {
	dir := t.TempDir()
	first := stagedGeoliteFile{path: filepath.Join(dir, "new.csv"), dest: filepath.Join(dir, "cidr-as.csv")}
	writeTestFile(t, first.dest, []byte("previous"))
	writeTestFile(t, first.path, []byte("current"))
	if err := first.prepareBackup(); err != nil {
		t.Fatal(err)
	}
	files := []stagedGeoliteFile{first, {path: filepath.Join(dir, "missing.csv"), dest: filepath.Join(dir, "country.csv")}}
	publishErr, rollbackErr := publishGeoliteFiles(files)
	if publishErr == nil || rollbackErr != nil {
		t.Fatalf("publish error = %v, rollback error = %v", publishErr, rollbackErr)
	}
	if got, err := os.ReadFile(first.dest); err != nil || string(got) != "previous" {
		t.Fatalf("previous file contents = %q, error = %v", got, err)
	}
}

func TestGeoliteVolumeLockWaitsForPublisher(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("uses flock")
	}
	root := t.TempDir()
	unlock, err := lockGeoliteUpdate(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	second, err := lockGeoliteUpdate(ctx, root)
	cancel()
	unlock()
	if err == nil {
		second()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error = %v, want deadline exceeded", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	after, err := lockGeoliteUpdate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	after()
}

func releaseZip(t *testing.T, name, payload string) []byte {
	t.Helper()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
