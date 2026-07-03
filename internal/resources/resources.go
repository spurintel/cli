// Package resources registers MCP resources that expose the embedded Spur
// JSON schemas and cleaned Markdown docs to the client. Each resource has a
// stable spur:// URI and a handler that reads from the embedded filesystem.
package resources

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// resourceDef describes a single MCP resource to register.
type resourceDef struct {
	URI         string
	Name        string
	Title       string
	Description string
	MIMEType    string
	// fsys is the embedded filesystem to read from; path is the filename
	// within that filesystem.
	fsys fs.FS
	path string
}

// Register attaches every Spur resource to s. Resources are read from the
// embedded filesystems in the spur package so they travel with the binary.
func Register(s *mcp.Server) error {
	schemasFS, err := fs.Sub(spur.SchemasFS(), "schemas")
	if err != nil {
		return fmt.Errorf("sub schemas fs: %w", err)
	}
	docsFS, err := fs.Sub(spur.DocsFS(), "docs")
	if err != nil {
		return fmt.Errorf("sub docs fs: %w", err)
	}

	defs := []resourceDef{
		{
			URI:         "spur://schemas/ip-context",
			Name:        "ip-context-schema",
			Title:       "Spur IP Context JSON Schema",
			Description: "Full JSON Schema for the IP Context Object returned by GET /v2/context/:ip. Includes descriptions for every field and enum value (risks, services, infrastructure, device types, AI activity, etc.).",
			MIMEType:    "application/json",
			fsys:        schemasFS,
			path:        "ip-context.schema.json",
		},
		{
			URI:         "spur://schemas/tag-metadata",
			Name:        "tag-metadata-schema",
			Title:       "Spur Tag Metadata JSON Schema",
			Description: "Full JSON Schema for the Tag Metadata Object returned by GET /v2/metadata/tags/:tag. Includes descriptions for categories, platforms, protocols, targeting types, and aggregate metrics.",
			MIMEType:    "application/json",
			fsys:        schemasFS,
			path:        "tag-metadata.schema.json",
		},
		{
			URI:         "spur://docs/context-api-overview",
			Name:        "context-api-overview",
			Title:       "Spur Context API Overview",
			Description: "Short overview of the Spur Context API with a quickstart curl example and a sample response.",
			MIMEType:    "text/markdown",
			fsys:        docsFS,
			path:        "context-api-overview.md",
		},
		{
			URI:         "spur://docs/context-api-endpoints",
			Name:        "context-api-endpoints",
			Title:       "Spur Context API Endpoints Reference",
			Description: "Narrative reference for the Context API: base URL, auth, query arguments (dt, mmgeo), response codes, response headers (x-balance-remaining, x-result-dt), endpoint examples, and FAQs.",
			MIMEType:    "text/markdown",
			fsys:        docsFS,
			path:        "context-api-endpoints.md",
		},
		{
			URI:         "spur://docs/context-api-integrations",
			Name:        "context-api-integrations",
			Title:       "Spur Context API Integrations",
			Description: "Available third-party integrations for the Context API (Vertex Project, Anomali, ThreatQuotient, Spiderfoot HX).",
			MIMEType:    "text/markdown",
			fsys:        docsFS,
			path:        "context-api-integrations.md",
		},
		{
			URI:         "spur://docs/analyst-playbook",
			Name:        "analyst-playbook",
			Title:       "Spur Analyst Playbook",
			Description: "Procedure for answering IP-assessment questions with Spur: how to sequence get_ip_context and get_tag_metadata calls, how to disambiguate tunnels[] vs client.proxies[], how to read client.concentration, risk framing for anonymous and callback/malware proxies, and worked examples.",
			MIMEType:    "text/markdown",
			fsys:        docsFS,
			path:        "analyst-playbook.md",
		},
	}

	for _, d := range defs {
		s.AddResource(&mcp.Resource{
			URI:         d.URI,
			Name:        d.Name,
			Title:       d.Title,
			Description: d.Description,
			MIMEType:    d.MIMEType,
		}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			data, err := fs.ReadFile(d.fsys, d.path)
			if err != nil {
				return nil, mcp.ResourceNotFoundError(d.URI)
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      d.URI,
					MIMEType: d.MIMEType,
					Text:     string(data),
				}},
			}, nil
		})
	}
	return nil
}
