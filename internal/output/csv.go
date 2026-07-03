package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spurintel/cli/internal/spur"
)

// renderCSV writes a header row and one record per value. Slice-valued fields
// are pipe-joined within a cell so the row stays flat. Only entities with a
// defined column mapping are supported; anything else is an error rather than
// a silently malformed table.
func renderCSV(w io.Writer, v any) error {
	cw := csv.NewWriter(w)
	switch c := v.(type) {
	case spur.IPContext:
		return writeIPContextCSV(cw, c)
	case *spur.IPContext:
		return writeIPContextCSV(cw, *c)
	case []spur.IPContext:
		return writeIPContextCSVBatch(cw, c)
	default:
		return fmt.Errorf("csv output is not supported for %T", v)
	}
}

// ipContextCSVHeader is the column order shared by the single-value and batch
// CSV writers, so a batch is one header row followed by one record per context.
var ipContextCSVHeader = []string{
	"ip",
	"infrastructure",
	"organization",
	"as.number",
	"as.organization",
	"location.city",
	"location.state",
	"location.country",
	"client.count",
	"client.countries",
	"client.types",
	"client.behaviors",
	"client.proxies",
	"risks",
	"services",
}

// writeIPContextCSVBatch writes the header once, then one record per context.
func writeIPContextCSVBatch(cw *csv.Writer, cs []spur.IPContext) error {
	if err := cw.Write(ipContextCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for i := range cs {
		if err := cw.Write(ipContextCSVRecord(cs[i])); err != nil {
			return fmt.Errorf("write csv record: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeIPContextCSV(cw *csv.Writer, c spur.IPContext) error {
	if err := cw.Write(ipContextCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	if err := cw.Write(ipContextCSVRecord(c)); err != nil {
		return fmt.Errorf("write csv record: %w", err)
	}
	cw.Flush()
	return cw.Error()
}

// ipContextCSVRecord flattens one context into a row matching ipContextCSVHeader.
func ipContextCSVRecord(c spur.IPContext) []string {
	return []string{
		c.IP,
		c.Infrastructure,
		c.Organization,
		strconv.Itoa(c.AS.Number),
		c.AS.Organization,
		c.Location.City,
		c.Location.State,
		c.Location.Country,
		strconv.Itoa(c.Client.Count),
		strconv.Itoa(c.Client.Countries),
		strings.Join(c.Client.Types, "|"),
		strings.Join(c.Client.Behaviors, "|"),
		strings.Join(c.Client.Proxies, "|"),
		strings.Join(c.Risks, "|"),
		strings.Join(c.Services, "|"),
	}
}
