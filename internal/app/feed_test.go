package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

// googHash computes the base64 crc32c/md5 digests X-Goog-Hash would carry for
// raw, and formats them as a single X-Goog-Hash header value the way GCS does
// (comma-joined).
func googHash(raw []byte) string {
	c := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = c.Write(raw)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], c.Sum32())
	crc32c := base64.StdEncoding.EncodeToString(sum[:])

	m := md5.Sum(raw)
	md5b64 := base64.StdEncoding.EncodeToString(m[:])

	return "crc32c=" + crc32c + ",md5=" + md5b64
}

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

// attachFeedDigests populates JSON.CRC32C/MD5 from a successful probe of the
// JSON artifact (AC: "populate them via a best-effort ProbeFeedIntegrity per
// artifact").
func TestAttachFeedDigestsPopulatesJSON(t *testing.T) {
	t.Parallel()

	raw := []byte("feed bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/latest.json.gz") {
			t.Errorf("unexpected probe path %q", r.URL.Path)
		}
		w.Header().Set("X-Goog-Hash", googHash(raw))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(raw)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(raw[:1])
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	md := &spur.FeedMetadata{JSON: spur.FeedFile{Date: "20250324"}}
	attachFeedDigests(context.Background(), client, "anonymous", md)

	if md.JSON.CRC32C == "" || md.JSON.MD5 == "" {
		t.Errorf("JSON digests not populated: %+v", md.JSON)
	}
}

// attachFeedDigests also probes the MMDB artifact when the metadata reports
// one, populating its digests independently of the JSON artifact's.
func TestAttachFeedDigestsPopulatesMMDB(t *testing.T) {
	t.Parallel()

	jsonRaw := []byte("json bytes")
	mmdbRaw := []byte("mmdb bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := jsonRaw
		if strings.HasSuffix(r.URL.Path, "/latest.mmdb") {
			raw = mmdbRaw
		}
		w.Header().Set("X-Goog-Hash", googHash(raw))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(raw)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(raw[:1])
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	md := &spur.FeedMetadata{
		JSON: spur.FeedFile{Date: "20250324"},
		MMDB: &spur.FeedFile{Date: "20250324"},
	}
	attachFeedDigests(context.Background(), client, "ipgeo", md)

	if md.JSON.CRC32C == "" || md.JSON.MD5 == "" {
		t.Errorf("JSON digests not populated: %+v", md.JSON)
	}
	if md.MMDB.CRC32C == "" || md.MMDB.MD5 == "" {
		t.Errorf("MMDB digests not populated: %+v", md.MMDB)
	}
	if md.JSON.CRC32C == md.MMDB.CRC32C {
		t.Errorf("JSON and MMDB digests should differ (different bytes), both = %q", md.JSON.CRC32C)
	}
}

// A probe failure is best-effort: it leaves the artifact's digests empty and
// never fails the caller (AC: "if the probe fails, render metadata as before
// with empty digests").
func TestAttachFeedDigestsProbeFailureLeavesEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	md := &spur.FeedMetadata{JSON: spur.FeedFile{Date: "20250324"}}
	attachFeedDigests(context.Background(), client, "anonymous", md)

	if md.JSON.CRC32C != "" || md.JSON.MD5 != "" {
		t.Errorf("digests should stay empty on probe failure, got %+v", md.JSON)
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
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), true, nil, false, nil); err != nil {
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
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, false, nil); err != nil {
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
	err := downloadFeed(context.Background(), client, "dch", "", spur.FeedJSONGzip, bufSink(&dst), true, nil, false, nil)
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
	err := downloadFeed(context.Background(), client, "dch", "", spur.FeedMMDB, bufSink(&dst), false, nil, false, nil)
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
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedMMDB, bufSink(&dst), true, nil, false, nil); err != nil {
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

// A successful download always reports the CDN's size and both digests to
// the summary writer, with no --verify involved.
func TestDownloadFeedSummaryReportsIntegrity(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Hash", googHash(raw))
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst, summary bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, false, &summary); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	got := summary.String()
	if !strings.Contains(got, fmt.Sprintf("size=%d bytes", len(raw))) {
		t.Errorf("summary %q should report the size", got)
	}
	if !strings.Contains(got, "crc32c=") {
		t.Errorf("summary %q should report crc32c", got)
	}
	if !strings.Contains(got, "md5=") {
		t.Errorf("summary %q should report md5", got)
	}
	if strings.Contains(got, "verified") {
		t.Errorf("summary %q should not claim verification when --verify was not used", got)
	}
}

// A nil summary writer (the --silent path) means nothing is written, and the
// download still succeeds and produces the expected bytes.
func TestDownloadFeedNilSummarySuppressed(t *testing.T) {
	t.Parallel()

	ndjson := `{"ip":"1.2.3.4"}` + "\n"
	raw := gzipString(t, ndjson)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), true, nil, false, nil); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if dst.String() != ndjson {
		t.Errorf("output = %q, want %q", dst.String(), ndjson)
	}
}

// --verify with matching digests succeeds and the summary notes verification.
func TestDownloadFeedVerifyMatchSucceeds(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Hash", googHash(raw))
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst, summary bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, true, &summary); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if !strings.Contains(summary.String(), "verified") {
		t.Errorf("summary %q should note the digests were verified", summary.String())
	}
}

// --verify against a served body whose reported crc32c does not match the raw
// bytes fails clearly, naming crc32c.
func TestDownloadFeedVerifyMismatchCRC32C(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	good := googHash(raw)
	// Corrupt only the crc32c half of the header, keep a valid-looking md5.
	bad := strings.Replace(good, "crc32c=", "crc32c=AAAAAA==,ignored=", 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Hash", bad)
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, true, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "crc32c") {
		t.Errorf("error %q should name crc32c", err.Error())
	}
}

// --verify against a served body whose reported md5 does not match the raw
// bytes fails clearly, naming md5.
func TestDownloadFeedVerifyMismatchMD5(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Hash", "md5=AAAAAAAAAAAAAAAAAAAAAA==")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, true, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "md5") {
		t.Errorf("error %q should name md5", err.Error())
	}
}

// --verify hashes the RAW, pre-decompress bytes: with expand=true (the stdout
// decompress path) the digest served must still match the COMPRESSED bytes,
// proving the hash covers the gzip stream, not the expanded JSON.
func TestDownloadFeedVerifyHashesRawGzipBytes(t *testing.T) {
	t.Parallel()

	const ndjson = `{"ip":"1.2.3.4"}` + "\n"
	raw := gzipString(t, ndjson)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Hash", googHash(raw))
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), true, nil, true, nil); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if dst.String() != ndjson {
		t.Errorf("decompressed output = %q, want %q", dst.String(), ndjson)
	}
}

// --verify with no X-Goog-Hash at all from the server is a clear error; the
// same response without --verify succeeds and reports the size.
func TestDownloadFeedVerifyNoChecksumIsError(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst bytes.Buffer
	err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, true, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error %q should say no checksum was available", err.Error())
	}
}

func TestDownloadFeedNoChecksumWithoutVerifySucceeds(t *testing.T) {
	t.Parallel()

	raw := gzipString(t, `{"ip":"1.2.3.4"}`+"\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	var dst, summary bytes.Buffer
	if err := downloadFeed(context.Background(), client, "anonymous", "", spur.FeedJSONGzip, bufSink(&dst), false, nil, false, &summary); err != nil {
		t.Fatalf("downloadFeed: %v", err)
	}
	if !strings.Contains(summary.String(), fmt.Sprintf("size=%d bytes", len(raw))) {
		t.Errorf("summary %q should report the size", summary.String())
	}
}
