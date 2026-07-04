package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func TestFetchTagMetadataReturnsResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/v2/metadata/tags/OXYLABS_PROXY") {
			t.Errorf("request path = %q, want .../v2/metadata/tags/OXYLABS_PROXY", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Mixed shapes on purpose: bare-number metrics and string-form
		// booleans, both of which the tolerant client must accept.
		_, _ = w.Write([]byte(`{"tag":"OXYLABS_PROXY","name":"Oxylabs Proxy","categories":["RESIDENTIAL_PROXY"],"isAnonymous":"true","metrics":{"distinctIPs":123456}}`))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.Base = srv.URL

	got, err := fetchTagMetadata(context.Background(), client, "OXYLABS_PROXY")
	if err != nil {
		t.Fatalf("fetchTagMetadata: %v", err)
	}
	if got.Tag != "OXYLABS_PROXY" {
		t.Errorf("Tag = %q, want OXYLABS_PROXY", got.Tag)
	}
	if got.IsAnonymous == nil || !bool(*got.IsAnonymous) {
		t.Errorf("IsAnonymous = %v, want true", got.IsAnonymous)
	}
	if got.Metrics == nil || got.Metrics.DistinctIPs.String() != "123456" {
		t.Errorf("Metrics.DistinctIPs = %v, want 123456", got.Metrics)
	}
}

// An unknown tag (404) must become a clear, tag-specific message naming the
// tag, not the bare "not found" sentinel (AC: "clear, actionable error").
func TestFetchTagMetadataUnknownTagIsActionable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such tag", http.StatusNotFound)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.Base = srv.URL

	_, err := fetchTagMetadata(context.Background(), client, "NOPE_PROXY")
	if err == nil {
		t.Fatal("expected an error for an unknown tag, got nil")
	}
	if !strings.Contains(err.Error(), "NOPE_PROXY") {
		t.Errorf("error should name the unknown tag, got %q", err.Error())
	}
	if err.Error() == spur.ErrNotFound.Error() {
		t.Errorf("error should be tag-specific, not the bare %q sentinel", spur.ErrNotFound.Error())
	}
}

// An invalid token must surface the client's clean unauthorized message, not a
// raw status line or leaked upstream body.
func TestFetchTagMetadataInvalidTokenIsClean(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal upstream detail that must not leak", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := spur.NewClient("bad-token")
	client.Base = srv.URL

	_, err := fetchTagMetadata(context.Background(), client, "OXYLABS_PROXY")
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
