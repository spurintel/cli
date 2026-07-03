package spur

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadBodyRejectsOversizeResponses(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("a", int(maxResponseBodyBytes)+1)
	_, err := readBody(strings.NewReader(body), maxResponseBodyBytes)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestGetStatusReturnsSanitizedAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that should not escape", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("token")
	client.Base = srv.URL

	_, err := client.GetStatus(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if got, want := apiErr.Message(), ErrForbidden.Error(); got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "internal detail") {
		t.Fatalf("error leaked upstream body: %q", err.Error())
	}
}
