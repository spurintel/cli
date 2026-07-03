package spur

import (
	"errors"
	"fmt"
)

// Sentinel errors mapped from Context API HTTP status codes. Tools wrap these
// into MCP tool errors with a human-readable message and, where helpful, a
// remediation hint (for example, "run get_api_status to check your quota").
var (
	ErrBadRequest       = errors.New("bad request: IP must be a public IPv4 or IPv6 address")
	ErrUnauthorized     = errors.New("unauthorized: SPUR_TOKEN is missing or invalid")
	ErrForbidden        = errors.New("forbidden: token lacks access to the Context API")
	ErrThrottled        = errors.New("throttled: insufficient queries remaining in this billing cycle")
	ErrNotFound         = errors.New("not found")
	ErrResponseTooLarge = errors.New("response exceeded the safe size limit")
	ErrServer           = errors.New("spur server error")
)

// APIError wraps the sentinel for a given status code. MCP callers should use
// Message() so they get a stable, sanitized user-facing error string.
type APIError struct {
	Status int
	Kind   error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("spur api error (status %d): %v", e.Status, e.Kind)
}

func (e *APIError) Unwrap() error { return e.Kind }

// Message returns a stable user-facing string without including raw upstream
// response bodies.
func (e *APIError) Message() string {
	switch {
	case e == nil:
		return ""
	case errors.Is(e, ErrBadRequest),
		errors.Is(e, ErrUnauthorized),
		errors.Is(e, ErrForbidden),
		errors.Is(e, ErrThrottled),
		errors.Is(e, ErrNotFound),
		errors.Is(e, ErrServer):
		return e.Kind.Error()
	default:
		return fmt.Sprintf("spur api request failed with status %d", e.Status)
	}
}

// newAPIError builds an APIError for a raw status code.
func newAPIError(status int) error {
	var kind error
	switch status {
	case 400:
		kind = ErrBadRequest
	case 401:
		kind = ErrUnauthorized
	case 403:
		kind = ErrForbidden
	case 404:
		kind = ErrNotFound
	case 429:
		kind = ErrThrottled
	default:
		if status >= 500 {
			kind = ErrServer
		} else {
			kind = fmt.Errorf("unexpected status %d", status)
		}
	}
	return &APIError{Status: status, Kind: kind}
}
