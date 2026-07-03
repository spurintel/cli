package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/spur"
)

func TestFetchContextReturnsResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token header = %q, want secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"8.8.8.8","infrastructure":"DATACENTER","as":{"number":15169,"organization":"GOOGLE"}}`))
	}))
	defer srv.Close()

	client := spur.NewClient("secret")
	client.Base = srv.URL

	got, err := fetchContext(context.Background(), client, "8.8.8.8", spur.IPContextOptions{})
	if err != nil {
		t.Fatalf("fetchContext: %v", err)
	}
	if got.IP != "8.8.8.8" {
		t.Fatalf("fetched IP = %q, want 8.8.8.8", got.IP)
	}
	if got.AS.Number != 15169 {
		t.Fatalf("fetched ASN = %d, want 15169", got.AS.Number)
	}
}

func TestFetchContextErrorMessagesAreDistinctAndClean(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: spur.ErrUnauthorized.Error()},
		{name: "forbidden", status: http.StatusForbidden, want: spur.ErrForbidden.Error()},
		{name: "throttled", status: http.StatusTooManyRequests, want: spur.ErrThrottled.Error()},
		{name: "bad request", status: http.StatusBadRequest, want: spur.ErrBadRequest.Error()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal upstream detail that must not leak", tc.status)
			}))
			defer srv.Close()

			client := spur.NewClient("token")
			client.Base = srv.URL

			_, err := fetchContext(context.Background(), client, "8.8.8.8", spur.IPContextOptions{})
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "internal upstream detail") {
				t.Fatalf("error leaked upstream body: %q", err.Error())
			}
		})
	}
}

func TestFetchContextRejectsInvalidIP(t *testing.T) {
	t.Parallel()

	_, err := fetchContext(context.Background(), spur.NewClient("token"), "not-an-ip", spur.IPContextOptions{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// contextEchoServer returns an httptest server that echoes the requested IP back
// as the body, so a batch lookup can be checked for completeness and order.
func contextEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := path.Base(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"ip":%q}`, ip)
	}))
}

func TestLookupContextsPreservesOrderAndSkipsFailures(t *testing.T) {
	t.Parallel()

	srv := contextEchoServer(t)
	defer srv.Close()

	client := spur.NewClient("token")
	client.Base = srv.URL

	ips := []string{"8.8.8.8", "not-an-ip", "1.1.1.1"}

	// The bounded and unbounded cases must produce identical, ordered output.
	for _, parallelism := range []int{1, 8} {
		results, failures := lookupContexts(context.Background(), client, ips, spur.IPContextOptions{}, parallelism)

		if len(results) != 2 {
			t.Fatalf("parallelism=%d: results = %d, want 2", parallelism, len(results))
		}
		if results[0].IP != "8.8.8.8" || results[1].IP != "1.1.1.1" {
			t.Errorf("parallelism=%d: results out of input order: %q, %q", parallelism, results[0].IP, results[1].IP)
		}
		if len(failures) != 1 || failures[0].ip != "not-an-ip" {
			t.Errorf("parallelism=%d: failures = %+v, want one for not-an-ip", parallelism, failures)
		}
	}
}

func TestGatherIPs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		stdin   string
		extract bool
		want    []string
	}{
		{name: "args clean", args: []string{"8.8.8.8", "1.1.1.1"}, want: []string{"8.8.8.8", "1.1.1.1"}},
		{name: "args comma", args: []string{"8.8.8.8,1.1.1.1"}, want: []string{"8.8.8.8", "1.1.1.1"}},
		{name: "stdin newline", stdin: "8.8.8.8\n1.1.1.1\n", want: []string{"8.8.8.8", "1.1.1.1"}},
		{name: "args win over stdin", args: []string{"9.9.9.9"}, stdin: "8.8.8.8\n", want: []string{"9.9.9.9"}},
		{name: "extract from stdin", stdin: "the ip is 8.8.8.8 today", extract: true, want: []string{"8.8.8.8"}},
		{name: "extract from args", args: []string{"host 8.8.8.8 up"}, extract: true, want: []string{"8.8.8.8"}},
		{name: "empty", stdin: "", want: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tc.stdin))
			got, err := gatherIPs(cmd, tc.args, tc.extract)
			if err != nil {
				t.Fatalf("gatherIPs: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("gatherIPs = %#v, want %#v", got, tc.want)
			}
		})
	}
}
