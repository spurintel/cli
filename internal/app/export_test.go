package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

// streamExport requests /v1/feeds/<slug> with the supplied query and copies CSV
// rows to the destination, as used by `spur export anonymous --format csv`.
func TestStreamExportWritesCSV(t *testing.T) {
	t.Parallel()

	const csv = "1.2.3.0/24,OXYLABS_PROXY\n5.6.7.0/24,LUMINATI_PROXY\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/feeds/anonymous"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csv))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.ExportsBase = srv.URL

	q := url.Values{"output": {"csv"}, "limit": {"5"}}
	var dst bytes.Buffer
	if err := streamExport(context.Background(), client, "anonymous", "anonymous", q, bufSink(&dst)); err != nil {
		t.Fatalf("streamExport: %v", err)
	}
	if dst.String() != csv {
		t.Errorf("output = %q, want %q", dst.String(), csv)
	}
}

// With --format mmdb, the CLI copies the server's bytes unchanged. This checks
// binary preservation; validating the database itself is the server's job.
func TestStreamExportWritesMMDBBytes(t *testing.T) {
	t.Parallel()

	const mmdb = "\x00\x01MMDBbinary\xff"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("output"); got != "mmdb" {
			t.Errorf("output = %q, want mmdb", got)
		}
		w.Header().Set("Content-Type", "application/vnd.maxmind.maxmind-db")
		_, _ = w.Write([]byte(mmdb))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.ExportsBase = srv.URL

	q := url.Values{"output": {"mmdb"}}
	var dst bytes.Buffer
	if err := streamExport(context.Background(), client, "ipgeo", "ipgeo", q, bufSink(&dst)); err != nil {
		t.Fatalf("streamExport: %v", err)
	}
	if dst.String() != mmdb {
		t.Errorf("output = %q, want raw mmdb bytes %q", dst.String(), mmdb)
	}
}

// A 429 surfaces as a clear, actionable rate-limit message and never leaks the
// upstream body (AC: "a rate-limit (429) response surfaces as a clear,
// actionable message").
func TestStreamExportRateLimitIsClear(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal upstream rate-limit detail that must not leak", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.ExportsBase = srv.URL

	var dst bytes.Buffer
	err := streamExport(context.Background(), client, "anonymous", "anonymous", nil, bufSink(&dst))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "rate limit") {
		t.Errorf("error %q should mention the rate limit", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "retry") {
		t.Errorf("error %q should tell the user to retry", msg)
	}
	if strings.Contains(msg, "internal upstream") {
		t.Fatalf("error leaked upstream body: %q", msg)
	}
	if dst.Len() != 0 {
		t.Errorf("nothing should have been written, got %d bytes", dst.Len())
	}
}

// A forbidden export gets a feed-aware access message, not the generic sentinel.
func TestStreamExportForbiddenIsClear(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that must not leak", http.StatusForbidden)
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.ExportsBase = srv.URL

	var dst bytes.Buffer
	err := streamExport(context.Background(), client, "ztna", "ztna", nil, bufSink(&dst))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ztna") {
		t.Errorf("forbidden error %q should name the feed", msg)
	}
	if !strings.Contains(msg, "denied") && !strings.Contains(msg, "subscription") {
		t.Errorf("forbidden error %q should explain access is denied", msg)
	}
	if strings.Contains(msg, "internal detail") {
		t.Fatalf("error leaked upstream body: %q", msg)
	}
}

// The command wires the flags onto the query end-to-end: --callback-proxies is
// accepted for the residential feed and reflected in the request.
func TestExportCmdResidentialCallbackProxies(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("row\n"))
	}))
	defer srv.Close()

	out, errOut, err := runExportCmd(t, srv.URL,
		"export", "anonymous-residential", "--callback-proxies", "HOLA,NIMBLEWAY", "--format", "csv")
	if err != nil {
		t.Fatalf("export: %v (stderr: %s)", err, errOut)
	}
	if got := gotQuery.Get("callbackProxies"); got != "HOLA,NIMBLEWAY" {
		t.Errorf("callbackProxies = %q, want HOLA,NIMBLEWAY", got)
	}
	if out != "row\n" {
		t.Errorf("stdout = %q, want the CSV row", out)
	}
}

// --callback-proxies is rejected for a non-residential feed before any request.
func TestExportCmdCallbackProxiesRejectedForAnon(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	_, _, err := runExportCmd(t, srv.URL,
		"export", "anonymous", "--callback-proxies", "HOLA")
	if err == nil {
		t.Fatal("expected an error for --callback-proxies on anonymous")
	}
	if !strings.Contains(err.Error(), "--callback-proxies") {
		t.Errorf("error %q should name the rejected flag", err.Error())
	}
	if called {
		t.Error("no request should have been made for a rejected flag")
	}
}

// runExportCmd builds the root command, points it at the test server via a
// --config endpoint override, sets a token in the environment, and runs the
// given args, returning captured stdout and stderr.
func runExportCmd(t *testing.T, exportsURL string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("SPUR_TOKEN", "secret")
	// Isolate config discovery to an empty dir so no real ~/.config leaks in.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfgPath := writeExportConfig(t, exportsURL)

	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"--config", cfgPath}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func writeExportConfig(t *testing.T, exportsURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[endpoints]\nexports = \"" + exportsURL + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// A failed export must not destroy a pre-existing -o file: the destination is
// only opened (and truncated) after the request has succeeded.
func TestExportCmdFailureLeavesExistingFileIntact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer srv.Close()

	existing := filepath.Join(t.TempDir(), "data.csv")
	const precious = "yesterday's rows that must survive\n"
	if err := os.WriteFile(existing, []byte(precious), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, _, err := runExportCmd(t, srv.URL, "export", "anonymous", "-o", existing)
	if err == nil {
		t.Fatal("expected the 403 to surface as an error")
	}

	got, readErr := os.ReadFile(existing)
	if readErr != nil {
		t.Fatalf("read seeded file: %v", readErr)
	}
	if string(got) != precious {
		t.Fatalf("pre-existing -o file was clobbered: %q", got)
	}
}
