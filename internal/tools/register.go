// Package tools wires Spur-backed MCP tools onto an mcp.Server.
package tools

import (
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
	"github.com/spurintel/cli/internal/spur/enumdict"
)

// Register adds every Spur tool to s, wired to the provided client and enum
// dictionary.
func Register(s *mcp.Server, c *spur.Client, dict *enumdict.Dict) {
	registerGetIPContext(s, c)
	registerGetTagMetadata(s, c)
	registerGetAPIStatus(s, c)
	registerExplainEnum(s, dict)
	registerFormatAssessment(s)
}

// toolError converts a Go error into an MCP tool-error result. A non-empty
// hint is appended on a second line to guide the caller.
func toolError(err error, hint string) *mcp.CallToolResult {
	msg := err.Error()
	var apiErr *spur.APIError
	if errors.As(err, &apiErr) {
		msg = apiErr.Message()
	}
	if hint != "" {
		msg = fmt.Sprintf("%s\nHint: %s", msg, hint)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}
