package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

const metadataBody = `{
	"json": {
		"location": "20250324/feed.json.gz",
		"date": "20250324",
		"generated_at": "2025-03-24T00:59:41Z",
		"available_at": "2025-03-24T00:59:56Z",
		"filesize": 89553131
	}
}`

const metadataBodyWithMMDB = `{
	"json": {
		"location": "20250324/feed.json.gz",
		"date": "20250324",
		"filesize": 89553131
	},
	"mmdb": {
		"location": "20250324/feed.mmdb",
		"date": "20250324",
		"filesize": 42
	}
}`

// feedTestServer routes the metadata endpoint plus one or more artifact
// probe endpoints, each answering with the given status/headers. probes is
// keyed by URL path.
func feedTestServer(t *testing.T, metadata string, probes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/anonymous/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(metadata))
			return
		}
		if h, ok := probes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		t.Fatalf("unexpected request path %q", r.URL.Path)
	}))
}

func okProbeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Range", "bytes 0-0/999")
	w.Header().Set("X-Goog-Hash", "crc32c=n03x6A==,md5=Ojk9c3dhcmUgeW91IGtpZGRpbmc=")
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write([]byte("x"))
}

// 1. feed_status: metadata OK + artifact probe succeeds -> Integrity populated,
// Metadata unchanged.
func TestFeedStatusPopulatesIntegrityOnSuccessfulProbe(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBody, map[string]http.HandlerFunc{
		"/v2/anonymous/latest.json.gz": okProbeHandler,
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedStatus(context.Background(), client, FeedStatusInput{Type: "anonymous"})
	if err != nil {
		t.Fatalf("feedStatus: %v", err)
	}
	if res != nil {
		t.Fatalf("expected success result, got error result: %+v", res)
	}
	if out.Metadata.JSON.Date != "20250324" {
		t.Errorf("Metadata.JSON.Date = %q, want 20250324", out.Metadata.JSON.Date)
	}
	if out.Integrity == nil {
		t.Fatal("Integrity = nil, want populated")
	}
	if out.Integrity.Size != 999 {
		t.Errorf("Integrity.Size = %d, want 999", out.Integrity.Size)
	}
	if out.Integrity.CRC32C != "9f4df1e8" {
		t.Errorf("Integrity.CRC32C = %q, want 9f4df1e8", out.Integrity.CRC32C)
	}
}

// 2. feed_status: probe endpoint returns 500 -> Integrity nil, tool result
// still success (best-effort, no mcp error).
func TestFeedStatusBestEffortWhenProbeFails(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBody, map[string]http.HandlerFunc{
		"/v2/anonymous/latest.json.gz": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedStatus(context.Background(), client, FeedStatusInput{Type: "anonymous"})
	if err != nil {
		t.Fatalf("feedStatus: %v", err)
	}
	if res != nil {
		t.Fatalf("expected success result despite probe failure, got: %+v", res)
	}
	if out.Integrity != nil {
		t.Errorf("Integrity = %+v, want nil", out.Integrity)
	}
	if out.Metadata.JSON.Date != "20250324" {
		t.Errorf("Metadata.JSON.Date = %q, want 20250324", out.Metadata.JSON.Date)
	}
}

// 3. feed_download json default -> Integrity reflects the JSON artifact;
// File/DownloadURL/Command unchanged.
func TestFeedDownloadJSONPopulatesIntegrity(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBody, map[string]http.HandlerFunc{
		"/v2/anonymous/latest.json.gz": okProbeHandler,
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedDownload(context.Background(), client, FeedDownloadInput{Type: "anonymous"})
	if err != nil {
		t.Fatalf("feedDownload: %v", err)
	}
	if res != nil {
		t.Fatalf("expected success result, got: %+v", res)
	}
	if out.File == nil || out.File.Date != "20250324" {
		t.Errorf("File = %+v, want date 20250324", out.File)
	}
	if out.Command != "spur feed download anonymous --format json" {
		t.Errorf("Command = %q", out.Command)
	}
	if out.Integrity == nil || out.Integrity.Size != 999 {
		t.Errorf("Integrity = %+v, want size 999", out.Integrity)
	}
}

// 4a. feed_download --mmdb when metadata offers MMDB -> probes the mmdb
// artifact.
func TestFeedDownloadMMDBOfferedProbesMMDBArtifact(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBodyWithMMDB, map[string]http.HandlerFunc{
		"/v2/anonymous/latest.mmdb": okProbeHandler,
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedDownload(context.Background(), client, FeedDownloadInput{Type: "anonymous", Format: "mmdb"})
	if err != nil {
		t.Fatalf("feedDownload: %v", err)
	}
	if res != nil {
		t.Fatalf("expected success result, got: %+v", res)
	}
	if out.Integrity == nil || out.Integrity.Size != 999 {
		t.Errorf("Integrity = %+v, want size 999", out.Integrity)
	}
}

// 4b. feed_download --mmdb when NOT offered -> existing MMDB error still
// fires, with no probe attempted.
func TestFeedDownloadMMDBNotOfferedErrorsWithoutProbe(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBody, map[string]http.HandlerFunc{
		"/v2/anonymous/latest.mmdb": func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("probe should not be attempted when mmdb is not offered")
		},
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedDownload(context.Background(), client, FeedDownloadInput{Type: "anonymous", Format: "mmdb"})
	if err != nil {
		t.Fatalf("feedDownload: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected mcp error result, got %+v (out=%+v)", res, out)
	}
	if out.Integrity != nil {
		t.Errorf("Integrity = %+v, want nil", out.Integrity)
	}
}

// 5. feed_download historical date -> File still nil, Integrity populated
// from the dated-artifact probe.
func TestFeedDownloadHistoricalDatePopulatesIntegrity(t *testing.T) {
	t.Parallel()

	srv := feedTestServer(t, metadataBody, map[string]http.HandlerFunc{
		"/v2/anonymous/20250101/feed.json.gz": okProbeHandler,
	})
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, out, err := feedDownload(context.Background(), client, FeedDownloadInput{Type: "anonymous", Date: "20250101"})
	if err != nil {
		t.Fatalf("feedDownload: %v", err)
	}
	if res != nil {
		t.Fatalf("expected success result, got: %+v", res)
	}
	if out.File != nil {
		t.Errorf("File = %+v, want nil for historical date", out.File)
	}
	if out.Integrity == nil || out.Integrity.Size != 999 {
		t.Errorf("Integrity = %+v, want size 999", out.Integrity)
	}
}

// 6. Unknown feed type / invalid format / bad date -> unchanged existing
// errors, no probe attempted (server fails the test if hit at all).
func TestFeedDownloadValidationErrorsSkipProbe(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %q on a validation error path", r.URL.Path)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	cases := []FeedDownloadInput{
		{Type: "nope-such-feed"},
		{Type: "anonymous", Format: "bogus"},
		{Type: "anonymous", Date: "not-a-date"},
	}
	for _, in := range cases {
		res, _, err := feedDownload(context.Background(), client, in)
		if err != nil {
			t.Fatalf("feedDownload(%+v): %v", in, err)
		}
		if res == nil || !res.IsError {
			t.Errorf("feedDownload(%+v) = %+v, want mcp error result", in, res)
		}
	}
}

func TestFeedStatusUnknownTypeSkipsProbe(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %q on a validation error path", r.URL.Path)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.FeedsBase = srv.URL

	res, _, err := feedStatus(context.Background(), client, FeedStatusInput{Type: "nope-such-feed"})
	if err != nil {
		t.Fatalf("feedStatus: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected mcp error result, got %+v", res)
	}
}
