package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func TestRunContextPrintsJSON(t *testing.T) {
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

	var buf bytes.Buffer
	if err := runContext(context.Background(), client, &buf, "8.8.8.8"); err != nil {
		t.Fatalf("runContext: %v", err)
	}

	// Output must be valid JSON echoing the looked-up address.
	var got spur.IPContext
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if got.IP != "8.8.8.8" {
		t.Fatalf("decoded IP = %q, want 8.8.8.8", got.IP)
	}
}

func TestRunContextErrorMessagesAreDistinctAndClean(t *testing.T) {
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

			var buf bytes.Buffer
			err := runContext(context.Background(), client, &buf, "8.8.8.8")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "internal upstream detail") {
				t.Fatalf("error leaked upstream body: %q", err.Error())
			}
			if buf.Len() != 0 {
				t.Fatalf("expected no stdout on error, got %q", buf.String())
			}
		})
	}
}

func TestRunContextRejectsInvalidIP(t *testing.T) {
	t.Parallel()

	// A nil client is safe here: validation must fail before any HTTP call.
	var buf bytes.Buffer
	err := runContext(context.Background(), spur.NewClient("token"), &buf, "not-an-ip")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output for invalid IP, got %q", buf.String())
	}
}
