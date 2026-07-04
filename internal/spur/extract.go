package spur

import (
	"regexp"
	"strings"
)

// ipv4Regex matches an IPv4-looking token: four 1-3 digit groups on word
// boundaries. It deliberately does not validate octet ranges (999.999.999.999
// matches) — this preserves the behavior of spurctx-cli's -g/-gf garbage mode,
// which this helper ports. ValidateIP is the real gate on any candidate.
var ipv4Regex = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// ExtractIPs pulls IPv4 addresses out of freeform ("garbage") text via a regex,
// preserving the behavior of the old tool's -g/-gf flags. Callers still pass
// each result through ValidateIP before use. Returns nil when nothing matches.
func ExtractIPs(text string) []string {
	return ipv4Regex.FindAllString(text, -1)
}

// SplitCandidates splits clean input into candidate IP tokens on any run of
// whitespace, newlines, or commas. It performs no extraction or validation, so
// IPv6 addresses survive intact for ValidateIP. Returns nil when the input is
// empty or only separators.
func SplitCandidates(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
	})
	if len(fields) == 0 {
		return nil
	}
	return fields
}
