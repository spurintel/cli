package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/spurintel/cli/internal/spur"
)

// renderIPContext adapts the IP result layout from app.spur.us for a terminal:
// an overview followed by proxy, geo, and device intelligence. History and maps
// are omitted because they require data beyond the context response.
func renderIPContext(w io.Writer, s Styles, width int, c spur.IPContext) error {
	width = max(1, min(80, width))
	var b strings.Builder
	line := func(text string) { fmt.Fprintln(&b, ansi.Wrap(text, width, "")) }
	title := func(text string) { line(s.Title.Render(text)) }
	row := func(key string, value any) { writeReportRow(&b, s, width, key, value) }
	section := func(name string) { fmt.Fprintln(&b); title("■ " + name) }

	title(c.IP)
	// An empty tunnel list does not establish that an address is safe or that
	// all of its traffic is non-anonymous.
	anonymous := false
	for _, t := range c.Tunnels {
		anonymous = anonymous || t.Anonymous
	}
	if anonymous {
		line(s.Warning.Render("[ANONYMOUS TUNNEL]"))
	} else {
		line(s.Empty.UnsetItalic().Render("[NO ANONYMOUS TUNNEL REPORTED]"))
	}
	fmt.Fprintln(&b)
	associations := len(c.Tunnels) + len(c.Client.Proxies)
	asn := ""
	if c.AS.Number != 0 {
		asn = fmt.Sprintf("AS%d", c.AS.Number)
	}
	row("Associations", associations)
	row("Country", c.Location.Country)
	row("Infrastructure", c.Infrastructure)
	row("ASN", asn)
	row("AS organization", c.AS.Organization)
	if c.Organization != "" {
		row("Organization", c.Organization)
	}
	row("Risk indicators", c.Risks)
	line(s.Empty.UnsetItalic().Render(strings.Repeat("─", min(56, width))))

	section("PROXY & ANONYMIZATION INTELLIGENCE")
	if len(c.Tunnels) == 0 {
		row("Tunnel operators", "")
	}
	for i, t := range c.Tunnels {
		row(fmt.Sprintf("Tunnel %d", i+1), t.Operator)
		row("Type", t.Type)
		row("Anonymous", t.Anonymous)
		if len(t.Entries) > 0 {
			row("Entry IPs", t.Entries)
		}
		if len(t.Exits) > 0 {
			row("Exit IPs", t.Exits)
		}
	}
	row("Client proxies", c.Client.Proxies)
	row("Services", c.Services)
	section("GEO INTELLIGENCE")
	row("IP geolocation", contextLocation(c.Location.City, c.Location.State, c.Location.Country))
	if con := c.Client.Concentration; con != nil {
		row("Concentration", contextLocation(con.City, con.State, con.Country))
		row("Client countries", c.Client.Countries)
		row("Density", con.Density)
		row("Skew (km)", con.Skew)
	} else {
		row("Concentration", "")
		row("Client countries", c.Client.Countries)
		row("Density", "")
		row("Skew (km)", "")
	}
	section("DEVICE ACTIVITY")
	row("Devices", c.Client.Count)
	row("Behavior signals", c.Client.Behaviors)
	row("Device types", c.Client.Types)
	row("Client spread", c.Client.Spread)
	if c.AI != nil {
		row("AI operator", c.AI.Operator)
		row("AI activity", c.AI.Types)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func contextLocation(parts ...string) string {
	var nonempty []string
	for _, part := range parts {
		if part != "" {
			nonempty = append(nonempty, part)
		}
	}
	return strings.Join(nonempty, ", ")
}
