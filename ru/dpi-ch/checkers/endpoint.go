// Checks the availability of some web resources (endpoints)

package checkers

import (
	"context"
	"dpich/config"
	"dpich/netutil"
	"errors"
	"net/http"
)

var ErrEndpointFail = errors.New("endpoint fail")

func Endpoint(url string) error {
	cfg := config.Get().Checkers.Endpoint
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if _, err := netutil.Get(ctx, http.DefaultClient, url, true); err == nil {
		return nil
	}

	return ErrEndpointFail
}
