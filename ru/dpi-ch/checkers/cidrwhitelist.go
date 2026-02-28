// Checks if a censor restricts tcp/udp/etc connections by ip subnets (aka cidr censorship)

package checkers

import (
	"context"
	"dpich/config"
	"dpich/netutil"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
)

var ErrCidrWhitelistDetected = errors.New("cidr whitelist detected")
var ErrCidrWhitelistNoInetAccess = errors.New("no internet access")

func CidrWhitelist() error {
	cfg := config.Get().Checkers.CidrWhitelist

	var wg sync.WaitGroup
	var wlCount, normCount int32

	wlCtx, wlCancel := context.WithTimeout(context.Background(), cfg.Timeout)
	normCtx, normCancel := context.WithTimeout(context.Background(), cfg.Timeout)

	defer wlCancel()
	defer normCancel()

	for _, url := range cfg.NormEndpoints {
		wg.Go(func() {
			if err := netutil.Head(normCtx, http.DefaultClient, url, true); err == nil {
				normCancel()
				wlCancel() // results are already clear
				atomic.AddInt32(&normCount, 1)
			}
		})
	}

	for _, url := range cfg.WlEndpoints {
		wg.Go(func() {
			if err := netutil.Head(wlCtx, http.DefaultClient, url, true); err == nil {
				wlCancel()
				atomic.AddInt32(&wlCount, 1)
			}
		})
	}

	wg.Wait()

	// Resources not on the whitelist are available
	if normCount > 0 {
		return nil
	}

	// ONLY resources from the whitelist are available
	if wlCount > 0 {
		return ErrCidrWhitelistDetected
	}

	// It seems there is no Internet connection
	return ErrCidrWhitelistNoInetAccess
}
