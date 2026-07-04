// Package prompts registers MCP prompts that guide the LLM through
// multi-step Spur analyst procedures. Each prompt embeds the relevant
// playbook content so clients that do not auto-fetch resources still get
// the guidance inline.
package prompts

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

const playbookURI = "spur://docs/analyst-playbook"

// Register attaches every Spur prompt to s. It reads the embedded analyst
// playbook once at registration and closes over its bytes so handler calls
// do not re-hit the filesystem.
func Register(s *mcp.Server) error {
	docsFS, err := fs.Sub(spur.DocsFS(), "docs")
	if err != nil {
		return fmt.Errorf("sub docs fs: %w", err)
	}
	playbook, err := fs.ReadFile(docsFS, "analyst-playbook.md")
	if err != nil {
		return fmt.Errorf("read analyst-playbook.md: %w", err)
	}
	playbookText := string(playbook)

	s.AddPrompt(&mcp.Prompt{
		Name:  "assess_ip",
		Title: "Assess an IP address with Spur",
		Description: "Runs the Spur analyst playbook against an IP: fetches context with get_ip_context, " +
			"then fans out to get_tag_metadata for every tunnel operator, client proxy, and _USER behavior, " +
			"and synthesizes a structured assessment with risk framing.",
		Arguments: []*mcp.PromptArgument{
			{Name: "ip", Description: "IPv4 or IPv6 address to assess.", Required: true},
			{Name: "dt", Description: "Optional YYYYmmdd for historical lookup (Enterprise only).", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		instruction, err := assessIPInstruction(req.Params.Arguments)
		if err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "Spur IP assessment",
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: instruction},
				},
				{
					Role: "user",
					Content: &mcp.EmbeddedResource{
						Resource: &mcp.ResourceContents{
							URI:      playbookURI,
							MIMEType: "text/markdown",
							Text:     playbookText,
						},
					},
				},
			},
		}, nil
	})
	return nil
}

func assessIPInstruction(arguments map[string]string) (string, error) {
	ip, err := spur.ValidateIP(arguments["ip"])
	if err != nil {
		return "", fmt.Errorf("assess_ip: %w", err)
	}
	dt, err := spur.ValidateHistoricalDate(arguments["dt"])
	if err != nil {
		return "", fmt.Errorf("assess_ip: %w", err)
	}
	return fmt.Sprintf(
		"Follow the Spur analyst playbook (embedded below) to assess IP %s%s"+
			"Start with get_ip_context, then call get_tag_metadata in parallel for every "+
			"tunnels[].operator, client.proxies[] entry, and client.behaviors value matching "+
			"^*_USER$ (stripping the _USER suffix to recover the tag). Do not skip any operator. "+
			"Distinguish tunnels[] (what the IP IS) from client.proxies[] (what clients ON the IP "+
			"are using) in your answer. Lead with risk framing whenever the IP involves anonymous "+
			"operators or CALLBACK_PROXY. "+
			"FIDELITY: report only fields Spur actually returned. Do NOT fabricate values for "+
			"absent fields (e.g. do not invent an infrastructure value if the field is missing). "+
			"Do NOT invent risk enums from derived signals — only cite AD_FRAUD, GEO_MISMATCH, "+
			"CALLBACK_PROXY, TUNNEL, LOGIN_BRUTEFORCE, or WEB_SCRAPING if they appear verbatim "+
			"in risks[]. Do NOT extrapolate customer use-cases (ad fraud, data quality, etc.) "+
			"that Spur did not flag.",
		ip, dtSuffix(dt),
	), nil
}

func dtSuffix(dt string) string {
	if dt == "" {
		return ". "
	}
	return " for date " + dt + ". "
}
