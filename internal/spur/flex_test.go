package spur

import (
	"encoding/json"
	"testing"
)

func TestFlexBoolUnmarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    bool
		wantErr bool
	}{
		{name: "bare true", in: `true`, want: true},
		{name: "bare false", in: `false`, want: false},
		{name: "quoted true", in: `"true"`, want: true},
		{name: "quoted false", in: `"false"`, want: false},
		{name: "quoted one", in: `"1"`, want: true},
		{name: "quoted zero", in: `"0"`, want: false},
		{name: "quoted yes", in: `"yes"`, want: true},
		{name: "quoted no", in: `"no"`, want: false},
		{name: "quoted mixed case", in: `"Yes"`, want: true},
		{name: "null leaves zero value", in: `null`, want: false},
		{name: "quoted empty is false", in: `""`, want: false},
		{name: "garbage is an error", in: `"maybe"`, wantErr: true},
		{name: "number is an error", in: `2`, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b FlexBool
			err := json.Unmarshal([]byte(tc.in), &b)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if bool(b) != tc.want {
				t.Fatalf("FlexBool(%q) = %v, want %v", tc.in, bool(b), tc.want)
			}
		})
	}
}

func TestFlexBoolMarshalAlwaysBare(t *testing.T) {
	t.Parallel()

	got, err := json.Marshal(FlexBool(true))
	if err != nil {
		t.Fatalf("marshal true: %v", err)
	}
	if string(got) != "true" {
		t.Fatalf("marshal FlexBool(true) = %s, want true", got)
	}

	got, err = json.Marshal(FlexBool(false))
	if err != nil {
		t.Fatalf("marshal false: %v", err)
	}
	if string(got) != "false" {
		t.Fatalf("marshal FlexBool(false) = %s, want false", got)
	}
}

func TestFlexNumberUnmarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare integer", in: `42`, want: "42"},
		{name: "bare float", in: `3.14159`, want: "3.14159"},
		{name: "quoted integer", in: `"42"`, want: "42"},
		{name: "quoted float", in: `"3.14159"`, want: "3.14159"},
		{name: "quoted with whitespace", in: `" 7 "`, want: "7"},
		{name: "null is empty", in: `null`, want: ""},
		{name: "quoted empty is empty", in: `""`, want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var n FlexNumber
			if err := json.Unmarshal([]byte(tc.in), &n); err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if n.String() != tc.want {
				t.Fatalf("FlexNumber(%q) = %q, want %q", tc.in, n.String(), tc.want)
			}
		})
	}
}

func TestFlexNumberMarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   FlexNumber
		want string
	}{
		{name: "integer stays integer", in: FlexNumber("42"), want: "42"},
		{name: "float keeps precision", in: FlexNumber("3.14159"), want: "3.14159"},
		{name: "empty encodes as zero", in: FlexNumber(""), want: "0"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshal %q: %v", tc.in, err)
			}
			if string(got) != tc.want {
				t.Fatalf("marshal FlexNumber(%q) = %s, want %s", string(tc.in), got, tc.want)
			}
		})
	}
}

// TestTagMetadataTolerantDecode is the end-to-end tolerant-decoding check: the
// upstream docs type these boolean/metric fields as strings while the live API
// returns bare booleans and numbers. Decoding a mixed payload must succeed and
// normalize to the underlying Go types regardless of which shape arrived.
func TestTagMetadataTolerantDecode(t *testing.T) {
	t.Parallel()

	// isAnonymous arrives as a bare bool, isNoLog as a quoted string; the
	// metrics mix bare numbers and quoted-number strings.
	payload := `{
		"tag": "OXYLABS_PROXY",
		"name": "Oxylabs",
		"isAnonymous": true,
		"isNoLog": "false",
		"metrics": {
			"distinctIPs": 12345,
			"churnRate": "0.42"
		}
	}`

	var tm TagMetadata
	if err := json.Unmarshal([]byte(payload), &tm); err != nil {
		t.Fatalf("unmarshal mixed-shape payload: %v", err)
	}

	if tm.IsAnonymous == nil || !bool(*tm.IsAnonymous) {
		t.Fatalf("IsAnonymous = %v, want true", tm.IsAnonymous)
	}
	if tm.IsNoLog == nil || bool(*tm.IsNoLog) {
		t.Fatalf("IsNoLog = %v, want false", tm.IsNoLog)
	}
	if tm.Metrics == nil {
		t.Fatal("Metrics is nil")
	}
	if got := tm.Metrics.DistinctIPs.String(); got != "12345" {
		t.Fatalf("DistinctIPs = %q, want 12345", got)
	}
	if got := tm.Metrics.ChurnRate.String(); got != "0.42" {
		t.Fatalf("ChurnRate = %q, want 0.42", got)
	}
}
