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

func TestRenderUnknownFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Render(&buf, Format("yaml"), false, sampleContext()); err == nil {
		t.Error("Render with unknown format = nil error, want error")
	}
}
