package spur

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func mustFeed(t *testing.T, name string) ExportFeed {
	t.Helper()
	f, ok := LookupExportFeed(name)
	if !ok {
		t.Fatalf("feed %q not in catalog", name)
	}
	return f
}

// The flag-to-query mapping is the logic worth testing (per the ticket): each
// case asserts the exact query parameters a set of options produces for a feed.
func TestBuildExportQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		feed string
		opts ExportOptions
		want url.Values // exact expected query
	}{
		{
			name: "format and limit",
			feed: "anonymous",
			opts: ExportOptions{Format: "csv", Limit: 5},
			want: url.Values{"output": {"csv"}, "limit": {"5"}},
		},
		{
			name: "fields tags ccs asns",
			feed: "anonymous",
			opts: ExportOptions{
				Fields: []string{"ip", "tag", "asNumber"},
				Tags:   []string{"OXYLABS_PROXY", "LUMINATI_PROXY"},
				Ccs:    []string{"us", "gb"},
				Asns:   []int{7922, 15169},
			},
			want: url.Values{
				"fields": {"ip,tag,asNumber"},
				"tags":   {"OXYLABS_PROXY,LUMINATI_PROXY"},
				"ccs":    {"US,GB"},
				"asns":   {"7922,15169"},
			},
		},
		{
			name: "diff bare",
			feed: "anonymous",
			opts: ExportOptions{Diff: true},
			want: url.Values{"diff": {"true"}},
		},
		{
			name: "diff date implies diff, sends only date",
			feed: "anonymous",
			opts: ExportOptions{DiffDate: "2026-07-01"},
			want: url.Values{"diffDate": {"2026-07-01"}},
		},
		{
			name: "attributed false is sent",
			feed: "anonymous",
			opts: ExportOptions{Attributed: boolPtr(false)},
			want: url.Values{"attributed": {"false"}},
		},
		{
			name: "attributed true is sent",
			feed: "anonymous",
			opts: ExportOptions{Attributed: boolPtr(true)},
			want: url.Values{"attributed": {"true"}},
		},
		{
			name: "csv header",
			feed: "anonymous",
			opts: ExportOptions{Format: "csv", CSVHeader: true},
			want: url.Values{"output": {"csv"}, "includeCSVHeader": {"true"}},
		},
		{
			name: "residential callback proxies",
			feed: "anonymous-residential",
			opts: ExportOptions{CallbackProxies: []string{"HOLA", "NIMBLEWAY"}},
			want: url.Values{"callbackProxies": {"HOLA,NIMBLEWAY"}},
		},
		{
			name: "dch fields only",
			feed: "dch",
			opts: ExportOptions{Format: "mmdb", Fields: []string{"network", "organization"}},
			want: url.Values{"output": {"mmdb"}, "fields": {"network,organization"}},
		},
		{
			name: "ipgeo accepts ccs",
			feed: "ipgeo",
			opts: ExportOptions{Ccs: []string{"US"}},
			want: url.Values{"ccs": {"US"}},
		},
		{
			name: "no options is empty query",
			feed: "anonymous",
			opts: ExportOptions{},
			want: url.Values{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildExportQuery(mustFeed(t, tc.feed), tc.opts)
			if err != nil {
				t.Fatalf("BuildExportQuery: %v", err)
			}
			if got.Encode() != tc.want.Encode() {
				t.Errorf("query = %q, want %q", got.Encode(), tc.want.Encode())
			}
		})
	}
}

// Each family rejects the filters it does not accept, before any network call.
func TestBuildExportQueryRejectsUnsupportedFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		feed    string
		opts    ExportOptions
		wantSub string
	}{
		{"callback-proxies on anonymous", "anonymous", ExportOptions{CallbackProxies: []string{"HOLA"}}, "--callback-proxies"},
		{"diff on residential", "anonymous-residential", ExportOptions{Diff: true}, "--diff"},
		{"diff-date on residential", "anonymous-residential", ExportOptions{DiffDate: "2026-07-01"}, "--diff"},
		{"tags on dch", "dch", ExportOptions{Tags: []string{"x"}}, "--tags"},
		{"limit on dch", "dch", ExportOptions{Limit: 10}, "--limit"},
		{"ccs on dch", "dch", ExportOptions{Ccs: []string{"US"}}, "--ccs"},
		{"attributed on ipgeo", "ipgeo", ExportOptions{Attributed: boolPtr(false)}, "--attributed"},
		{"limit on ipgeo", "ipgeo", ExportOptions{Limit: 10}, "--limit"},
		{"tags on ipgeo", "ipgeo", ExportOptions{Tags: []string{"x"}}, "--tags"},
		{"bad format", "anonymous", ExportOptions{Format: "xml"}, "--format"},
		{"bad diff-date", "anonymous", ExportOptions{DiffDate: "07/01/2026"}, "--diff-date"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildExportQuery(mustFeed(t, tc.feed), tc.opts)
			if err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q should mention %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResolveExportFeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arg      string
		ipv6     bool
		wantSlug string
		wantErr  bool
	}{
		{name: "plain", arg: "anonymous", wantSlug: "anonymous"},
		{name: "ipv6 modifier", arg: "anonymous", ipv6: true, wantSlug: "anonymous-ipv6"},
		{name: "residential ipv6", arg: "anonymous-residential", ipv6: true, wantSlug: "anonymous-residential-ipv6"},
		{name: "ztna ipv6", arg: "ztna", ipv6: true, wantSlug: "ztna-ipv6"},
		{name: "full ipv6 slug direct", arg: "anonymous-ipv6", wantSlug: "anonymous-ipv6"},
		{name: "case insensitive", arg: "Anonymous", wantSlug: "anonymous"},
		{name: "dch has no ipv6", arg: "dch", ipv6: true, wantErr: true},
		{name: "ipgeo has no ipv6", arg: "ipgeo", ipv6: true, wantErr: true},
		{name: "unknown feed", arg: "nope", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, slug, err := ResolveExportFeed(tc.arg, tc.ipv6)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveExportFeed(%q, %v) = %q, want error", tc.arg, tc.ipv6, slug)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveExportFeed: %v", err)
			}
			if slug != tc.wantSlug {
				t.Errorf("slug = %q, want %q", slug, tc.wantSlug)
			}
		})
	}
}

// ExportFeed carries the Token header, hits /v1/feeds/<slug> with the built
// query, and streams the raw body back to the caller.
func TestExportFeedStreamsBody(t *testing.T) {
	t.Parallel()

	const csv = "1.2.3.0/24,OXYLABS_PROXY\n5.6.7.0/24,LUMINATI_PROXY\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/feeds/anonymous"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("output"); got != "csv" {
			t.Errorf("output = %q, want csv", got)
		}
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = io.WriteString(w, csv)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.ExportsBase = srv.URL

	body, _, err := client.ExportFeed(context.Background(), "anonymous", url.Values{"output": {"csv"}})
	if err != nil {
		t.Fatalf("ExportFeed: %v", err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != csv {
		t.Errorf("body = %q, want %q", got, csv)
	}
}

// A 429 from the Exports host maps to ErrThrottled so the command layer can turn
// it into an actionable rate-limit message.
func TestExportFeedThrottledMapsToErrThrottled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited upstream detail", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.ExportsBase = srv.URL

	_, _, err := client.ExportFeed(context.Background(), "anonymous", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrThrottled) {
		t.Errorf("error = %v, want ErrThrottled", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
		t.Errorf("want APIError with status 429, got %v", err)
	}
}
