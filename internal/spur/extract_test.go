package spur

import (
	"reflect"
	"testing"
)

// TestSplitCandidates covers the clean-input path ported from spurctx-cli's
// processIPFlag (comma-separated) and processFileFlag (newline list): tokens are
// split on whitespace, newlines, and commas with no regex extraction.
func TestSplitCandidates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "comma separated", in: "192.168.1.1,192.168.1.2", want: []string{"192.168.1.1", "192.168.1.2"}},
		{name: "newline list", in: "192.168.1.1\n192.168.1.2\n", want: []string{"192.168.1.1", "192.168.1.2"}},
		{name: "spaces", in: "192.168.1.1 192.168.1.2", want: []string{"192.168.1.1", "192.168.1.2"}},
		{name: "comma with spaces", in: "192.168.1.1, 192.168.1.2", want: []string{"192.168.1.1", "192.168.1.2"}},
		{name: "empty", in: "   \n ", want: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitCandidates(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SplitCandidates(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestExtractIPs covers the garbage path ported from spurctx-cli's
// processGarbageFileFlag / -g stdin mode: IPv4 addresses are regex-pulled out of
// freeform text.
func TestExtractIPs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "garbage text",
			in:   "There is a bunch of junk in this file.\nThere might be some ips like, 192.168.1.1 or\n192.168.1.2, but maybe not\n",
			want: []string{"192.168.1.1", "192.168.1.2"},
		},
		{name: "inline sentence", in: "the ip is 8.8.8.8 today", want: []string{"8.8.8.8"}},
		{name: "none", in: "no addresses here", want: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractIPs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ExtractIPs(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
