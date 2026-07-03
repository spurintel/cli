package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur/enumdict"
)

// ExplainEnumInput is the argument for explain_enum.
type ExplainEnumInput struct {
	Value string `json:"value" jsonschema:"An enum value used in Spur responses, e.g., CALLBACK_PROXY, OPENVPN, DATACENTER, RESIDENTIAL_PROXY. Case-insensitive."`
}

// ExplainEnumOutput is the list of matching entries (one value can occur in
// multiple fields; each occurrence is reported).
type ExplainEnumOutput struct {
	Matches []enumdict.Entry `json:"matches" jsonschema:"Entries that matched the value. Empty when no match."`
}

// registerExplainEnum wires the explain_enum tool.
func registerExplainEnum(s *mcp.Server, dict *enumdict.Dict) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "explain_enum",
		Title: "Explain a Spur enum value",
		Description: "Looks up the meaning of an enum value used by the Spur Context API " +
			"(e.g., CALLBACK_PROXY, GEO_MISMATCH, OPENVPN, DATACENTER, RESIDENTIAL_PROXY). " +
			"Returns the field it belongs to, the schema, and the human description. " +
			"Answers client questions about what a risk/service/infrastructure value means " +
			"without making an API call.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ExplainEnumInput) (*mcp.CallToolResult, ExplainEnumOutput, error) {
		matches := dict.Lookup(in.Value)
		if len(matches) == 0 {
			msg := fmt.Sprintf("Unknown enum value %q. Known categories: %s. See the schema resources "+
				"(spur://schemas/ip-context, spur://schemas/tag-metadata) for full enumerations.",
				in.Value, strings.Join(dict.Categories(), ", "))
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: msg}},
			}, ExplainEnumOutput{}, nil
		}
		return nil, ExplainEnumOutput{Matches: matches}, nil
	})
}
