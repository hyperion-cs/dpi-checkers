package updater

import (
	"archive/zip"
	"context"
	"dpich/config"
	"dpich/httputil"
	"dpich/internal/version"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type SelfCheckUpdatesResult struct {
	Url      string
	Name     string
	Required bool
}

const HASH_POSTFIX = ".hash"

var ErrUnsupportedOsOrArch = errors.New("updater/self: unsupported os or arch")

// Determines if it is time to update itself and inetlookup.
// Independently updates the associated timestamp.
func TimeToUpdate() (bool, error) {
	cfg := config.Get().Updater
	dst := path.Join(cfg.RootDir, cfg.UpdateTsFile)

	if _, err := os.Stat(dst); err != nil {
		return true, writeUpdateTimestamp(dst)
	}

	ts, err := readUpdateTimestamp(dst)
	if err != nil {
		return true, writeUpdateTimestamp(dst)
	}

	delta := time.Duration(time.Now().Unix()-ts) * time.Second
	if delta > cfg.Period {
		return true, writeUpdateTimestamp(dst)
	}

	return false, nil
}

func readUpdateTimestamp(dst string) (int64, error) {
	data, err := os.ReadFile(dst)
	if err != nil {
		return 0, err
	}

	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return 0, err
	}

	return ts, nil
}

func writeUpdateTimestamp(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return os.WriteFile(dst, []byte(ts), 0644)
}

// Updates itself. Automatically downloads, unzips, and replaces the executable file.
// If the update is successful, it is necessary to restart manually.
func SelfUpdate(ctx context.Context, name, url string) error {
	cfg := config.Get().Updater
	dir := path.Join(cfg.RootDir, cfg.Self.Dir)
	zipDst := path.Join(dir, name)
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	if err := download(ctx, url, zipDst); err != nil {
		return err
	}

	if err := unzip(zipDst, dir); err != nil {
		return err
	}

	if err := os.Remove(zipDst); err != nil {
		return err
	}

	log.Println("updater/self: executable downloaded and unzipped")
	selfPath, err := os.Executable()
	if err != nil {
		return err
	}

	newPath := path.Join(dir, cfg.Self.Bin)
	if err := os.Chmod(newPath, 0755); err != nil {
		return err
	}

	cmd := exec.Command(selfPath, "--update", selfPath, newPath)
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// Replaces the executable file from with to and kills the current process.
func SelfUpdateExecutable(from, to string) error {
	if err := os.Remove(from); err != nil {
		return err
	}
	if err := os.Rename(to, from); err != nil {
		return err
	}
	log.Printf("updater/self: executable replaced %s => %s\n", from, to)
	os.Exit(0)
	return nil
}

// Checks if there are new versions of itself.
func SelfCheckUpdates(ctx context.Context) (SelfCheckUpdatesResult, error) {
	cfg := config.Get().Updater
	url := latestReleaseUrl(cfg.Self.Owner, cfg.Self.Repo)
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	type assetType struct {
		Name               string
		BrowserDownloadUrl string `json:"browser_download_url"`
	}

	var respRaw struct {
		TagName string `json:"tag_name"`
		Assets  []assetType
	}
	if err := httputil.GetAndUnmarshal(ctx, http.DefaultClient, url, &respRaw, true, false); err != nil {
		return SelfCheckUpdatesResult{}, err
	}

	currVer := version.Value
	latestVer := strings.TrimPrefix(respRaw.TagName, cfg.Self.TagPrefix)

	if currVer == latestVer {
		return SelfCheckUpdatesResult{Required: false}, nil
	}

	goos := runtime.GOOS
	if goos == "darwin" {
		goos = "macos"
	}

	assetIdx := slices.IndexFunc(respRaw.Assets, func(x assetType) bool {
		return strings.Contains(x.Name, runtime.GOARCH) && strings.Contains(x.Name, goos)
	})

	if assetIdx == -1 {
		return SelfCheckUpdatesResult{}, ErrUnsupportedOsOrArch
	}

	asset := respRaw.Assets[assetIdx]
	log.Printf("updater/self: available %s (%s => %s)\n", asset.Name, currVer, latestVer)
	return SelfCheckUpdatesResult{Url: asset.BrowserDownloadUrl, Name: asset.Name, Required: true}, nil
}

func GeoliteUpdate(ctx context.Context) error {
	cfg := config.Get().Updater
	dir := path.Join(cfg.RootDir, cfg.Geolite.Dir)

	if err := geolitePartUpdate(ctx, cfg.Geolite.CidrAs.From, path.Join(dir, cfg.Geolite.CidrAs.To)); err != nil {
		return err
	}
	if err := geolitePartUpdate(ctx, cfg.Geolite.CidrCountry.From, path.Join(dir, cfg.Geolite.CidrCountry.To)); err != nil {
		return err
	}
	if err := geolitePartUpdate(ctx, cfg.Geolite.GeonameidCountry.From, path.Join(dir, cfg.Geolite.GeonameidCountry.To)); err != nil {
		return err
	}

	return nil
}

func geolitePartUpdate(ctx context.Context, from, to string) error {
	cfg := config.Get().Updater

	localHash, err := readLocalHash(to)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	remoteHash, err := remoteHash(
		ctx,
		cfg.Geolite.Owner,
		cfg.Geolite.Repo,
		from,
		cfg.Geolite.Branch,
	)
	if err != nil {
		return err
	}

	if localHash == remoteHash && !cfg.ForceInetlookupUpdate {
		return nil
	}

	log.Println("geoliteCidrAsUpdate:download", localHash, remoteHash)
	ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	download(ctx, contentUrl(
		cfg.Geolite.Owner,
		cfg.Geolite.Repo,
		from,
		cfg.Geolite.Branch,
	), to)

	return writeLocalHash(to, remoteHash)
}

func writeLocalHash(path, hash string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path+HASH_POSTFIX, []byte(hash), 0o644); err != nil {
		return err
	}
	return nil
}

func readLocalHash(path string) (string, error) {
	b, err := os.ReadFile(path + HASH_POSTFIX)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func remoteHash(ctx context.Context, owner, repo, path, branch string) (string, error) {
	url := attrUrl(owner, repo, path, branch)
	var respRaw struct{ Sha string }
	if err := httputil.GetAndUnmarshal(ctx, http.DefaultClient, url, &respRaw, true, false); err != nil {
		return "", err
	}

	return respRaw.Sha, nil
}

func download(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

func attrUrl(owner, repo, path, branch string) string {
	return fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		url.PathEscape(owner),
		url.PathEscape(repo),
		url.PathEscape(path),
		url.QueryEscape(branch),
	)
}

func contentUrl(owner, repo, path, branch string) string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/refs/heads/%s/%s",
		url.PathEscape(owner),
		url.PathEscape(repo),
		url.PathEscape(branch),
		url.PathEscape(path),
	)
}

func latestReleaseUrl(owner, repo string) string {
	return fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/releases/latest",
		url.PathEscape(owner),
		url.PathEscape(repo),
	)
}

func unzip(zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		p := filepath.Join(dst, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(p), 0755)

		in, _ := f.Open()
		out, _ := os.Create(p)

		io.Copy(out, in)

		in.Close()
		out.Close()
	}
	return nil
}
