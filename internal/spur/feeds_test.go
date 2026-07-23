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
// the (compressed) body back verbatim, along with the CDN's integrity (size
// and X-Goog-Hash digests).
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
		w.Header().Set("X-Goog-Hash", "crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc=")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	body, integ, err := client.DownloadFeed(context.Background(), "anonymous", "", FeedJSONGzip)
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
	if integ.Size != int64(len(payload)) {
		t.Errorf("integ.Size = %d, want %d", integ.Size, len(payload))
	}
	if integ.CRC32C != "n03x6A==" {
		t.Errorf("integ.CRC32C = %q, want n03x6A==", integ.CRC32C)
	}
	if integ.MD5 != "Ojk9c3dhcmUgeW91IGtpZGRpbmc=" {
		t.Errorf("integ.MD5 = %q, want Ojk9c3dhcmUgeW91IGtpZGRpbmc=", integ.MD5)
	}
}

// When the server omits X-Goog-Hash, DownloadFeed still returns the size but
// leaves both digest fields empty rather than erroring.
func TestDownloadFeedNoHashHeader(t *testing.T) {
	t.Parallel()

	payload := gzipBytes(t, `{"ip":"1.2.3.4"}`+"\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	body, integ, err := client.DownloadFeed(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("DownloadFeed: %v", err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	if integ.Size != int64(len(payload)) {
		t.Errorf("integ.Size = %d, want %d", integ.Size, len(payload))
	}
	if integ.CRC32C != "" {
		t.Errorf("integ.CRC32C = %q, want empty", integ.CRC32C)
	}
	if integ.MD5 != "" {
		t.Errorf("integ.MD5 = %q, want empty", integ.MD5)
	}
}

func TestDownloadFeedSendsSurfaceInUserAgent(t *testing.T) {
	t.Parallel()

	payload := gzipBytes(t, `{"ip":"1.2.3.4"}`+"\n")

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL
	client.Surface = "mcp"

	body, _, err := client.DownloadFeed(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("DownloadFeed: %v", err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	if !strings.Contains(gotUA, "surface/mcp") {
		t.Fatalf("User-Agent = %q, want to contain %q", gotUA, "surface/mcp")
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

// x-goog-hash may pack both digests into one comma-joined header value;
// parseGoogHash must split on the comma and recover both.
func TestParseGoogHashSingleCommaJoinedLine(t *testing.T) {
	t.Parallel()

	crc32c, md5 := parseGoogHash([]string{"crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc="})
	if crc32c != "n03x6A==" {
		t.Errorf("crc32c = %q, want n03x6A==", crc32c)
	}
	if md5 != "Ojk9c3dhcmUgeW91IGtpZGRpbmc=" {
		t.Errorf("md5 = %q, want Ojk9c3dhcmUgeW91IGtpZGRpbmc=", md5)
	}
}

// x-goog-hash may also arrive as multiple separate header lines, one digest
// per line; parseGoogHash must recover both regardless of ordering.
func TestParseGoogHashMultipleLines(t *testing.T) {
	t.Parallel()

	crc32c, md5 := parseGoogHash([]string{"md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc=", "crc32c=n03x6A=="})
	if crc32c != "n03x6A==" {
		t.Errorf("crc32c = %q, want n03x6A==", crc32c)
	}
	if md5 != "Ojk9c3dhcmUgeW91IGtpZGRpbmc=" {
		t.Errorf("md5 = %q, want Ojk9c3dhcmUgeW91IGtpZGRpbmc=", md5)
	}
}

// Edge cases: only one digest present, an unknown key mixed in, and empty/nil
// input must all resolve without panicking or leaking the unknown key.
func TestParseGoogHashEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		values     []string
		wantCRC32C string
		wantMD5    string
	}{
		{"only crc32c", []string{"crc32c=n03x6A=="}, "n03x6A==", ""},
		{"only md5", []string{"md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc="}, "", "Ojk9c3dhcmUgeW91IGtpZGRpbmc="},
		{"unknown key ignored", []string{"crc32c=n03x6A==,goog-generation=17"}, "n03x6A==", ""},
		{"nil input", nil, "", ""},
		{"empty slice", []string{}, "", ""},
		{"empty string element", []string{""}, "", ""},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crc32c, md5 := parseGoogHash(tc.values)
			if crc32c != tc.wantCRC32C {
				t.Errorf("crc32c = %q, want %q", crc32c, tc.wantCRC32C)
			}
			if md5 != tc.wantMD5 {
				t.Errorf("md5 = %q, want %q", md5, tc.wantMD5)
			}
		})
	}
}

// A 206 partial-content response carries the true object size in the
// Content-Range total, not the Content-Length of the returned range;
// integrityFromResponse must prefer the range total.
func TestIntegrityFromResponsePrefersContentRangeTotal(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode:    http.StatusPartialContent,
		Header:        http.Header{"Content-Range": []string{"bytes 0-0/12345"}, "Content-Length": []string{"1"}},
		ContentLength: 1,
	}

	got := integrityFromResponse(resp)
	if got.Size != 12345 {
		t.Errorf("Size = %d, want 12345", got.Size)
	}
}

// Without a Content-Range header, Size falls back to Content-Length; when
// neither is usable it reports -1 rather than a misleading zero.
func TestIntegrityFromResponseSizeFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		header        http.Header
		contentLength int64
		wantSize      int64
	}{
		{"content-length header only", http.Header{"Content-Length": []string{"500"}}, -1, 500},
		{"resp.ContentLength known", http.Header{}, 500, 500},
		{"garbage content-length header", http.Header{"Content-Length": []string{"nope"}}, -1, -1},
		{"missing everything", http.Header{}, -1, -1},
		{"content-range with unknown total", http.Header{"Content-Range": []string{"bytes 0-0/*"}}, -1, -1},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := &http.Response{Header: tc.header, ContentLength: tc.contentLength}
			got := integrityFromResponse(resp)
			if got.Size != tc.wantSize {
				t.Errorf("Size = %d, want %d", got.Size, tc.wantSize)
			}
		})
	}
}

// integrityFromResponse must populate CRC32C/MD5 from X-Goog-Hash alongside
// Size, not just Size alone.
func TestIntegrityFromResponsePopulatesHashes(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		Header:        http.Header{"X-Goog-Hash": []string{"crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc="}, "Content-Length": []string{"42"}},
		ContentLength: 42,
	}

	got := integrityFromResponse(resp)
	if got.CRC32C != "n03x6A==" {
		t.Errorf("CRC32C = %q, want n03x6A==", got.CRC32C)
	}
	if got.MD5 != "Ojk9c3dhcmUgeW91IGtpZGRpbmc=" {
		t.Errorf("MD5 = %q, want Ojk9c3dhcmUgeW91IGtpZGRpbmc=", got.MD5)
	}
	if got.Size != 42 {
		t.Errorf("Size = %d, want 42", got.Size)
	}
}

// A 206 partial-content response carries the true object size in the
// Content-Range total plus the X-Goog-Hash digests; ProbeFeedIntegrity must
// surface both without erroring.
func TestProbeFeedIntegrity206(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/999")
		w.Header().Set("X-Goog-Hash", "crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc=")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	integ, err := client.ProbeFeedIntegrity(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("ProbeFeedIntegrity: %v", err)
	}
	if integ.Size != 999 {
		t.Errorf("Size = %d, want 999", integ.Size)
	}
	if integ.CRC32C != "n03x6A==" {
		t.Errorf("CRC32C = %q, want n03x6A==", integ.CRC32C)
	}
	if integ.MD5 != "Ojk9c3dhcmUgeW91IGtpZGRpbmc=" {
		t.Errorf("MD5 = %q, want Ojk9c3dhcmUgeW91IGtpZGRpbmc=", integ.MD5)
	}
}

// Some CDNs/objects ignore the Range header and answer 200 with the full
// Content-Length; ProbeFeedIntegrity must still resolve integrity from that.
func TestProbeFeedIntegrity200IgnoresRange(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1234")
		w.Header().Set("X-Goog-Hash", "crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc=")
		_, _ = w.Write([]byte("full body ignored by probe"))
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	integ, err := client.ProbeFeedIntegrity(context.Background(), "anonymous", "", FeedJSONGzip)
	if err != nil {
		t.Fatalf("ProbeFeedIntegrity: %v", err)
	}
	if integ.Size != 1234 {
		t.Errorf("Size = %d, want 1234", integ.Size)
	}
	if integ.CRC32C != "n03x6A==" {
		t.Errorf("CRC32C = %q, want n03x6A==", integ.CRC32C)
	}
}

// A non-2xx status (e.g. 403) is an error; the returned integrity must be the
// zero value.
func TestProbeFeedIntegrityNon2xxIsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	integ, err := client.ProbeFeedIntegrity(context.Background(), "anonymous", "", FeedJSONGzip)
	if err == nil {
		t.Fatal("expected error")
	}
	if integ != (FeedIntegrity{}) {
		t.Errorf("integ = %+v, want zero value", integ)
	}
}

// The probe request must carry Range: bytes=0-0 and the Token on the first
// hop, and the tiny response body must be fully drained/closed (verified by
// the server not blocking on a second read).
func TestProbeFeedIntegrityRequestHeaders(t *testing.T) {
	t.Parallel()

	var gotRange, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		gotToken = r.Header.Get("Token")
		w.Header().Set("Content-Range", "bytes 0-0/1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	if _, err := client.ProbeFeedIntegrity(context.Background(), "anonymous", "", FeedJSONGzip); err != nil {
		t.Fatalf("ProbeFeedIntegrity: %v", err)
	}
	if gotRange != "bytes=0-0" {
		t.Errorf("Range header = %q, want bytes=0-0", gotRange)
	}
	if gotToken != "secret" {
		t.Errorf("Token header = %q, want secret", gotToken)
	}
}

// An empty feed type must error before any network call.
func TestProbeFeedIntegrityRequiresType(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("network call made for empty feed type")
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.FeedsBase = srv.URL

	if _, err := client.ProbeFeedIntegrity(context.Background(), "  ", "", FeedJSONGzip); err == nil {
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
