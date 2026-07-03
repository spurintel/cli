package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	got, err := fetchContext(context.Background(), client, "8.8.8.8")
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

			_, err := fetchContext(context.Background(), client, "8.8.8.8")
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

	_, err := fetchContext(context.Background(), spur.NewClient("token"), "not-an-ip")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
