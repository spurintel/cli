package app

import (
	"errors"

	"github.com/spurintel/cli/internal/spur"
)

// collapseAPIError maps a client failure to the operator-facing message every
// command shows: an *spur.APIError becomes its stable, sanitized Message()
// (never a leaked upstream body), any other error passes through unchanged.
// overrides substitutes a command-specific, actionable message for a given
// sentinel — e.g. a feed command turning ErrForbidden into "your subscription
// does not include this feed" — so the per-command phrasing lives at the call
// site while the sanitization contract lives here.
func collapseAPIError(err error, overrides map[error]string) error {
	var apiErr *spur.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	for sentinel, msg := range overrides {
		if errors.Is(apiErr, sentinel) {
			return errors.New(msg)
		}
	}
	return errors.New(apiErr.Message())
}
