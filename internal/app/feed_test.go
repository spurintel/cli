package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func TestFetchFeedMetadataReturnsResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"json":{"date":"20250324","location":"20250324/feed.json.gz","filesize":100}}`))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	got, err := fetchFeedMetadata(context.Background(), client, "anonymous")
	if err != nil {
		t.Fatalf("fetchFeedMetadata: %v", err)
	}
	if got.JSON.Date != "20250324" {
		t.Errorf("JSON.Date = %q, want 20250324", got.JSON.Date)
	}
	if got.JSON.Filesize != 100 {
		t.Errorf("JSON.Filesize = %d, want 100", got.JSON.Filesize)
	}
}

// A feed the token can't access must yield a clear, feed-specific forbidden
// message — not the generic client sentinel and not a leaked upstream body
// (AC: "a feed type the token can't access returns a clear forbidden message").
func TestFetchFeedMetadataForbiddenIsClear(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal upstream detail that must not leak", http.StatusForbidden)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	_, err := fetchFeedMetadata(context.Background(), client, "dch")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "dch") {
		t.Errorf("forbidden error %q should name the feed type", msg)
	}
	if !strings.Contains(msg, "denied") && !strings.Contains(msg, "subscription") {
		t.Errorf("forbidden error %q should explain access is denied", msg)
	}
	if strings.Contains(msg, "internal upstream detail") {
		t.Fatalf("error leaked upstream body: %q", msg)
	}
}
