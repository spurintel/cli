package tools

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// FormatAssessmentInput is the argument object for format_assessment. The
// caller (LLM) passes the raw Spur responses alongside two short prose
// blocks it authors itself. The tool validates select fidelity rules around
// risk citations and density formatting, then renders the final
// customer-facing markdown deterministically.
type FormatAssessmentInput struct {
	Context    spur.IPContext              `json:"context" jsonschema:"The complete get_ip_context result for the IP being assessed."`
	Tags       map[string]spur.TagMetadata `json:"tags,omitempty" jsonschema:"Map from service tag (e.g. NEXX_PROXY) to its get_tag_metadata result. Include every tunnels[].operator, every client.proxies[] entry, and every client.behaviors value matching ^*_USER$ (stripped of the _USER suffix). Omit if no tags apply."`
	BottomLine string                      `json:"bottom_line" jsonschema:"One sentence classifying the IP plus risk framing when warranted. Must not cite any risk enum (AD_FRAUD, CALLBACK_PROXY, GEO_MISMATCH, LOGIN_BRUTEFORCE, TUNNEL, WEB_SCRAPING) that is not present in context.risks[]. Must not use paraphrases like 'geo mismatch' or 'ad fraud' unless the corresponding enum is in risks[]. Must not convert density to a percentage."`
	Assessment string                      `json:"assessment" jsonschema:"Plain-English analyst paragraph explaining what the findings mean for the customer. Reference only facts present in context and tags. Do not invent risk enums or extrapolate use cases (ad fraud, data quality, ATO, bot detection, etc.) that Spur did not flag."`
}

// FormatAssessmentOutput wraps the rendered markdown. We return the final
// customer-facing text in a single field so clients can forward it without
// reassembly.
type FormatAssessmentOutput struct {
	Markdown string `json:"markdown" jsonschema:"Rendered assessment markdown with the six-section template plus the analyst assessment paragraph. Suitable for sending verbatim to the end user."`
}

// registerFormatAssessment wires the format_assessment tool onto the server.
func registerFormatAssessment(s *mcp.Server) {
	opts := &jsonschema.ForOptions{TypeSchemas: flexTypeSchemas()}
	in, err := jsonschema.For[FormatAssessmentInput](opts)
	if err != nil {
		panic(fmt.Errorf("format_assessment input schema: %w", err))
	}
	out, err := jsonschema.For[FormatAssessmentOutput](opts)
	if err != nil {
		panic(fmt.Errorf("format_assessment output schema: %w", err))
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:  "format_assessment",
		Title: "Render a Spur IP assessment",
		Description: "Renders the final customer-facing markdown for a Spur IP assessment. " +
			"Call this LAST, after get_ip_context and all get_tag_metadata lookups have " +
			"returned. Pass the raw get_ip_context result as `context`, a map of " +
			"tag->get_tag_metadata results as `tags`, a one-sentence `bottom_line`, and " +
			"a plain-English `assessment` paragraph. The tool validates specific " +
			"fidelity rules around risk citations and density formatting, and returns " +
			"the final markdown with Bottom line / Network / " +
			"Operators / Client signal / Risks / Recommendation / Assessment sections. " +
			"If validation fails, the tool returns an error explaining which rule was " +
			"violated; fix the prose and retry.",
		InputSchema:  in,
		OutputSchema: out,
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FormatAssessmentInput) (*mcp.CallToolResult, FormatAssessmentOutput, error) {
		if err := validateAssessment(in); err != nil {
			return toolError(err, "Revise bottom_line and/or assessment to respect the cited rule, then retry format_assessment."), FormatAssessmentOutput{}, nil
		}
		md := renderAssessment(in)
		return nil, FormatAssessmentOutput{Markdown: md}, nil
	})
}

// riskEnums is the complete set Spur may return in context.risks[].
var riskEnums = []string{
	"AD_FRAUD",
	"CALLBACK_PROXY",
	"GEO_MISMATCH",
	"LOGIN_BRUTEFORCE",
	"TUNNEL",
	"WEB_SCRAPING",
}

// riskGlosses maps each enum to the plain-English gloss used in the rendered
// Risks section. Kept in one place so analyst output is consistent across IPs.
var riskGlosses = map[string]string{
	"AD_FRAUD":         "ad-fraud signatures observed.",
	"CALLBACK_PROXY":   "network can route traffic for residential or malware proxies.",
	"GEO_MISMATCH":     "datacenter location differs from client location.",
	"LOGIN_BRUTEFORCE": "persistent login attempts observed.",
	"TUNNEL":           "anonymizing-tunnel exit.",
	"WEB_SCRAPING":     "automated or headless scraping observed.",
}

// riskParaphrases maps common prose paraphrases of risk enums back to the
// enum tokens. If the LLM uses the phrase but the enum is not in risks[],
// validation fails.
var riskParaphrases = map[string]string{
	"ad fraud":            "AD_FRAUD",
	"ad-fraud":            "AD_FRAUD",
	"callback proxy":      "CALLBACK_PROXY",
	"callback-proxy":      "CALLBACK_PROXY",
	"geo mismatch":        "GEO_MISMATCH",
	"geo-mismatch":        "GEO_MISMATCH",
	"geographic mismatch": "GEO_MISMATCH",
	"login bruteforce":    "LOGIN_BRUTEFORCE",
	"login brute-force":   "LOGIN_BRUTEFORCE",
	"web scraping":        "WEB_SCRAPING",
}

// validateAssessment enforces fidelity rules on the LLM-authored prose. It
// rejects the input if the LLM cited or paraphrased a risk enum that is not
// present in context.risks[].
func validateAssessment(in FormatAssessmentInput) error {
	if strings.TrimSpace(in.BottomLine) == "" {
		return fmt.Errorf("bottom_line is required")
	}
	if strings.TrimSpace(in.Assessment) == "" {
		return fmt.Errorf("assessment is required")
	}

	present := map[string]bool{}
	for _, r := range in.Context.Risks {
		present[strings.ToUpper(r)] = true
	}

	combined := in.BottomLine + "\n" + in.Assessment
	combinedLower := strings.ToLower(combined)

	// Direct enum tokens — CASE-SENSITIVE. The lowercase form of an enum like
	// TUNNEL is a normal English word ("no tunnels observed"); we only reject
	// when the LLM cites the enum in its formal uppercase form.
	for _, enum := range riskEnums {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(enum) + `\b`)
		if re.MatchString(combined) && !present[enum] {
			return fmt.Errorf(
				"bottom_line/assessment cites risk enum %q, but it is not in context.risks[]=%v. "+
					"Only cite risks Spur actually returned, or rephrase to remove the reference.",
				enum, in.Context.Risks,
			)
		}
	}

	// Paraphrases — case-insensitive. These phrases ("geo mismatch", "ad
	// fraud", etc.) are distinctive multi-word terms that customers use to
	// name the enum by common-English alias. Even a negation like "no geo
	// mismatch" is worth flagging because it still anchors the customer's
	// attention on a Spur classification that was not actually returned.
	for phrase, enum := range riskParaphrases {
		if strings.Contains(combinedLower, phrase) && !present[enum] {
			return fmt.Errorf(
				"bottom_line/assessment uses the phrase %q, which maps to risk enum %q, but that enum is not in context.risks[]=%v. "+
					"Either rephrase without the term, or drop the claim.",
				phrase, enum, in.Context.Risks,
			)
		}
	}

	// Density-as-percentage check.
	if in.Context.Client.Concentration != nil {
		pctRe := regexp.MustCompile(`density\s*[^\d]*\d+(\.\d+)?\s*%`)
		if pctRe.MatchString(combined) {
			return fmt.Errorf(
				"density is a 0-1 decimal (%.3f in this response); do not convert to a percentage. Rewrite as 'density %.3f'.",
				in.Context.Client.Concentration.Density, in.Context.Client.Concentration.Density,
			)
		}
	}

	return nil
}

// renderAssessment produces the final markdown. All facts are drawn from the
// structured input; only bottom_line and assessment come from LLM prose. The
// output shape is identical for every IP so customers learn a single format.
func renderAssessment(in FormatAssessmentInput) string {
	ctx := in.Context
	var b strings.Builder

	// Bottom line. Append services suffix if any.
	bottom := strings.TrimSpace(in.BottomLine)
	if len(ctx.Services) > 0 {
		// Only append if the prose didn't already mention services.
		if !strings.Contains(strings.ToLower(bottom), "exposed services") {
			suffix := " Exposed services: " + strings.Join(ctx.Services, ", ") + "."
			if !strings.HasSuffix(bottom, ".") {
				bottom += "."
			}
			bottom += suffix
		}
	}
	fmt.Fprintf(&b, "**Bottom line:** %s\n\n", bottom)

	// Network.
	if ctx.AS.Number != 0 || ctx.AS.Organization != "" || ctx.Location.City != "" {
		parts := []string{}
		if ctx.AS.Number != 0 || ctx.AS.Organization != "" {
			parts = append(parts, fmt.Sprintf("AS %d (%s)", ctx.AS.Number, ctx.AS.Organization))
		}
		if ctx.Location.City != "" || ctx.Location.State != "" || ctx.Location.Country != "" {
			parts = append(parts, fmt.Sprintf("Registered in %s.", joinLocation(ctx.Location)))
		}
		fmt.Fprintf(&b, "**Network:** %s\n\n", strings.Join(parts, ". "))
	}

	// Operators: iterate tunnels[] first (IP attributes), then client.proxies[]
	// (client attributes), then _USER behaviors, deduping on tag name.
	seen := map[string]bool{}
	type opRow struct {
		tag         string
		fromClient  bool
		fromUserTag bool
	}
	rows := []opRow{}
	for _, t := range ctx.Tunnels {
		if !seen[t.Operator] && t.Operator != "" {
			rows = append(rows, opRow{tag: t.Operator})
			seen[t.Operator] = true
		}
	}
	for _, p := range ctx.Client.Proxies {
		if !seen[p] && p != "" {
			rows = append(rows, opRow{tag: p, fromClient: true})
			seen[p] = true
		}
	}
	for _, behavior := range ctx.Client.Behaviors {
		if tag, ok := strings.CutSuffix(behavior, "_USER"); ok {
			if !seen[tag] && tag != "" {
				rows = append(rows, opRow{tag: tag, fromUserTag: true})
				seen[tag] = true
			}
		}
	}
	if len(rows) > 0 {
		b.WriteString("**Operators:**\n")
		for _, r := range rows {
			meta, ok := in.Tags[r.tag]
			b.WriteString(renderOperatorBullet(r.tag, meta, ok, r.fromClient || r.fromUserTag))
		}
		b.WriteString("\n")
	}

	// Client signal: Part A (concentration) + Part B (infrastructure).
	var csParts []string
	if ctx.Client.Concentration != nil || ctx.Client.Count > 0 {
		csParts = append(csParts, renderConcentration(ctx))
	}
	if ctx.Infrastructure != "" && isAllowedInfra(ctx.Infrastructure) {
		csParts = append(csParts, fmt.Sprintf("Infrastructure: %s.", ctx.Infrastructure))
	}
	if len(csParts) > 0 {
		fmt.Fprintf(&b, "**Client signal:** %s\n\n", strings.Join(csParts, " "))
	}

	// Risks.
	if len(ctx.Risks) > 0 {
		b.WriteString("**Risks:**\n")
		for _, r := range ctx.Risks {
			gloss := riskGlosses[r]
			if gloss == "" {
				gloss = "see explain_enum for details."
			}
			fmt.Fprintf(&b, "- `%s` — %s\n", r, gloss)
		}
		b.WriteString("\n")
	}

	// Recommendation (auto-picked based on flags).
	if rec := recommendation(in); rec != "" {
		fmt.Fprintf(&b, "**Recommendation:** %s\n\n", rec)
	}

	// Assessment (LLM-authored prose).
	fmt.Fprintf(&b, "**Assessment:** %s\n", strings.TrimSpace(in.Assessment))

	// Dashboard link for the analyst to drill in.
	if ctx.IP != "" {
		fmt.Fprintf(&b, "\n[View in Spur dashboard](https://app.spur.us/search?q=%s)\n", url.QueryEscape(ctx.IP))
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func joinLocation(l spur.Location) string {
	parts := []string{}
	if l.City != "" {
		parts = append(parts, l.City)
	}
	if l.State != "" {
		parts = append(parts, l.State)
	}
	if l.Country != "" {
		parts = append(parts, l.Country)
	}
	return strings.Join(parts, ", ")
}

// renderOperatorBullet emits one "- **Name (`TAG`)** — …" line for a tag. If
// metadata is missing we still emit the tag, because Step 2 required a lookup.
func renderOperatorBullet(tag string, meta spur.TagMetadata, haveMeta, fromClient bool) string {
	name := meta.Name
	if name == "" {
		name = tag
	}
	qualifiers := []string{}
	if cat := primaryCategory(meta); cat != "" {
		qualifiers = append(qualifiers, cat)
	}
	if flexBoolTrue(meta.IsAnonymous) {
		qualifiers = append(qualifiers, "anonymous")
	}
	if flexBoolTrue(meta.IsNoLog) {
		qualifiers = append(qualifiers, "no-log")
	}
	if flexBoolTrue(meta.IsCallbackProxy) {
		qualifiers = append(qualifiers, "**callback-proxy**")
	}
	if flexBoolTrue(meta.IsInactive) {
		qualifiers = append(qualifiers, "inactive per Spur")
	}

	body := fmt.Sprintf("- **%s (`%s`)** — %s", name, tag, strings.Join(qualifiers, ", "))

	if meta.Metrics != nil {
		ips := flexNumberInt(meta.Metrics.DistinctIPs)
		asns := flexNumberInt(meta.Metrics.DistinctASNs)
		ctys := flexNumberInt(meta.Metrics.DistinctCountries)
		if ips > 0 || asns > 0 || ctys > 0 {
			body += fmt.Sprintf("; %s distinct IPs across %s ASNs / %s countries", humanInt(ips), humanInt(asns), humanInt(ctys))
		}
	}

	if fromClient && haveMeta {
		body += " (observed via clients on this IP, not an attribute of the IP itself)"
	} else if !haveMeta {
		body += " (Spur tracks this tag but returned no metadata)"
	}

	if !strings.HasSuffix(body, ".") {
		body += "."
	}
	return body + "\n"
}

// primaryCategory returns a lowercase phrasing of the first category on a tag.
func primaryCategory(meta spur.TagMetadata) string {
	if len(meta.Categories) == 0 {
		return ""
	}
	switch meta.Categories[0] {
	case "ISP_PROXY":
		return "ISP proxy"
	case "RESIDENTIAL_PROXY":
		return "residential proxy"
	case "DATACENTER_PROXY":
		return "datacenter proxy"
	case "MOBILE_PROXY":
		return "mobile proxy"
	case "COMMERCIAL_VPN":
		return "commercial VPN"
	case "ENTERPRISE_VPN":
		return "enterprise VPN"
	case "FREE_VPN":
		return "free VPN"
	case "BLOCKCHAIN_VPN":
		return "blockchain VPN"
	case "CDN_PROXY":
		return "CDN proxy"
	case "P2P_PROXY":
		return "P2P proxy"
	case "PUBLIC_PROXY":
		return "public proxy"
	case "ZTNA_PROVIDER":
		return "ZTNA provider"
	default:
		return strings.ToLower(strings.ReplaceAll(meta.Categories[0], "_", " "))
	}
}

func renderConcentration(ctx spur.IPContext) string {
	c := ctx.Client.Concentration
	parts := []string{}
	if c != nil && (c.City != "" || c.State != "") {
		loc := c.City
		if c.State != "" {
			if loc != "" {
				loc += ", "
			}
			loc += c.State
		}
		parts = append(parts, fmt.Sprintf("Clients cluster around %s", loc))
	}
	if c != nil {
		sub := fmt.Sprintf("(density %.3f, ~%.0f km skew from the IP's registered location)", c.Density, c.Skew)
		if len(parts) == 0 {
			parts = append(parts, "Client concentration "+sub)
		} else {
			parts[len(parts)-1] = parts[len(parts)-1] + " " + sub
		}
	}
	summary := ""
	if ctx.Client.Count > 0 {
		ctys := ctx.Client.Countries
		summary = fmt.Sprintf("%s client(s) observed across %s countr%s", humanInt(ctx.Client.Count), humanInt(ctys), countrySuffix(ctys))
	}
	if len(ctx.Client.Types) > 0 {
		// canonical order
		types := append([]string(nil), ctx.Client.Types...)
		sort.Strings(types)
		if summary != "" {
			summary += "; "
		} else {
			summary = ""
		}
		summary += "types: " + strings.Join(types, ", ")
	}

	line := strings.Join(parts, "")
	if line != "" && !strings.HasSuffix(line, ".") {
		line += "."
	}
	if summary != "" {
		if line != "" {
			line += " "
		}
		line += summary + "."
	}
	return line
}

func countrySuffix(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// humanInt formats an integer with thousands separators for readability.
func humanInt(n int) string {
	if n < 0 {
		return "-" + humanInt(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// recommendation picks the canonical recommendation string. CALLBACK_PROXY
// takes priority over plain-anonymous.
func recommendation(in FormatAssessmentInput) string {
	callback := false
	for _, r := range in.Context.Risks {
		if strings.EqualFold(r, "CALLBACK_PROXY") {
			callback = true
			break
		}
	}
	if !callback {
		for _, meta := range in.Tags {
			if flexBoolTrue(meta.IsCallbackProxy) {
				callback = true
				break
			}
		}
	}
	if callback {
		return "Block for auth, payment, and account-change flows; any successful action from this IP should trigger review."
	}

	anonymous := false
	for _, t := range in.Context.Tunnels {
		if t.Anonymous {
			anonymous = true
			break
		}
	}
	if !anonymous {
		for _, meta := range in.Tags {
			if flexBoolTrue(meta.IsAnonymous) {
				anonymous = true
				break
			}
		}
	}
	if anonymous {
		return "Block or step-up auth for high-value flows; route successful auth from this IP to review."
	}

	return ""
}

func isAllowedInfra(v string) bool {
	switch v {
	case "DATACENTER", "MOBILE", "SATELLITE", "IN_FLIGHT_WIFI", "GOOGLE":
		return true
	}
	return false
}

// flexBoolTrue returns true iff ptr is non-nil and the underlying FlexBool is
// true. The Spur API sometimes returns boolean-ish fields as strings.
func flexBoolTrue(p *spur.FlexBool) bool {
	if p == nil {
		return false
	}
	return bool(*p)
}

// flexNumberInt parses a FlexNumber (which is a string holding the raw
// textual number Spur returned) into an int, rounding toward zero. Returns 0
// when the field is absent or unparseable.
func flexNumberInt(n spur.FlexNumber) int {
	s := strings.TrimSpace(string(n))
	if s == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}
