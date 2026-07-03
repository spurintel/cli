package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// GetAPIStatusInput is intentionally empty — the endpoint takes no arguments.
type GetAPIStatusInput struct{}

// GetAPIStatusOutput is the API token status.
type GetAPIStatusOutput struct {
	Status spur.Status `json:"status" jsonschema:"Status of the configured SPUR_TOKEN."`
}

// registerGetAPIStatus wires the get_api_status tool.
func registerGetAPIStatus(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_api_status",
		Title:       "API token status",
		Description: "Returns the status of the SPUR_TOKEN: whether it is active, how many queries remain in the current billing period, and the service tier.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetAPIStatusInput) (*mcp.CallToolResult, GetAPIStatusOutput, error) {
		body, err := c.GetStatus(ctx)
		if err != nil {
			return toolError(err, statusHint(err)), GetAPIStatusOutput{}, nil
		}
		return nil, GetAPIStatusOutput{Status: body}, nil
	})
}
