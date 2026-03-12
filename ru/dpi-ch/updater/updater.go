package updater

import (
	"context"
	"dpich/config"
	"dpich/httputil"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

const HASH_POSTFIX = ".hash"

func GeoliteUpdate() error {
	cfg := config.Get().Updater
	dir := path.Join(cfg.RootDir, cfg.Geolite.Dir)

	if err := geolitePartUpdate(cfg.Geolite.CidrAs.From, path.Join(dir, cfg.Geolite.CidrAs.To)); err != nil {
		return err
	}
	if err := geolitePartUpdate(cfg.Geolite.CidrCountry.From, path.Join(dir, cfg.Geolite.CidrCountry.To)); err != nil {
		return err
	}
	if err := geolitePartUpdate(cfg.Geolite.GeonameidCountry.From, path.Join(dir, cfg.Geolite.GeonameidCountry.To)); err != nil {
		return err
	}

	return nil
}

func geolitePartUpdate(from, to string) error {
	cfg := config.Get().Updater

	localHash, err := readLocalHash(to)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
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

	if localHash == remoteHash {
		return nil
	}

	log.Println("geoliteCidrAsUpdate:download", localHash, remoteHash)
	ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
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
