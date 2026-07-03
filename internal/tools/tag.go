package tools

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// GetTagMetadataInput is the argument object for get_tag_metadata.
type GetTagMetadataInput struct {
	Tag string `json:"tag" jsonschema:"Service tag to look up, e.g. OXYLABS_PROXY, PROTON_VPN, TOR_PROXY."`
}

// GetTagMetadataOutput wraps the parsed tag metadata body.
type GetTagMetadataOutput struct {
	Tag spur.TagMetadata `json:"tag" jsonschema:"Tag Metadata object. See resource spur://schemas/tag-metadata for the full field reference."`
}

// flexTypeSchemas maps the Flex* helper types to the JSON schema shape they
// actually marshal as. Without this, schema inference sees the underlying Go
// types (bool / string) and the SDK rejects live responses because the Spur
// API sometimes returns bare numbers for FlexNumber fields.
func flexTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[spur.FlexBool]():   {Type: "boolean"},
		reflect.TypeFor[spur.FlexNumber](): {Type: "number"},
	}
}

// registerGetTagMetadata wires the get_tag_metadata tool onto the server.
func registerGetTagMetadata(s *mcp.Server, c *spur.Client) {
	opts := &jsonschema.ForOptions{TypeSchemas: flexTypeSchemas()}
	in, err := jsonschema.For[GetTagMetadataInput](opts)
	if err != nil {
		panic(fmt.Errorf("get_tag_metadata input schema: %w", err))
	}
	out, err := jsonschema.For[GetTagMetadataOutput](opts)
	if err != nil {
		panic(fmt.Errorf("get_tag_metadata output schema: %w", err))
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_tag_metadata",
		Title: "Look up tag metadata",
		Description: "Retrieves metrics and metadata for a Spur service tag (proxy/VPN brand). " +
			"Useful when an IP's risks or services include a tag and you want to know " +
			"about the provider (categories, anonymity, platforms, protocols, " +
			"aggregate metrics). See resource spur://schemas/tag-metadata for the full field reference.",
		InputSchema:  in,
		OutputSchema: out,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetTagMetadataInput) (*mcp.CallToolResult, GetTagMetadataOutput, error) {
		body, err := c.GetTagMetadata(ctx, in.Tag)
		if err != nil {
			return toolError(err, statusHint(err)), GetTagMetadataOutput{}, nil
		}
		return nil, GetTagMetadataOutput{Tag: body}, nil
	})
}
