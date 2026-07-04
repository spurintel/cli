package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func TestFetchStatusReturnsResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":true,"queriesRemaining":999,"serviceTier":"online"}`))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.Base = srv.URL

	got, err := fetchStatus(context.Background(), client)
	if err != nil {
		t.Fatalf("fetchStatus: %v", err)
	}
	if !got.Active {
		t.Error("status Active = false, want true")
	}
	if got.QueriesRemaining != 999 {
		t.Errorf("QueriesRemaining = %d, want 999", got.QueriesRemaining)
	}
	if got.ServiceTier != "online" {
		t.Errorf("ServiceTier = %q, want online", got.ServiceTier)
	}
}

// An invalid token must surface the client's clean unauthorized message, not a
// raw status line or leaked upstream body (AC: "clear unauthorized message").
func TestFetchStatusInvalidTokenIsClean(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal upstream detail that must not leak", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := spur.NewClient("bad-token")
	client.Base = srv.URL

	_, err := fetchStatus(context.Background(), client)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err.Error() != spur.ErrUnauthorized.Error() {
		t.Fatalf("error = %q, want %q", err.Error(), spur.ErrUnauthorized.Error())
	}
	if strings.Contains(err.Error(), "internal upstream detail") {
		t.Fatalf("error leaked upstream body: %q", err.Error())
	}
}
