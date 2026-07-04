package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

// gzipString returns plain compressed with gzip, for feeding a fake feed body.
func gzipString(t *testing.T, plain string) []byte {
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

// stdout (expand=true) yields decompressed newline-JSON — the default so the
// output pipes straight into a JSON tool (AC: "streams decompressed newline-JSON
// to stdout").
func TestDownloadFeedDecompressesToStdout(t *testing.T) {
	t.Parallel()

	const ndjson = `{"ip":"1.2.3.4"}` + "\n" + `{"ip":"5.6.7.8"}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipString(t, ndjson))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), true, nil); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if dst.String() != ndjson {
		t.Errorf("decompressed output = %q, want %q", dst.String(), ndjson)
	}
}

// -o without --decompress (expand=false) writes the raw gzip bytes verbatim, so
// large feeds are saved without spending CPU on decompression.
func TestDownloadFeedRawToFile(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), raw) {
		t.Errorf("raw output did not match the served gzip bytes")
	}
}

// A feed the token can't access must yield the same clear, feed-specific
// forbidden message as `feed status`, without leaking the upstream body.
func TestDownloadFeedForbiddenIsClear(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal upstream detail that must not leak", http.StatusForbidden)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	err := downloadFeed(context.Background(), client, "dch", "", spur.FeedJSONGzip, bufSink(&dst), true, nil)
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

// Requesting MMDB for a feed whose metadata has no MMDB container must fail
// clearly and before any download — availability is read from the metadata, not
// assumed from a static list.
func TestDownloadMMDBNotOfferedIsClear(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/latest") {
			t.Errorf("expected only a metadata request, got %q", r.URL.Path)
		}
		// A JSON-only feed: metadata carries no mmdb container.
		_, _ = w.Write([]byte(`{"json":{"date":"20250324","location":"20250324/feed.json.gz"}}`))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	err := downloadFeed(context.Background(), client, "dch", "", spur.FeedMMDB, bufSink(&dst), false, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "MMDB") || !strings.Contains(err.Error(), "dch") {
		t.Errorf("error %q should say the dch feed is not offered in MMDB", err.Error())
	}
	if dst.Len() != 0 {
		t.Errorf("nothing should have been downloaded, got %d bytes", dst.Len())
	}
}

// When the feed offers MMDB, the binary artifact streams through untouched (no
// gzip decompression) even though expand is set.
func TestDownloadMMDBStreamsRaw(t *testing.T) {
	t.Parallel()

	const mmdb = "\x00\x01MMDBbinary\xff"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"):
			_, _ = w.Write([]byte(`{"json":{"date":"20250324","location":"20250324/feed.json.gz"},"mmdb":{"date":"20250324","location":"20250324/feed.mmdb"}}`))
		case strings.HasSuffix(r.URL.Path, "/latest.mmdb"):
			_, _ = w.Write([]byte(mmdb))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedMMDB, bufSink(&dst), true, nil); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if dst.String() != mmdb {
		t.Errorf("MMDB output = %q, want the raw bytes %q", dst.String(), mmdb)
	}
}

func TestResolveDownloadFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		slug    string
		mmdb    bool
		asJSON  bool
		want    spur.FeedFormat
		wantErr bool
	}{
		{name: "default json", slug: "anonymous", want: spur.FeedJSONGzip},
		{name: "ipgeo defaults mmdb", slug: "ipgeo", want: spur.FeedMMDB},
		{name: "force mmdb", slug: "anonymous", mmdb: true, want: spur.FeedMMDB},
		{name: "force json", slug: "anonymous", asJSON: true, want: spur.FeedJSONGzip},
		{name: "ipgeo forced json", slug: "ipgeo", asJSON: true, want: spur.FeedJSONGzip},
		{name: "conflict", slug: "anonymous", mmdb: true, asJSON: true, wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveDownloadFormat(tc.slug, tc.mmdb, tc.asJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveDownloadFormat(%q, %v, %v) = %v, want error", tc.slug, tc.mmdb, tc.asJSON, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDownloadFormat: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveDownloadFormat(%q, %v, %v) = %v, want %v", tc.slug, tc.mmdb, tc.asJSON, got, tc.want)
			}
		})
	}
}

func TestResolveDownloadSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arg      string
		ipv6     bool
		realtime bool
		want     string
		wantErr  bool
	}{
		{name: "plain", arg: "anonymous", want: "anonymous"},
		{name: "ipv6 modifier", arg: "anonymous", ipv6: true, want: "anonymous-ipv6"},
		{name: "residential ipv6", arg: "anonymous-residential", ipv6: true, want: "anonymous-residential-ipv6"},
		{name: "realtime modifier", arg: "anonymous-residential", realtime: true, want: "anonymous-residential/realtime"},
		{name: "full slug direct", arg: "anonymous-residential/realtime", want: "anonymous-residential/realtime"},
		{name: "idempotent ipv6", arg: "anonymous-ipv6", ipv6: true, want: "anonymous-ipv6"},
		{name: "unknown", arg: "nope", wantErr: true},
		{name: "impossible combo", arg: "dch", ipv6: true, wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveDownloadSlug(tc.arg, tc.ipv6, tc.realtime)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveDownloadSlug(%q, %v, %v) = %q, want error", tc.arg, tc.ipv6, tc.realtime, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDownloadSlug: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveDownloadSlug(%q, %v, %v) = %q, want %q", tc.arg, tc.ipv6, tc.realtime, got, tc.want)
			}
		})
	}
}
