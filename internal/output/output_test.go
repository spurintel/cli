package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

// sampleContext is a representative IPContext exercising scalar, slice, and
// nested fields so every renderer path has something to show.
func sampleContext() spur.IPContext {
	return spur.IPContext{
		IP:             "8.8.8.8",
		Infrastructure: "DATACENTER",
		Organization:   "GOOGLE LLC",
		AS:             spur.ASInfo{Number: 15169, Organization: "GOOGLE"},
		Location:       spur.Location{City: "Mountain View", State: "California", Country: "US"},
		Client: spur.ClientInfo{
			Count:     42,
			Countries: 3,
			Types:     []string{"DESKTOP", "MOBILE"},
			Behaviors: []string{"WEB_SCRAPING"},
			Proxies:   []string{"OXYLABS_PROXY"},
		},
		Risks:    []string{"TUNNEL", "WEB_SCRAPING"},
		Services: []string{"OPENVPN", "WIREGUARD"},
		Tunnels: []spur.TunnelEntry{
			{Operator: "PROTON_VPN", Type: "VPN", Anonymous: true},
		},
	}
}

const esc = "\x1b"

func TestDefaultFormat(t *testing.T) {
	t.Parallel()
	if got := DefaultFormat(true); got != FormatText {
		t.Errorf("DefaultFormat(isTTY=true) = %q, want %q", got, FormatText)
	}
	if got := DefaultFormat(false); got != FormatJSON {
		t.Errorf("DefaultFormat(isTTY=false) = %q, want %q", got, FormatJSON)
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"text", "json", "csv"} {
		if _, err := ParseFormat(f); err != nil {
			t.Errorf("ParseFormat(%q) unexpected error: %v", f, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(\"yaml\") = nil error, want error")
	}
}

func TestRenderJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, false, sampleContext()); err != nil {
		t.Fatalf("Render json: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("json output contains ANSI escape codes: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("json output not newline-terminated: %q", out)
	}
	// Newline-delimited: a single object is exactly one line.
	if lines := strings.Count(strings.TrimRight(out, "\n"), "\n"); lines != 0 {
		t.Errorf("json output should be a single line, got %d newlines: %q", lines, out)
	}
	var got spur.IPContext
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json output is not valid JSON: %v\n%s", err, out)
	}
	if got.IP != "8.8.8.8" {
		t.Errorf("decoded IP = %q, want 8.8.8.8", got.IP)
	}
}

func TestRenderCSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatCSV, false, sampleContext()); err != nil {
		t.Fatalf("Render csv: %v", err)
	}
	if strings.Contains(buf.String(), esc) {
		t.Errorf("csv output contains ANSI escape codes: %q", buf.String())
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("csv output does not parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("csv records = %d, want 2 (header + row)", len(records))
	}
	header, row := records[0], records[1]
	if !slices.Contains(header, "ip") {
		t.Errorf("csv header missing 'ip' column: %v", header)
	}
	if !slices.Contains(row, "8.8.8.8") {
		t.Errorf("csv row missing looked-up IP: %v", row)
	}
	if len(header) != len(row) {
		t.Errorf("csv header has %d columns but row has %d", len(header), len(row))
	}
}

func TestRenderTextNoColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, sampleContext()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("--no-color text output contains ANSI escape codes: %q", out)
	}
	for _, want := range []string{"8.8.8.8", "ASN", "15169", "GOOGLE", "Mountain View", "TUNNEL"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q\n%s", want, out)
		}
	}
}

func TestRenderTextColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, true, sampleContext()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	if !strings.Contains(buf.String(), esc) {
		t.Errorf("colored text output has no ANSI escape codes: %q", buf.String())
	}
}

// batchContexts is two distinct IPContexts for exercising slice rendering.
func batchContexts() []spur.IPContext {
	a := sampleContext()
	b := sampleContext()
	b.IP = "1.1.1.1"
	b.Organization = "CLOUDFLARE"
	b.AS = spur.ASInfo{Number: 13335, Organization: "CLOUDFLARENET"}
	return []spur.IPContext{a, b}
}

func TestRenderJSONBatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, false, batchContexts()); err != nil {
		t.Fatalf("Render json batch: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("batch json = %d lines, want 2 (NDJSON, one object per line): %q", len(lines), buf.String())
	}
	var ips []string
	for _, ln := range lines {
		var c spur.IPContext
		if err := json.Unmarshal([]byte(ln), &c); err != nil {
			t.Fatalf("line is not a bare JSON object: %v\n%s", err, ln)
		}
		ips = append(ips, c.IP)
	}
	if !slices.Equal(ips, []string{"8.8.8.8", "1.1.1.1"}) {
		t.Errorf("batch json IPs = %v, want [8.8.8.8 1.1.1.1] in order", ips)
	}
}

func TestRenderCSVBatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatCSV, false, batchContexts()); err != nil {
		t.Fatalf("Render csv batch: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("csv batch does not parse: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("csv batch records = %d, want 3 (one header + two rows)", len(records))
	}
	if !slices.Contains(records[1], "8.8.8.8") || !slices.Contains(records[2], "1.1.1.1") {
		t.Errorf("csv batch rows out of order or missing IPs: %v", records)
	}
}

func TestRenderTextBatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, batchContexts()); err != nil {
		t.Fatalf("Render text batch: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"8.8.8.8", "1.1.1.1", "CLOUDFLARE"} {
		if !strings.Contains(out, want) {
			t.Errorf("batch text missing %q\n%s", want, out)
		}
	}
	// Two IP reports means two ◆ headers.
	if n := strings.Count(out, "◆"); n != 2 {
		t.Errorf("batch text has %d report headers, want 2\n%s", n, out)
	}
}

// sampleStatus is a representative active-token status.
func sampleStatus() spur.Status {
	return spur.Status{Active: true, QueriesRemaining: 12345, ServiceTier: "online"}
}

func TestRenderStatusJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, false, sampleStatus()); err != nil {
		t.Fatalf("Render json: %v", err)
	}
	var got spur.Status
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("status json is not valid JSON: %v\n%s", err, buf.String())
	}
	if got != sampleStatus() {
		t.Errorf("decoded status = %+v, want %+v", got, sampleStatus())
	}
}

func TestRenderStatusCSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatCSV, false, sampleStatus()); err != nil {
		t.Fatalf("Render csv: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("status csv does not parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("status csv records = %d, want 2 (header + row)", len(records))
	}
	if !slices.Equal(records[0], []string{"active", "queriesRemaining", "serviceTier"}) {
		t.Errorf("status csv header = %v", records[0])
	}
	if !slices.Equal(records[1], []string{"true", "12345", "online"}) {
		t.Errorf("status csv row = %v, want [true 12345 online]", records[1])
	}
}

func TestRenderStatusTextNoColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, sampleStatus()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("--no-color status text contains ANSI escape codes: %q", out)
	}
	for _, want := range []string{"Token Status", "Active", "active", "Queries Remaining", "12345", "Service Tier", "online"} {
		if !strings.Contains(out, want) {
			t.Errorf("status text missing %q\n%s", want, out)
		}
	}
}

func TestRenderStatusTextInactive(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	st := spur.Status{Active: false, QueriesRemaining: 0, ServiceTier: "online"}
	if err := Render(&buf, FormatText, false, st); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "inactive") {
		t.Errorf("inactive status text missing 'inactive' marker\n%s", out)
	}
}

func TestRenderStatusTextColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, true, sampleStatus()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	if !strings.Contains(buf.String(), esc) {
		t.Errorf("colored status text has no ANSI escape codes: %q", buf.String())
	}
}

// sampleTag is a representative service tag exercising scalar, list, optional
// boolean, and nested-metrics fields so every renderer path has something to
// show. IsNoLog is left nil to exercise the absent-flag path.
func sampleTag() spur.TagMetadata {
	yes := spur.FlexBool(true)
	return spur.TagMetadata{
		Tag:         "OXYLABS_PROXY",
		Name:        "Oxylabs Proxy",
		Description: "Residential proxy network",
		Website:     "https://oxylabs.io",
		Categories:  []string{"RESIDENTIAL_PROXY", "ISP_PROXY"},
		IsAnonymous: &yes,
		Platforms:   []string{"WINDOWS", "MACOS"},
		Protocols:   []string{"HTTP", "SOCKS"},
		Metrics: &spur.TagMetrics{
			DistinctIPs:       "123456",
			DistinctASNs:      "789",
			DistinctCountries: "42",
		},
	}
}

func TestRenderTagJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, false, sampleTag()); err != nil {
		t.Fatalf("Render json: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("tag json contains ANSI escape codes: %q", out)
	}
	var got spur.TagMetadata
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("tag json is not valid JSON: %v\n%s", err, out)
	}
	if got.Tag != "OXYLABS_PROXY" {
		t.Errorf("decoded tag = %q, want OXYLABS_PROXY", got.Tag)
	}
	// FlexNumber metrics must round-trip as bare numbers, not quoted strings.
	if !strings.Contains(out, `"distinctIPs":123456`) {
		t.Errorf("tag json metrics not emitted as bare number: %s", out)
	}
}

func TestRenderTagCSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatCSV, false, sampleTag()); err != nil {
		t.Fatalf("Render csv: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("tag csv does not parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("tag csv records = %d, want 2 (header + row)", len(records))
	}
	header, row := records[0], records[1]
	if len(header) != len(row) {
		t.Errorf("tag csv header has %d columns but row has %d", len(header), len(row))
	}
	if !slices.Contains(row, "OXYLABS_PROXY") {
		t.Errorf("tag csv row missing tag: %v", row)
	}
	if !slices.Contains(row, "RESIDENTIAL_PROXY|ISP_PROXY") {
		t.Errorf("tag csv row missing pipe-joined categories: %v", row)
	}
	if !slices.Contains(row, "123456") {
		t.Errorf("tag csv row missing distinctIPs metric: %v", row)
	}
}

func TestRenderTagTextNoColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, sampleTag()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("--no-color tag text contains ANSI escape codes: %q", out)
	}
	for _, want := range []string{"OXYLABS_PROXY", "Oxylabs Proxy", "RESIDENTIAL_PROXY", "Anonymous", "WINDOWS", "HTTP", "Metrics", "123456"} {
		if !strings.Contains(out, want) {
			t.Errorf("tag text missing %q\n%s", want, out)
		}
	}
	// An absent optional flag (IsNoLog) collapses to the muted placeholder
	// rather than reading as a definite "false".
	if strings.Contains(out, "No-log     false") {
		t.Errorf("absent No-log flag rendered as false instead of placeholder\n%s", out)
	}
}

func TestRenderTagTextColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, true, sampleTag()); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	if !strings.Contains(buf.String(), esc) {
		t.Errorf("colored tag text has no ANSI escape codes: %q", buf.String())
	}
}

// A tag whose metrics the API omitted must still render (no Metrics block, no
// panic) so a sparsely-described tag doesn't break the report.
func TestRenderTagTextNoMetrics(t *testing.T) {
	t.Parallel()
	tag := sampleTag()
	tag.Metrics = nil
	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, tag); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Metrics") {
		t.Errorf("tag without metrics should omit the Metrics block\n%s", out)
	}
	if !strings.Contains(out, "OXYLABS_PROXY") {
		t.Errorf("tag text missing tag identity\n%s", out)
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, Format("yaml"), false, sampleContext()); err == nil {
		t.Error("Render with unknown format = nil error, want error")
	}
}
