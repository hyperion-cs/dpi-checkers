package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

const HASH_POSTFIX = ".hash"

func updateDue(tsfile string) bool {
	cfg := config.Get()
	data, err := os.ReadFile(cfg.DataPath(tsfile))
	if err != nil {
		return true
	}
	ts, err := strconv.ParseInt(string(data), 10, 64)
	return err != nil || time.Since(time.Unix(ts, 0)) > cfg.Updater.Period
}

func writeUpdateTimestamp(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
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
	if err := inetutil.GetAndUnmarshal(ctx, url, &respRaw, true, false); err != nil {
		return "", err
	}

	return respRaw.Sha, nil
}

func openDownload(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	return resp.Body, nil
}

func download(ctx context.Context, url, stagedPath string) error {
	body, err := openDownload(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	f, err := os.Create(stagedPath)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, body)
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("downloaded CSV is empty")
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
