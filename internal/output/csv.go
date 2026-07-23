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
	case spur.Status:
		return writeStatusCSV(cw, c)
	case *spur.Status:
		return writeStatusCSV(cw, *c)
	case spur.TagMetadata:
		return writeTagMetadataCSV(cw, c)
	case *spur.TagMetadata:
		return writeTagMetadataCSV(cw, *c)
	case spur.FeedMetadata:
		return writeFeedMetadataCSV(cw, c)
	case *spur.FeedMetadata:
		return writeFeedMetadataCSV(cw, *c)
	case []spur.FeedType:
		return writeFeedTypesCSV(cw, c)
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

// statusCSVHeader mirrors the /status JSON fields so a scripted `spur status
// --format csv` maps one-to-one onto the API shape.
var statusCSVHeader = []string{"active", "queriesRemaining", "serviceTier"}

// writeStatusCSV writes the status header row and a single record. The active
// flag is emitted as a bare true/false so the column stays machine-parseable,
// unlike the styled ●/○ marker used in the text report.
func writeStatusCSV(cw *csv.Writer, s spur.Status) error {
	if err := cw.Write(statusCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	if err := cw.Write([]string{
		strconv.FormatBool(s.Active),
		strconv.Itoa(s.QueriesRemaining),
		s.ServiceTier,
	}); err != nil {
		return fmt.Errorf("write csv record: %w", err)
	}
	cw.Flush()
	return cw.Error()
}

// tagMetadataCSVHeader flattens the tag report into scalar columns: the
// identity and anonymity fields, pipe-joined category/platform/protocol lists,
// and the aggregate metrics. Absent optional flags emit an empty cell so a
// scripted reader can tell "unknown" from a definite false.
var tagMetadataCSVHeader = []string{
	"tag",
	"name",
	"categories",
	"isAnonymous",
	"platforms",
	"protocols",
	"metrics.distinctIPs",
	"metrics.distinctASNs",
	"metrics.distinctISPs",
	"metrics.distinctCountries",
	"metrics.averageDeviceCount",
	"metrics.churnRate",
}

func writeTagMetadataCSV(cw *csv.Writer, t spur.TagMetadata) error {
	if err := cw.Write(tagMetadataCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	var m spur.TagMetrics
	if t.Metrics != nil {
		m = *t.Metrics
	}
	if err := cw.Write([]string{
		t.Tag,
		t.Name,
		strings.Join(t.Categories, "|"),
		flexBoolCSV(t.IsAnonymous),
		strings.Join(t.Platforms, "|"),
		strings.Join(t.Protocols, "|"),
		m.DistinctIPs.String(),
		m.DistinctASNs.String(),
		m.DistinctISPs.String(),
		m.DistinctCountries.String(),
		m.AverageDeviceCount.String(),
		m.ChurnRate.String(),
	}); err != nil {
		return fmt.Errorf("write csv record: %w", err)
	}
	cw.Flush()
	return cw.Error()
}

// flexBoolCSV renders an optional FlexBool for CSV: "true"/"false" when set, an
// empty cell when the API omitted the field.
func flexBoolCSV(b *spur.FlexBool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(bool(*b))
}

// feedTypeCSVHeader maps the feed catalog to two columns so `spur feed list
// --format csv` is a flat name/description table.
var feedTypeCSVHeader = []string{"feed", "description"}

func writeFeedTypesCSV(cw *csv.Writer, types []spur.FeedType) error {
	if err := cw.Write(feedTypeCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, ft := range types {
		if err := cw.Write([]string{ft.Name, ft.Description}); err != nil {
			return fmt.Errorf("write csv record: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// feedMetadataCSVHeader flattens both artifact containers into one row: the
// requested feed type, then the JSON and MMDB file fields. MMDB columns are
// empty when the feed is not offered in that format, and filesize is empty when
// absent (the realtime feed) so a scripted reader can tell "unknown" from zero.
// crc32c/md5 are populated only when the status command's best-effort CDN
// probe succeeded; they are empty otherwise.
var feedMetadataCSVHeader = []string{
	"feedType",
	"json.date",
	"json.location",
	"json.generatedAt",
	"json.availableAt",
	"json.filesize",
	"json.crc32c",
	"json.md5",
	"mmdb.date",
	"mmdb.location",
	"mmdb.generatedAt",
	"mmdb.availableAt",
	"mmdb.filesize",
	"mmdb.crc32c",
	"mmdb.md5",
}

func writeFeedMetadataCSV(cw *csv.Writer, md spur.FeedMetadata) error {
	if err := cw.Write(feedMetadataCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	record := append([]string{md.Type}, feedFileCSVFields(md.JSON)...)
	if md.MMDB != nil {
		record = append(record, feedFileCSVFields(*md.MMDB)...)
	} else {
		record = append(record, feedFileCSVFields(spur.FeedFile{})...)
	}
	if err := cw.Write(record); err != nil {
		return fmt.Errorf("write csv record: %w", err)
	}
	cw.Flush()
	return cw.Error()
}

// feedFileCSVFields renders one FeedFile as its seven columns. A zero filesize
// (absent on the realtime feed, and on an empty MMDB container) emits an empty
// cell rather than "0"; CRC32C/MD5 are empty unless the CDN probe populated
// them.
func feedFileCSVFields(f spur.FeedFile) []string {
	filesize := ""
	if f.Filesize > 0 {
		filesize = strconv.FormatInt(f.Filesize, 10)
	}
	return []string{f.Date, f.Location, f.GeneratedAt, f.AvailableAt, filesize, f.CRC32C, f.MD5}
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
