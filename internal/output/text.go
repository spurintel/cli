package output

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/spurintel/cli/internal/spur"
)

// block groups aligned key/value rows beneath a report section heading.
type block struct {
	name   string
	values []kv
}

type kv struct {
	key string
	val any
}

// renderText dispatches supported response types to their text layouts. Other
// values fall back to JSON.
func renderText(w io.Writer, color bool, v any) error {
	// Styles and width come from the raw destination; the writes themselves go
	// through the profile writer, which degrades ANSI to what w supports.
	s := newStyles(w, color)
	width := min(80, terminalWidth(w))
	cw := Writer(w, color)
	switch c := v.(type) {
	case spur.Status:
		return renderStatus(cw, s, width, c)
	case *spur.Status:
		return renderStatus(cw, s, width, *c)
	case spur.TagMetadata:
		return renderTagMetadata(cw, s, width, c)
	case *spur.TagMetadata:
		return renderTagMetadata(cw, s, width, *c)
	case spur.FeedMetadata:
		return renderFeedMetadata(cw, s, width, c)
	case *spur.FeedMetadata:
		return renderFeedMetadata(cw, s, width, *c)
	case []spur.FeedType:
		return renderFeedTypes(cw, s, width, c)
	case spur.IPContext:
		return renderIPContext(cw, s, width, c)
	case *spur.IPContext:
		return renderIPContext(cw, s, width, *c)
	case []spur.IPContext:
		for i := range c {
			if i > 0 {
				if _, err := io.WriteString(cw, "\n"); err != nil {
					return err
				}
			}
			if err := renderIPContext(cw, s, width, c[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		return renderJSON(w, v)
	}
}

// renderStatus lays out a token-status report: a single block with the active
// flag (as a colored ●/○ marker), queries remaining, and service tier. The
// marker uses the Success/Warning palette so a healthy token reads at a glance;
// the underlying bool is preserved verbatim in the JSON and CSV renderings.
func renderStatus(w io.Writer, s Styles, width int, st spur.Status) error {
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
	writeBlock(&b, s, width, blk)
	_, err := io.WriteString(w, b.String())
	return err
}

// renderTagMetadata lays out a service-tag report: an Overview block (name,
// description, categories, anonymity), a Capabilities block of the boolean
// attribute flags, a Reach block (platforms/protocols/targeting), and, when the
// API returned them, an aggregate Metrics block. Optional flags that the API
// omitted collapse to the muted placeholder rather than reading as "false".
func renderTagMetadata(w io.Writer, s Styles, width int, t spur.TagMetadata) error {
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
	writeReportHeader(&b, s, width, label)
	for _, blk := range blocks {
		writeBlock(&b, s, width, blk)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// renderFeedTypes lays out the feed catalog: a single block listing each feed
// type as name → description, so `spur feed list` reads as a legible menu at a
// terminal and stays greppable.
func renderFeedTypes(w io.Writer, s Styles, width int, types []spur.FeedType) error {
	blk := block{}
	for _, ft := range types {
		blk.values = append(blk.values, kv{ft.Name, ft.Description})
	}

	var b strings.Builder
	writeBlock(&b, s, width, blk)
	_, err := io.WriteString(w, b.String())
	return err
}

// renderFeedMetadata lays out a feed's metadata document: a JSON block (the
// always-present newline-JSON artifact) and, when the API returned it, an MMDB
// block. Each shows the date, relative location, generation timestamps, and a
// human-readable filesize. The realtime feed omits the timestamps and filesize,
// which collapse to the muted placeholder rather than reading as zero.
func renderFeedMetadata(w io.Writer, s Styles, width int, md spur.FeedMetadata) error {
	fileBlock := func(name string, f spur.FeedFile) block {
		filesize := ""
		if f.Filesize > 0 {
			filesize = fmt.Sprintf("%s (%d bytes)", HumanizeBytes(f.Filesize), f.Filesize)
		}
		return block{name: name, values: []kv{
			{"Date", f.Date},
			{"Location", f.Location},
			{"Generated At", f.GeneratedAt},
			{"Available At", f.AvailableAt},
			{"Filesize", filesize},
			{"CRC32C", f.CRC32C},
			{"MD5", f.MD5},
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
	writeReportHeader(&b, s, width, label)
	for _, blk := range blocks {
		writeBlock(&b, s, width, blk)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// HumanizeBytes renders a byte count with a binary (IEC) unit suffix, so a
// multi-gigabyte feed reads as "1.2 GiB" rather than a wall of digits.
func HumanizeBytes(n int64) string {
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

// flexBoolValue turns an optional FlexBool into a report value:
// the underlying bool when the API set the field, or the empty string (a muted
// "None") when it was absent, so an unset flag never masquerades as a definite
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

// writeReportHeader identifies the returned entity without repeating the command.
func writeReportHeader(b *strings.Builder, s Styles, width int, label string) {
	if label != "" {
		fmt.Fprintln(b, ansi.Wrap(s.Title.Render(label), max(1, width), ""))
	}
}

// writeBlock separates sections with whitespace instead of tree connectors.
func writeBlock(b *strings.Builder, s Styles, width int, blk block) {
	if b.Len() > 0 {
		fmt.Fprintln(b)
	}
	if blk.name != "" {
		fmt.Fprintln(b, ansi.Wrap(s.Title.Render("■ "+strings.ToUpper(blk.name)), max(1, width), ""))
	}
	indent := 20
	for _, v := range blk.values {
		indent = max(indent, ansi.StringWidth(v.key)+2)
	}
	for _, v := range blk.values {
		writeReportRowAt(b, s, width, indent, v.key, v.val)
	}
}

// preStyled preserves a caller-provided status marker's semantic color.
type preStyled string

// reportValue formats text report values. Zero numbers and false booleans stay
// explicit; empty strings and lists display None.
func reportValue(s Styles, value any) string {
	if pre, ok := value.(preStyled); ok {
		return string(pre)
	}
	text, style := valueText(s, value)
	return style.Render(text)
}

func writeReportRow(b *strings.Builder, s Styles, width int, key string, value any) {
	writeReportRowAt(b, s, width, max(20, ansi.StringWidth(key)+2), key, value)
}

func writeReportRowAt(b *strings.Builder, s Styles, width, indent int, key string, value any) {
	text := reportValue(s, value)
	width = max(1, width)
	if width < indent+20 {
		fmt.Fprintln(b, ansi.Wrap(s.Empty.UnsetItalic().Render(key), width, ""))
		fmt.Fprintln(b, ansi.Wrap(text, width, ""))
		return
	}
	for i, line := range strings.Split(ansi.Wrap(text, width-indent, ""), "\n") {
		label := strings.Repeat(" ", indent)
		if i == 0 {
			label = s.Empty.UnsetItalic().Render(padRight(key, indent))
		}
		fmt.Fprintln(b, label+line)
	}
}

// valueText maps a field value to its display text and style.
func valueText(s Styles, v any) (string, lipgloss.Style) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return "None", s.Empty
		}
		return val, s.Value
	case bool:
		return fmt.Sprintf("%t", val), s.Bool
	case int:
		return fmt.Sprintf("%d", val), s.Num
	case float64:
		return formatFloat(val), s.Num
	case []string:
		if len(val) == 0 {
			return "None", s.Empty
		}
		return strings.Join(val, ", "), s.Value
	default:
		return fmt.Sprintf("%v", val), s.Value
	}
}

func padRight(v string, width int) string {
	if gap := width - lipgloss.Width(v); gap > 0 {
		return v + strings.Repeat(" ", gap)
	}
	return v
}

// formatFloat formats a float without a trailing ".0" for whole numbers and
// without excess precision, keeping the report tidy.
func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", f), "0"), ".")
}
