package spur

import (
	"errors"
	"testing"
)

func TestAPIErrorMessageForKnownStatus(t *testing.T) {
	t.Parallel()

	err := newAPIError(401)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if got, want := apiErr.Message(), ErrUnauthorized.Error(); got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
}

func TestAPIErrorMessageForUnknownStatus(t *testing.T) {
	t.Parallel()

	err := newAPIError(418)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if got, want := apiErr.Message(), "spur api request failed with status 418"; got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
}
