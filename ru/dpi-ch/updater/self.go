package updater

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/creativeprojects/go-selfupdate/update"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
)

type SelfCheckUpdatesResult struct {
	AssetUrl      string
	AssetFilename string
	AssetVersion  string
	Required      bool
}

var ErrUnsupportedOsOrArch = errors.New("updater/self: unsupported os/arch")

func SelfUpdateDue() bool {
	return updateDue(config.Get().Updater.SelfTsFile)
}

func SelfCheckUpdates(ctx context.Context) (SelfCheckUpdatesResult, error) {
	cfg := config.Get().Updater
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// DetectLatest defaults to published releases and skips drafts and prereleases.
	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.NewRepositorySlug(cfg.Self.Owner, cfg.Self.Repo))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return SelfCheckUpdatesResult{}, ctxErr
	}
	if err != nil {
		return SelfCheckUpdatesResult{}, fmt.Errorf("check for a dpich release: %w", err)
	}
	if !found {
		return SelfCheckUpdatesResult{}, ErrUnsupportedOsOrArch
	}
	if latest.LessOrEqual(version.Value) {
		markSelfUpdate()
		return SelfCheckUpdatesResult{}, nil
	}
	return SelfCheckUpdatesResult{
		AssetUrl:      latest.AssetURL,
		AssetFilename: latest.AssetName,
		AssetVersion:  latest.Version(),
		Required:      true,
	}, nil
}

func SelfUpdate(ctx context.Context, url, filename, version string) error {
	exe, err := selfUpdateTarget()
	if err != nil {
		return fmt.Errorf("locate dpich executable: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, config.Get().Updater.Timeout)
	defer cancel()
	if err := applyRelease(ctx, url, filename, exe); err != nil {
		return fmt.Errorf("update %q: %w", exe, err)
	}
	markSelfUpdate()
	log.Printf("updater/self: updated to %s", version)
	return nil
}

func applyRelease(ctx context.Context, url, filename, target string) error {
	body, err := openDownload(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()

	// Release archives use a fixed name, even if the installed file was renamed.
	binary, err := selfupdate.DecompressCommand(body, filename, "dpich", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if closer, ok := binary.(io.Closer); ok && binary != body {
		defer closer.Close()
	}
	reader := bufio.NewReader(binary)
	if _, err := reader.Peek(1); err != nil {
		return fmt.Errorf("read release executable: %w", err)
	}
	if err := update.Apply(reader, update.Options{TargetPath: target}); err != nil {
		if rollback := update.RollbackError(err); rollback != nil {
			return fmt.Errorf("%w (restore previous executable: %v)", err, rollback)
		}
		return err
	}
	return nil
}

func selfUpdateTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "android" && isAndroidLinker(exe) {
		var argv0 string
		if len(os.Args) != 0 {
			argv0 = os.Args[0]
		}
		return termuxExecutable(os.Getenv("TERMUX_EXEC__PROC_SELF_EXE"), argv0)
	}
	return filepath.EvalSymlinks(exe)
}

func isAndroidLinker(path string) bool {
	base := filepath.Base(path)
	return base == "linker" || base == "linker64"
}

func termuxExecutable(termuxPath, argv0 string) (string, error) {
	// Termux can launch dpich through the linker, making /proc/self/exe point to the linker.
	for _, candidate := range []string{termuxPath, argv0} {
		if !filepath.IsAbs(candidate) {
			continue
		}
		target, err := filepath.EvalSymlinks(candidate)
		if err != nil || isAndroidLinker(target) {
			continue
		}
		info, err := os.Stat(target)
		if err == nil && info.Mode().IsRegular() {
			return target, nil
		}
	}
	return "", errors.New("unable to resolve the Termux executable; run dpich by its absolute path")
}

func markSelfUpdate() {
	cfg := config.Get()
	if err := writeUpdateTimestamp(cfg.DataPath(cfg.Updater.SelfTsFile)); err != nil {
		log.Printf("updater/self: write timestamp: %v", err)
	}
}
