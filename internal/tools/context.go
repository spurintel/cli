package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// GetIPContextInput is the argument object for get_ip_context.
type GetIPContextInput struct {
	IP string `json:"ip" jsonschema:"IPv4 or IPv6 address to look up."`
	DT string `json:"dt,omitempty" jsonschema:"Optional historical date in YYYYmmdd format. Requires an Enterprise subscription with historical access."`
}

// GetIPContextOutput combines the parsed context body with the Spur response
// headers (quota and result-date).
type GetIPContextOutput struct {
	Context spur.IPContext `json:"context" jsonschema:"The IP Context object. See resource spur://schemas/ip-context for the full field reference."`
	Meta    spur.Meta      `json:"meta" jsonschema:"Response metadata derived from Spur response headers."`
}

// registerGetIPContext wires the get_ip_context tool onto the server.
func registerGetIPContext(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_ip_context",
		Title: "Look up IP context",
		Description: "Retrieves the Spur IP Context for an IPv4 or IPv6 address. " +
			"Returns enrichment data including AS info, geolocation, infrastructure " +
			"type, risk factors, services (VPN/proxy protocols), tunnels, and client " +
			"concentration. Optional 'dt' enables historical lookups (Enterprise only). " +
			"See resource spur://schemas/ip-context for the full field reference. " +
			"When assessing an IP, also call get_tag_metadata in parallel on each " +
			"tunnels[].operator, each client.proxies[] entry, and each client.behaviors " +
			"value matching ^*_USER$ (strip the _USER suffix). See resource " +
			"spur://docs/analyst-playbook for the full procedure and field semantics.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetIPContextInput) (*mcp.CallToolResult, GetIPContextOutput, error) {
		dt, err := spur.ValidateHistoricalDate(in.DT)
		if err != nil {
			return toolError(err, "Use YYYYmmdd for historical lookups, for example 20250131."), GetIPContextOutput{}, nil
		}
		body, meta, err := c.GetIPContext(ctx, in.IP, spur.IPContextOptions{DT: dt})
		if err != nil {
			return toolError(err, statusHint(err)), GetIPContextOutput{}, nil
		}
		return nil, GetIPContextOutput{Context: body, Meta: meta}, nil
	})
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
