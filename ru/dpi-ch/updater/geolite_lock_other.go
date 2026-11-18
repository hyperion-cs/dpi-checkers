//go:build !linux

package updater

import "context"

func lockGeoliteUpdate(context.Context, string) (func(), error) {
	return func() {}, nil
}
