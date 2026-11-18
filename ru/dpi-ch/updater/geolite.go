package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type geolitePart struct {
	source string
	dest   string
	lookup string
}

func geoliteParts() [3]geolitePart {
	cfg := config.Get()
	asn, country, names := cfg.GeoliteOutputPaths()
	geo := cfg.Updater.Geolite
	lookup := cfg.InetlookupGeolitecsv
	return [3]geolitePart{
		{geo.CidrAs.From, asn, lookup.CidrAs},
		{geo.CidrCountry.From, country, lookup.CidrCountry},
		{geo.GeonameidCountry.From, names, lookup.GeonameidCountry},
	}
}

// GeoliteDue also checks the updater's destinations, which can be configured separately.
func GeoliteDue() bool {
	if updateDue(config.Get().Updater.InetlookupTsFile) {
		return true
	}
	for _, part := range geoliteParts() {
		if !geoliteFileAvailable(part.dest) || (part.lookup != part.dest && !geoliteFileAvailable(part.lookup)) {
			return true
		}
	}
	return false
}

func geoliteFileAvailable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

type stagedGeoliteFile struct {
	path   string
	dest   string
	backup string
}

var geoliteUpdateMu sync.Mutex

func GeoliteUpdate(ctx context.Context) error {
	geoliteUpdateMu.Lock()
	defer geoliteUpdateMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg := config.Get().Updater
	if config.Container() {
		unlock, err := lockGeoliteUpdate(ctx, cfg.RootDir)
		if err != nil {
			return fmt.Errorf("updater/geolite: lock data directory: %w", err)
		}
		defer unlock()
		// Another container may have refreshed the shared data while we waited.
		if !cfg.ForceInetlookupUpdate && !GeoliteDue() {
			return nil
		}
	}

	var files []stagedGeoliteFile
	keepBackups := false
	for _, part := range geoliteParts() {
		staged, dir, err := stageGeolitePart(ctx, part)
		if dir != "" {
			defer func() {
				if !keepBackups {
					os.RemoveAll(dir)
				}
			}()
		}
		if err != nil {
			return fmt.Errorf("updater/geolite: %s: %w", part.dest, err)
		}
		files = append(files, staged...)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	publishErr, rollbackErr := publishGeoliteFiles(files)
	if publishErr != nil {
		keepBackups = rollbackErr != nil
		return fmt.Errorf("updater/geolite: %w", errors.Join(publishErr, rollbackErr))
	}

	tsfile := config.Get().DataPath(cfg.InetlookupTsFile)
	if err := writeUpdateTimestamp(tsfile); err != nil {
		return fmt.Errorf("updater/geolite: update timestamp: %w", err)
	}
	log.Println("updater/geolite: successfully updated")
	return nil
}

func stageGeolitePart(ctx context.Context, part geolitePart) ([]stagedGeoliteFile, string, error) {
	cfg := config.Get().Updater
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	hash, err := remoteHash(ctx, cfg.Geolite.Owner, cfg.Geolite.Repo, part.source, cfg.Geolite.Branch)
	if err != nil {
		return nil, "", err
	}
	if hash == "" {
		return nil, "", errors.New("missing remote file hash")
	}

	if !cfg.ForceInetlookupUpdate && geoliteFileAvailable(part.dest) {
		localHash, err := readLocalHash(part.dest)
		if err != nil {
			return nil, "", err
		}
		if localHash == hash {
			return nil, "", nil
		}
	}

	parent := filepath.Dir(part.dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, "", err
	}
	dir, err := os.MkdirTemp(parent, ".geolite-")
	if err != nil {
		return nil, "", err
	}

	csv := filepath.Join(dir, "download.csv")
	if err := download(ctx, contentUrl(cfg.Geolite.Owner, cfg.Geolite.Repo, part.source, cfg.Geolite.Branch), csv); err != nil {
		return nil, dir, err
	}
	if err := os.WriteFile(csv+HASH_POSTFIX, []byte(hash), 0o644); err != nil {
		return nil, dir, err
	}

	files := []stagedGeoliteFile{
		{path: csv, dest: part.dest},
		{path: csv + HASH_POSTFIX, dest: part.dest + HASH_POSTFIX},
	}
	for i := range files {
		if err := files[i].prepareBackup(); err != nil {
			return nil, dir, err
		}
	}
	return files, dir, nil
}

func (file *stagedGeoliteFile) prepareBackup() error {
	info, err := os.Lstat(file.dest)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%q is not a regular file", file.dest)
	}

	file.backup = file.path + ".previous"
	if err := os.Link(file.dest, file.backup); err == nil {
		return nil
	}
	return copyGeoliteFile(file.dest, file.backup, info.Mode().Perm())
}

func copyGeoliteFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// All downloads and backups are staged before any existing file is replaced.
// Undo completed replacements if a local rename fails during publication.
func publishGeoliteFiles(files []stagedGeoliteFile) (publishErr, rollbackErr error) {
	for i, file := range files {
		if err := os.Rename(file.path, file.dest); err != nil {
			for j := i - 1; j >= 0; j-- {
				previous := files[j]
				if err := previous.restore(); err != nil {
					rollbackErr = errors.Join(rollbackErr, err)
				}
			}
			return err, rollbackErr
		}
	}
	return nil, nil
}

func (file stagedGeoliteFile) restore() error {
	if file.backup == "" {
		return os.Remove(file.dest)
	}
	return os.Rename(file.backup, file.dest)
}
