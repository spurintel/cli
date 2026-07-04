package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// maxContextBatch bounds how many IPs one get_ip_context call may look up. A
// batch is one Spur API request per IP, so the cap keeps a single tool call from
// spending an unbounded slice of the token's quota.
const maxContextBatch = 100

// GetIPContextInput is the argument object for get_ip_context. Supply either a
// single ip or a batch of ips (or both — they are concatenated in order).
type GetIPContextInput struct {
	IP  string   `json:"ip,omitempty" jsonschema:"A single IPv4 or IPv6 address to look up. Provide this, ips, or both."`
	IPs []string `json:"ips,omitempty" jsonschema:"Several IPv4/IPv6 addresses to look up in one call. Provide this, ip, or both."`
	DT  string   `json:"dt,omitempty" jsonschema:"Optional historical date in YYYYmmdd format. Requires an Enterprise subscription with historical access."`
}

// IPContextResult is one IP's outcome in a get_ip_context batch: either the
// context (with response metadata) or a per-IP error. One failed IP does not
// fail the others.
type IPContextResult struct {
	IP      string          `json:"ip" jsonschema:"The IP address this result is for."`
	Context *spur.IPContext `json:"context,omitempty" jsonschema:"The IP Context object, omitted when this IP failed. See resource spur://schemas/ip-context for the full field reference."`
	Meta    *spur.Meta      `json:"meta,omitempty" jsonschema:"Response metadata derived from Spur response headers, omitted when this IP failed."`
	Error   string          `json:"error,omitempty" jsonschema:"The failure reason when this IP could not be looked up; the other IPs in the batch still return."`
}

// GetIPContextOutput holds one result per requested IP, in request order.
type GetIPContextOutput struct {
	Results []IPContextResult `json:"results" jsonschema:"One entry per requested IP, in request order. For a single IP this is a one-element list."`
}

// registerGetIPContext wires the get_ip_context tool onto the server.
func registerGetIPContext(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_ip_context",
		Title: "Look up IP context",
		Description: "Retrieves the Spur IP Context for one or many IPv4/IPv6 addresses. " +
			"Pass a single 'ip' or a batch as 'ips'; the result is a list with one entry " +
			"per address, in request order (a single IP yields a one-element list). " +
			"Each entry returns enrichment data including AS info, geolocation, " +
			"infrastructure type, risk factors, services (VPN/proxy protocols), tunnels, " +
			"and client concentration; a per-IP error does not fail the others. Optional " +
			"'dt' enables historical lookups (Enterprise only). See resource " +
			"spur://schemas/ip-context for the full field reference. When assessing an " +
			"IP, also call get_tag_metadata in parallel on each tunnels[].operator, each " +
			"client.proxies[] entry, and each client.behaviors value matching ^*_USER$ " +
			"(strip the _USER suffix). See resource spur://docs/analyst-playbook for the " +
			"full procedure and field semantics.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetIPContextInput) (*mcp.CallToolResult, GetIPContextOutput, error) {
		dt, err := spur.ValidateHistoricalDate(in.DT)
		if err != nil {
			return toolError(err, "Use YYYYmmdd for historical lookups, for example 20250131."), GetIPContextOutput{}, nil
		}

		ips := make([]string, 0, 1+len(in.IPs))
		if strings.TrimSpace(in.IP) != "" {
			ips = append(ips, in.IP)
		}
		for _, ip := range in.IPs {
			if strings.TrimSpace(ip) != "" {
				ips = append(ips, ip)
			}
		}
		if len(ips) == 0 {
			return toolError(errors.New("no IP provided"), "Pass a single 'ip' or a batch as 'ips'."), GetIPContextOutput{}, nil
		}
		if len(ips) > maxContextBatch {
			return toolError(fmt.Errorf("too many IPs in one call: %d (max %d)", len(ips), maxContextBatch), "Split the batch into smaller calls."), GetIPContextOutput{}, nil
		}

		opts := spur.IPContextOptions{DT: dt}
		results := make([]IPContextResult, 0, len(ips))
		for i, ip := range ips {
			res, err := lookupOne(ctx, c, ip, opts)
			results = append(results, res)
			// A dead token or exhausted quota dooms every remaining lookup;
			// continuing would only burn round-trips (and, on 429, spend more
			// rate-limited requests). Mark the rest skipped and stop.
			if errors.Is(err, spur.ErrUnauthorized) || errors.Is(err, spur.ErrThrottled) {
				for _, rest := range ips[i+1:] {
					results = append(results, IPContextResult{IP: rest, Error: "skipped: " + res.Error})
				}
				break
			}
		}
		return nil, GetIPContextOutput{Results: results}, nil
	})
}

// lookupOne validates and looks up a single IP, returning its result with any
// error captured per-IP (so a bad address or API failure never sinks the rest
// of a batch). API errors are collapsed to the client's stable, sanitized
// message, matching the `spur context` command. The raw API error is also
// returned so the batch loop can classify batch-fatal failures (bad token,
// exhausted quota); a validation failure is per-IP only and returns nil.
func lookupOne(ctx context.Context, c *spur.Client, ip string, opts spur.IPContextOptions) (IPContextResult, error) {
	valid, err := spur.ValidateIP(ip)
	if err != nil {
		return IPContextResult{IP: ip, Error: err.Error()}, nil
	}
	body, meta, err := c.GetIPContext(ctx, valid, opts)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			return IPContextResult{IP: valid, Error: apiErr.Message()}, err
		}
		return IPContextResult{IP: valid, Error: err.Error()}, err
	}
	return IPContextResult{IP: valid, Context: &body, Meta: &meta}, nil
}

// statusHint returns a remediation hint for known Spur API errors, or "" if
// none applies.
func statusHint(err error) string {
	switch {
	case errors.Is(err, spur.ErrUnauthorized):
		return "Check that the SPUR_TOKEN environment variable is set correctly."
	case errors.Is(err, spur.ErrForbidden):
		return "Your token is valid but lacks Context API access. Contact sales@spur.us."
	case errors.Is(err, spur.ErrThrottled):
		return "Run get_api_status to verify remaining quota, or wait for the next billing cycle."
	case errors.Is(err, spur.ErrBadRequest):
		return "The IP must be a public IPv4 or IPv6 address."
	}
	return ""
}
