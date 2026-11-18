package updater

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Containers can share a named volume. Serialize refreshes in that volume and
// let the kernel release the lock when a container exits.
func lockGeoliteUpdate(ctx context.Context, root string) (func(), error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, ".geolite-update.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	retry := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { file.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-retry.C:
		}
	}
}
