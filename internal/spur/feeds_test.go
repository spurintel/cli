package spur

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A daily feed's metadata document carries the full JSON container (and an MMDB
// container when the feed is offered in that format); GetFeedMetadata must parse
// every field and stamp the requested feed type onto the result.
func TestGetFeedMetadataParsesDailyDocument(t *testing.T) {
	t.Parallel()

	const body = `{
		"json": {
			"location": "20250324/feed.json.gz",
			"date": "20250324",
			"generated_at": "2025-03-24T00:59:41Z",
			"available_at": "2025-03-24T00:59:56Z",
			"filesize": 89553131
		},
		"mmdb": {
			"location": "20250324/feed.mmdb",
			"date": "20250324",
			"filesize": 42
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/anonymous/latest"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	md, err := client.GetFeedMetadata(context.Background(), "anonymous")
	if err != nil {
		t.Fatalf("GetFeedMetadata: %v", err)
	}
	if md.Type != "anonymous" {
		t.Errorf("Type = %q, want anonymous", md.Type)
	}
	if md.JSON.Date != "20250324" {
		t.Errorf("JSON.Date = %q, want 20250324", md.JSON.Date)
	}
	if md.JSON.Location != "20250324/feed.json.gz" {
		t.Errorf("JSON.Location = %q", md.JSON.Location)
	}
	if md.JSON.GeneratedAt != "2025-03-24T00:59:41Z" {
		t.Errorf("JSON.GeneratedAt = %q", md.JSON.GeneratedAt)
	}
	if md.JSON.AvailableAt != "2025-03-24T00:59:56Z" {
		t.Errorf("JSON.AvailableAt = %q", md.JSON.AvailableAt)
	}
	if md.JSON.Filesize != 89553131 {
		t.Errorf("JSON.Filesize = %d, want 89553131", md.JSON.Filesize)
	}
	if md.MMDB == nil {
		t.Fatal("MMDB container missing, want present")
	}
	if md.MMDB.Location != "20250324/feed.mmdb" {
		t.Errorf("MMDB.Location = %q", md.MMDB.Location)
	}
}

// The realtime feed uses a nested path segment and returns a minimal document
// (only date + location). The slash must survive path construction, and the
// absent optional fields must decode as their zero values.
func TestGetFeedMetadataRealtimeNestedPath(t *testing.T) {
	t.Parallel()

	const body = `{"json":{"date":"2025-03-25T13:00:04Z","location":"realtime/20250325/1300.json.gz"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/anonymous-residential/realtime/latest"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	md, err := client.GetFeedMetadata(context.Background(), "anonymous-residential/realtime")
	if err != nil {
		t.Fatalf("GetFeedMetadata: %v", err)
	}
	if md.Type != "anonymous-residential/realtime" {
		t.Errorf("Type = %q", md.Type)
	}
	if md.JSON.Date != "2025-03-25T13:00:04Z" {
		t.Errorf("JSON.Date = %q", md.JSON.Date)
	}
	if md.JSON.Filesize != 0 || md.JSON.GeneratedAt != "" || md.JSON.AvailableAt != "" {
		t.Errorf("realtime optional fields should be zero, got %+v", md.JSON)
	}
	if md.MMDB != nil {
		t.Errorf("realtime document has no MMDB container, got %+v", md.MMDB)
	}
}

// A feed the token cannot access returns 403; the client must surface a
// sanitized APIError wrapping ErrForbidden without leaking the upstream body.
func TestGetFeedMetadataForbiddenIsSanitized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that should not escape", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	_, err := client.GetFeedMetadata(context.Background(), "anonymous")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if !errors.Is(apiErr, ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
	if strings.Contains(err.Error(), "internal detail") {
		t.Fatalf("error leaked upstream body: %q", err.Error())
	}
}

func TestGetFeedMetadataRequiresType(t *testing.T) {
	t.Parallel()
	client := NewClient("secret")
	if _, err := client.GetFeedMetadata(context.Background(), "  "); err == nil {
		t.Fatal("empty feed type = nil error, want error")
	}
}

func TestLookupFeedType(t *testing.T) {
	t.Parallel()

	// Every catalog entry the ticket calls out must resolve, including the IPv6
	// and realtime variants.
	for _, name := range []string{
		"anonymous",
		"anonymous-ipv6",
		"anonymous-residential",
		"anonymous-residential-ipv6",
		"anonymous-residential/realtime",
		"service-metrics",
		"dch",
		"ipgeo",
		"ipsummary",
	} {
		if _, ok := LookupFeedType(name); !ok {
			t.Errorf("LookupFeedType(%q) = not found, want found", name)
		}
	}

	if _, ok := LookupFeedType("nope"); ok {
		t.Error("LookupFeedType(\"nope\") = found, want not found")
	}
}
