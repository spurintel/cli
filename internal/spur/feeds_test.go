package spur

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
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

// gzipBytes is a small gzip payload used to stand in for a feed file.
func gzipBytes(t *testing.T, plain string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(plain)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// DownloadFeed hits the latest .json.gz path with the Token header and streams
// the (compressed) body back verbatim, along with the CDN Content-Length.
func TestDownloadFeedStreamsBody(t *testing.T) {
	t.Parallel()

	payload := gzipBytes(t, `{"ip":"1.2.3.4"}`+"\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/anonymous/latest.json.gz"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	body, size, err := client.DownloadFeed(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("DownloadFeed: %v", err)
	}
	defer func() { _ = body.Close() }()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("streamed body did not match the served bytes")
	}
	if size != int64(len(payload)) {
		t.Errorf("size = %d, want %d", size, len(payload))
	}
}

// The date and format together select the artifact path: latest vs historical,
// json.gz vs mmdb; a nested slug keeps its slash.
func TestDownloadFeedPathSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		feedType string
		date     string
		format   FeedFormat
		wantPath string
	}{
		{"json historical", "anonymous", "20250323", FeedJSONGzip, "/v2/anonymous/20250323/feed.json.gz"},
		{"json nested latest", "anonymous-residential/realtime", "", FeedJSONGzip, "/v2/anonymous-residential/realtime/latest.json.gz"},
		{"mmdb latest", "anonymous", "", FeedMMDB, "/v2/anonymous/latest.mmdb"},
		{"mmdb historical", "ipgeo", "20250323", FeedMMDB, "/v2/ipgeo/20250323/feed.mmdb"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Path; got != tc.wantPath {
					t.Errorf("path = %q, want %q", got, tc.wantPath)
				}
				_, _ = w.Write([]byte("body"))
			}))
			defer srv.Close()

			client := NewClient("secret")
			client.FeedsBase = srv.URL

			body, _, err := client.DownloadFeed(context.Background(), tc.feedType, tc.date, tc.format)
			if err != nil {
				t.Fatalf("DownloadFeed: %v", err)
			}
			_ = body.Close()
		})
	}
}

// A 403 must surface a sanitized APIError wrapping ErrForbidden, not the
// upstream error page.
func TestDownloadFeedForbiddenIsSanitized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that should not escape", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	_, _, err := client.DownloadFeed(context.Background(), "dch", "", FeedJSONGzip)
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

// The feeds host redirects to the CDN; the Token must not follow across the
// host boundary, or the Spur credential would leak to the third-party CDN.
func TestDownloadFeedStripsTokenOnRedirect(t *testing.T) {
	t.Parallel()

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "" {
			t.Errorf("Token forwarded to CDN = %q, want empty", got)
		}
		_, _ = w.Write(gzipBytes(t, "{}\n"))
	}))
	defer cdn.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token on initial request = %q, want secret", got)
		}
		http.Redirect(w, r, cdn.URL+"/signed/feed.json.gz", http.StatusFound)
	}))
	defer origin.Close()

	client := NewClient("secret")
	client.FeedsBase = origin.URL

	body, _, err := client.DownloadFeed(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("DownloadFeed: %v", err)
	}
	_ = body.Close()
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
