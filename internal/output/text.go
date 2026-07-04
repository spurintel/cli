package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/spurintel/cli/internal/spur"
)

// block is a titled group of aligned key/value rows, rendered under a tree
// connector. The layout mirrors the synthient/cli output package: a ◆ header,
// then blocks joined by ├ / └ connectors with │-prefixed rows.
type block struct {
	name   string
	values []kv
}

type kv struct {
	key string
	val any
}

// renderText writes a styled, aligned report for the values this cut knows how
// to lay out. Anything else falls back to JSON so the renderer never refuses a
// value outright; styled layouts are added per entity as commands land.
func renderText(w io.Writer, color bool, v any) error {
	s := newStyles(w, color)
	switch c := v.(type) {
	case spur.Status:
		return renderStatus(w, s, c)
	case *spur.Status:
		return renderStatus(w, s, *c)
	case spur.TagMetadata:
		return renderTagMetadata(w, s, c)
	case *spur.TagMetadata:
		return renderTagMetadata(w, s, *c)
	case spur.FeedMetadata:
		return renderFeedMetadata(w, s, c)
	case *spur.FeedMetadata:
		return renderFeedMetadata(w, s, *c)
	case []spur.FeedType:
		return renderFeedTypes(w, s, c)
	case spur.IPContext:
		return renderIPContext(w, s, c)
	case *spur.IPContext:
		return renderIPContext(w, s, *c)
	case []spur.IPContext:
		for i := range c {
			if i > 0 {
				if _, err := io.WriteString(w, "\n"); err != nil {
					return err
				}
			}
			if err := renderIPContext(w, s, c[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		return renderJSON(w, v)
	}
}

func renderIPContext(w io.Writer, s Styles, c spur.IPContext) error {
	blocks := []block{
		{name: "Network", values: []kv{
			{"ASN", c.AS.Number},
			{"AS Org", c.AS.Organization},
			{"Organization", c.Organization},
			{"Infrastructure", c.Infrastructure},
		}},
		{name: "Location", values: []kv{
			{"City", c.Location.City},
			{"State", c.Location.State},
			{"Country", c.Location.Country},
		}},
		{name: "Client", values: []kv{
			{"Count", c.Client.Count},
			{"Countries", c.Client.Countries},
			{"Spread", c.Client.Spread},
			{"Types", c.Client.Types},
			{"Behaviors", c.Client.Behaviors},
			{"Proxies", c.Client.Proxies},
		}},
		{name: "Assessment", values: []kv{
			{"Risks", c.Risks},
			{"Services", c.Services},
		}},
	}

	if con := c.Client.Concentration; con != nil {
		blocks = append(blocks, block{name: "Concentration", values: []kv{
			{"City", con.City},
			{"State", con.State},
			{"Country", con.Country},
			{"Density", con.Density},
			{"Skew (km)", con.Skew},
		}})
	}
	if ai := c.AI; ai != nil {
		blocks = append(blocks, block{name: "AI Activity", values: []kv{
			{"Operator", ai.Operator},
			{"Types", ai.Types},
		}})
	}
	if len(c.Tunnels) > 0 {
		tun := block{name: "Tunnels"}
		for _, t := range c.Tunnels {
			label := t.Operator
			if label == "" {
				label = t.Type
			}
			detail := t.Type
			if t.Anonymous {
				detail += " (anonymous)"
			}
			tun.values = append(tun.values, kv{label, detail})
		}
		blocks = append(blocks, tun)
	}

	var b strings.Builder
	fmt.Fprintln(&b, s.Title.Render("◆  IP ")+s.Value.Render(c.IP))
	for i, blk := range blocks {
		writeBlock(&b, s, blk, i+1 == len(blocks))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// renderStatus lays out a token-status report: a single block with the active
// flag (as a colored ●/○ marker), queries remaining, and service tier. The
// marker uses the Success/Warning palette so a healthy token reads at a glance;
// the underlying bool is preserved verbatim in the JSON and CSV renderings.
func renderStatus(w io.Writer, s Styles, st spur.Status) error {
	marker := s.Warning.Render("○ inactive")
	if st.Active {
		marker = s.Success.Render("● active")
	}

	blk := block{values: []kv{
		{"Active", preStyled(marker)},
		{"Queries Remaining", st.QueriesRemaining},
		{"Service Tier", st.ServiceTier},
	}}

	var b strings.Builder
	fmt.Fprintln(&b, s.Title.Render("◆  Token Status"))
	writeBlock(&b, s, blk, true)
	_, err := io.WriteString(w, b.String())
	return err
}

// renderTagMetadata lays out a service-tag report: an Overview block (name,
// description, categories, anonymity), a Capabilities block of the boolean
// attribute flags, a Reach block (platforms/protocols/targeting), and, when the
// API returned them, an aggregate Metrics block. Optional flags that the API
// omitted collapse to the muted placeholder rather than reading as "false".
func renderTagMetadata(w io.Writer, s Styles, t spur.TagMetadata) error {
	blocks := []block{
		{name: "Overview", values: []kv{
			{"Name", t.Name},
			{"Description", t.Description},
			{"Website", t.Website},
			{"Categories", t.Categories},
			{"Anonymous", flexBoolValue(t.IsAnonymous)},
		}},
		{name: "Capabilities", values: []kv{
			{"No-log", flexBoolValue(t.IsNoLog)},
			{"Enterprise", flexBoolValue(t.IsEnterprise)},
			{"Multihop", flexBoolValue(t.AllowsMultihop)},
			{"Crypto", flexBoolValue(t.AllowsCrypto)},
			{"Free access", flexBoolValue(t.AllowsFreeAccess)},
			{"Torrents", flexBoolValue(t.AllowsTorrents)},
			{"Tracked", flexBoolValue(t.IsTracked)},
			{"Inactive", flexBoolValue(t.IsInactive)},
		}},
		{name: "Reach", values: []kv{
			{"Platforms", t.Platforms},
			{"Protocols", t.Protocols},
			{"Targeting", t.TargetingTypes},
			{"Resold by", t.ResoldBy},
		}},
	}

	if m := t.Metrics; m != nil {
		blocks = append(blocks, block{name: "Metrics", values: []kv{
			{"Distinct IPs", flexNumberValue(m.DistinctIPs)},
			{"Distinct ASNs", flexNumberValue(m.DistinctASNs)},
			{"Distinct ISPs", flexNumberValue(m.DistinctISPs)},
			{"Distinct Countries", flexNumberValue(m.DistinctCountries)},
			{"Avg Device Count", flexNumberValue(m.AverageDeviceCount)},
			{"Churn Rate", flexNumberValue(m.ChurnRate)},
		}})
	}

	label := t.Tag
	if label == "" {
		label = t.Name
	}

	var b strings.Builder
	fmt.Fprintln(&b, s.Title.Render("◆  Tag ")+s.Value.Render(label))
	for i, blk := range blocks {
		writeBlock(&b, s, blk, i+1 == len(blocks))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// renderFeedTypes lays out the feed catalog: a single block listing each feed
// type as name → description, so `spur feed list` reads as a legible menu at a
// terminal and stays greppable.
func renderFeedTypes(w io.Writer, s Styles, types []spur.FeedType) error {
	blk := block{name: "Feeds"}
	for _, ft := range types {
		blk.values = append(blk.values, kv{ft.Name, ft.Description})
	}

	var b strings.Builder
	fmt.Fprintln(&b, s.Title.Render("◆  Feed Catalog"))
	writeBlock(&b, s, blk, true)
	_, err := io.WriteString(w, b.String())
	return err
}

// renderFeedMetadata lays out a feed's metadata document: a JSON block (the
// always-present newline-JSON artifact) and, when the API returned it, an MMDB
// block. Each shows the date, relative location, generation timestamps, and a
// human-readable filesize. The realtime feed omits the timestamps and filesize,
// which collapse to the muted placeholder rather than reading as zero.
func renderFeedMetadata(w io.Writer, s Styles, md spur.FeedMetadata) error {
	fileBlock := func(name string, f spur.FeedFile) block {
		filesize := ""
		if f.Filesize > 0 {
			filesize = fmt.Sprintf("%s (%d bytes)", humanizeBytes(f.Filesize), f.Filesize)
		}
		return block{name: name, values: []kv{
			{"Date", f.Date},
			{"Location", f.Location},
			{"Generated At", f.GeneratedAt},
			{"Available At", f.AvailableAt},
			{"Filesize", filesize},
		}}
	}

	blocks := []block{fileBlock("JSON", md.JSON)}
	if md.MMDB != nil {
		blocks = append(blocks, fileBlock("MMDB", *md.MMDB))
	}

	label := md.Type
	if label == "" {
		label = "metadata"
	}

	var b strings.Builder
	fmt.Fprintln(&b, s.Title.Render("◆  Feed ")+s.Value.Render(label))
	for i, blk := range blocks {
		writeBlock(&b, s, blk, i+1 == len(blocks))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// humanizeBytes renders a byte count with a binary (IEC) unit suffix, so a
// multi-gigabyte feed reads as "1.2 GiB" rather than a wall of digits.
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// flexBoolValue turns an optional FlexBool into a value formatValue can style:
// the underlying bool when the API set the field, or the empty string (a muted
// "—") when it was absent, so an unset flag never masquerades as a definite
// "false".
func flexBoolValue(b *spur.FlexBool) any {
	if b == nil {
		return ""
	}
	return bool(*b)
}

// flexNumberValue renders a metric as its numeric text, collapsing the zero
// value (an empty FlexNumber) to a muted placeholder.
func flexNumberValue(n spur.FlexNumber) any {
	return n.String()
}

// writeBlock renders one block: a connector + name line, then a │-prefixed row
// per value with keys padded to a common width for alignment.
func writeBlock(b *strings.Builder, s Styles, blk block, final bool) {
	connector, row := "├", "│"
	if final {
		connector, row = "└", " "
	}

	if blk.name != "" {
		fmt.Fprintf(b, "%s  %s\n", s.Frame.Render(connector), s.Block.Render(blk.name))
	}

	keyWidth := 0
	for _, v := range blk.values {
		if l := len(v.key); l > keyWidth {
			keyWidth = l
		}
	}
	for _, v := range blk.values {
		key := s.Key.Render(padRight(v.key, keyWidth))
		fmt.Fprintf(b, "%s    %s  %s\n", s.Frame.Render(row), key, formatValue(s, v.val))
	}
}

// preStyled is a value the caller has already rendered to a styled string.
// formatValue emits it verbatim so callers can inject markers with their own
// color (e.g. the status active/inactive marker) without it being re-wrapped in
// the default Value style.
type preStyled string

// formatValue renders a field value in a type-appropriate style, collapsing
// absent scalars and empty slices to a muted placeholder.
func formatValue(s Styles, v any) string {
	switch val := v.(type) {
	case preStyled:
		return string(val)
	case string:
		if val == "" {
			return s.Empty.Render("—")
		}
		return s.Value.Render(val)
	case bool:
		return s.Bool.Render(fmt.Sprintf("%t", val))
	case int:
		return s.Num.Render(fmt.Sprintf("%d", val))
	case float64:
		return s.Num.Render(formatFloat(val))
	case []string:
		if len(val) == 0 {
			return s.Empty.Render("—")
		}
		return s.Value.Render(strings.Join(val, ", "))
	default:
		return s.Value.Render(fmt.Sprintf("%v", val))
	}
}

func padRight(v string, width int) string {
	if gap := width - len(v); gap > 0 {
		return v + strings.Repeat(" ", gap)
	}
	return v
}

// formatFloat formats a float without a trailing ".0" for whole numbers and
// without excess precision, keeping the report tidy.
func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", f), "0"), ".")
}
