package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
)

type SelfCheckUpdatesResult struct {
	AssetUrl      string
	AssetFilename string
	AssetVersion  string
	Required      bool
}

const HASH_POSTFIX = ".hash"

var ErrInternal = errors.New("updater/self: internal")
var ErrUnsupportedOsOrArch = errors.New("updater/self: unsupported os/arch")

// Determines if it is time to update using the timestamp file
func TimeToUpdate(tsfile string) (bool, error) {
	cfg := config.Get().Updater
	dst := path.Join(cfg.RootDir, tsfile)

	if _, err := os.Stat(dst); err != nil {
		return true, nil
	}

	ts, err := readUpdateTimestamp(dst)
	if err != nil {
		return true, nil
	}

	delta := time.Duration(time.Now().Unix()-ts) * time.Second
	if delta > cfg.Period {
		return true, nil
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
func SelfUpdate(ctx context.Context, url, filename, version string) error {
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		log.Println("updater/self: could not locate executable path")
		return ErrInternal
	}

	// TODO: On windows, this hides the previous binary; it's a good idea to run a cleanup when the dpich is restarted.
	if err := selfupdate.UpdateTo(ctx, url, filename, exe); err != nil {
		log.Println("updater/self: error occurred while updating binary: ", err)
		return ErrInternal
	}

	log.Printf("updater/self: successfully updated to version %s", version)

	cfg := config.Get().Updater
	tsfile := path.Join(cfg.RootDir, cfg.SelfTsFile)
	err = writeUpdateTimestamp(tsfile)
	if err != nil {
		log.Printf("updater/self: fail to update ts file: %s", tsfile)
	}

	return nil
}

// Checks if there are new versions of itself.
func SelfCheckUpdates(ctx context.Context) (SelfCheckUpdatesResult, error) {
	cfg := config.Get().Updater
	latest, found, err := selfupdate.DetectLatest(context.Background(), selfupdate.NewRepositorySlug(cfg.Self.Owner, cfg.Self.Repo))
	if err != nil {
		log.Printf("updater/self: %s\n", err)
		return SelfCheckUpdatesResult{}, ErrInternal
	}

	if !found {
		log.Printf("updater/self: latest version for %s/%s not found\n", runtime.GOOS, runtime.GOARCH)
		return SelfCheckUpdatesResult{}, ErrUnsupportedOsOrArch
	}

	if latest.LessOrEqual(version.Value) {
		tsfile := path.Join(cfg.RootDir, cfg.SelfTsFile)
		err = writeUpdateTimestamp(tsfile)
		if err != nil {
			log.Printf("updater/self: fail to update ts file: %s", tsfile)
		}
		return SelfCheckUpdatesResult{Required: false}, nil
	}

	return SelfCheckUpdatesResult{
		AssetUrl:      latest.AssetURL,
		AssetFilename: latest.AssetName,
		AssetVersion:  latest.Version(),
		Required:      true,
	}, nil
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
	log.Println("updater/geolite: successfully updated")

	tsfile := path.Join(cfg.RootDir, cfg.InetlookupTsFile)
	err := writeUpdateTimestamp(tsfile)
	if err != nil {
		log.Printf("updater/geolite: fail to update ts file: %s", tsfile)
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
