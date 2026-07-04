package spur

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadBodyRejectsOversizeResponses(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("a", int(maxResponseBodyBytes)+1)
	_, err := readBody(strings.NewReader(body), maxResponseBodyBytes)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestGetIPContextSendsQueryArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		opts   IPContextOptions
		wantDT string // "" means the dt arg must be absent
	}{
		{name: "none", opts: IPContextOptions{}},
		{name: "date", opts: IPContextOptions{DT: "20240101"}, wantDT: "20240101"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("dt"); got != tc.wantDT {
					t.Errorf("dt = %q, want %q", got, tc.wantDT)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ip":"8.8.8.8"}`))
			}))
			defer srv.Close()

			client := NewClient("token")
			client.Base = srv.URL

			if _, _, err := client.GetIPContext(context.Background(), "8.8.8.8", tc.opts); err != nil {
				t.Fatalf("GetIPContext: %v", err)
			}
		})
	}
}

func TestGetStatusReturnsSanitizedAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal detail that should not escape", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("token")
	client.Base = srv.URL

	_, err := client.GetStatus(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if got, want := apiErr.Message(), ErrForbidden.Error(); got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "internal detail") {
		t.Fatalf("error leaked upstream body: %q", err.Error())
	}
}

// The JSON client must never re-send the Token header across a cross-host
// redirect: Go's default policy only strips Authorization/Cookie, so a
// redirect from the API host would otherwise hand the credential to whatever
// host the Location points at.
func TestDoJSONStripsTokenOnRedirect(t *testing.T) {
	t.Parallel()

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "" {
			t.Errorf("Token forwarded across hosts = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer other.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Token"); got != "secret" {
			t.Errorf("Token on initial request = %q, want secret", got)
		}
		http.Redirect(w, r, other.URL+"/status", http.StatusFound)
	}))
	defer origin.Close()

	client := NewClient("secret")
	client.Base = origin.URL

	if _, err := client.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
}

// An oversized upstream error page must not mask the status-mapped APIError:
// the status is classified before the body is read.
func TestDoJSONOversizeErrorBodyStillMapsStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		big := bytes.Repeat([]byte("x"), int(maxResponseBodyBytes)+512)
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	client := NewClient("secret")
	client.Base = srv.URL

	_, err := client.GetStatus(context.Background())
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrServer) {
		t.Fatalf("error = %v, want ErrServer despite the oversized body", err)
	}
}
