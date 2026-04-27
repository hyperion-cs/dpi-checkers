// Checks if a censor restricts tcp/udp/etc connections by ip subnets (aka cidr censorship)

package checkers

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
)

var ErrCidrWhitelistDetected = errors.New("cidr whitelist detected")
var ErrCidrWhitelistNoInetAccess = errors.New("no internet access")

func CidrWhitelist() error {
	cfg := config.Get().Checkers.CidrWhitelist

	var wg sync.WaitGroup
	var wlCount, regCount int32

	wlCtx, wlCancel := context.WithTimeout(context.Background(), cfg.Timeout)
	regCtx, regCancel := context.WithTimeout(context.Background(), cfg.Timeout)

	defer wlCancel()
	defer regCancel()

	for _, url := range cfg.Regular {
		wg.Go(func() {
			if err := inetutil.Head(regCtx, url, true, true); err == nil {
				regCancel()
				wlCancel() // results are already clear
				atomic.AddInt32(&regCount, 1)
			}
		})
	}

	for _, url := range cfg.Whitelisted {
		wg.Go(func() {
			if err := inetutil.Head(wlCtx, url, true, true); err == nil {
				wlCancel()
				atomic.AddInt32(&wlCount, 1)
			}
		})
	}

	wg.Wait()

	// Resources not on the whitelist are available
	if regCount > 0 {
		return nil
	}

	// ONLY resources from the whitelist are available
	if wlCount > 0 {
		return ErrCidrWhitelistDetected
	}

	// It seems there is no Internet connection
	return ErrCidrWhitelistNoInetAccess
}
