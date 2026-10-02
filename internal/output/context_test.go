package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spurintel/cli/internal/spur"
)

func TestContextReportSectionsAndDetails(t *testing.T) {
	c := sampleContext()
	c.Client.Concentration = &spur.Concentration{City: "Amsterdam", Country: "NL", Density: 0.82, Skew: 6200}
	c.AI = &spur.AIActivity{Operator: "Example AI", Types: []string{"CRAWLER"}}
	c.Tunnels[0].Entries = []string{"192.0.2.1"}
	c.Tunnels[0].Exits = []string{"192.0.2.2"}
	var b bytes.Buffer
	if err := Render(&b, FormatText, false, c); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	previous := -1
	for _, name := range []string{c.IP, "PROXY & ANONYMIZATION INTELLIGENCE", "GEO INTELLIGENCE", "DEVICE ACTIVITY"} {
		at := strings.Index(out, name)
		if at <= previous {
			t.Fatalf("missing or unordered section %q: %s", name, out)
		}
		previous = at
	}
	for _, want := range []string{"[ANONYMOUS TUNNEL]", "PROTON_VPN", "192.0.2.1", "192.0.2.2", "Amsterdam, NL", "0.82", "6200", "Example AI", "CRAWLER"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestContextReportEmptyValues(t *testing.T) {
	var b bytes.Buffer
	if err := Render(&b, FormatText, false, spur.IPContext{IP: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "[NO ANONYMOUS TUNNEL REPORTED]") {
		t.Fatal(out)
	}
	for _, want := range []string{"Risk indicators     None", "Tunnel operators    None", "Density             None", "Devices             0"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestContextReportRespectsWidth(t *testing.T) {
	c := sampleContext()
	c.IP = "2001:db8:1234:5678:abcd:abcd:abcd:abcd"
	c.AS.Organization = strings.Repeat("長い名前", 30)
	c.Infrastructure = strings.Repeat("LONG_INFRASTRUCTURE_", 8)
	c.Tunnels[0].Operator = strings.Repeat("LONG_OPERATOR_", 15)
	for _, width := range []int{24, 39, 40, 56, 80, 110} {
		var b bytes.Buffer
		if err := renderIPContext(&b, NewStyles(&b, true), width, c); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(b.String(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("width %d: line has %d columns: %q", width, got, line)
			}
		}
	}
}
