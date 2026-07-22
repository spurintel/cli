package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/buildinfo"
	"github.com/spurintel/cli/internal/prompts"
	"github.com/spurintel/cli/internal/resources"
	"github.com/spurintel/cli/internal/spur"
	"github.com/spurintel/cli/internal/spur/enumdict"
	"github.com/spurintel/cli/internal/tools"
)

// serverName is the MCP server identity reported in the initialize response.
const serverName = "spur-context"

// serverInstructions is returned in the MCP initialize response and many
// clients splice it into the system prompt. It points the LLM at the analyst
// playbook resource for any IP-assessment question.
const serverInstructions = `When the user asks what Spur thinks about an IP, or whether an IP is a proxy/VPN/tunnel, follow this procedure. Full details: resource spur://docs/analyst-playbook.

PROCEDURE
1. Call get_ip_context(ip). Also pass dt=YYYYmmdd if the user specified a date.
2. Collect every service tag from the response: every tunnels[].operator, every client.proxies[] entry, and every client.behaviors value matching ^*_USER$ (strip the _USER suffix to recover the tag).
3. Call get_tag_metadata IN PARALLEL for every tag collected in step 2. Do not skip any.
4. Call format_assessment with: the get_ip_context result as "context", a map of tag->get_tag_metadata result as "tags", a one-sentence "bottom_line" (classification + risk framing if warranted), and a plain-English "assessment" paragraph. The tool validates your prose against fidelity rules and returns the final markdown ready to send to the user. Send the returned markdown verbatim — do NOT rewrite, reorder, or re-format the sections it produced.

Use format_assessment as the ONLY path to final output. Do not hand-author the six-section markdown yourself — the tool renders it deterministically so facts stay faithful. Your prose contributions are just the one-sentence bottom_line and the analyst-style assessment paragraph.

KEY SEMANTIC RULES (the tool will reject violations)
- Never cite a risk enum (AD_FRAUD, CALLBACK_PROXY, GEO_MISMATCH, LOGIN_BRUTEFORCE, TUNNEL, WEB_SCRAPING) or its paraphrase ("geo mismatch", "ad fraud", etc.) in bottom_line/assessment unless it appears in context.risks[]. High skew is NOT GEO_MISMATCH.
- Never conflate tunnels[] (what the IP IS) with client.proxies[] (what clients ON the IP are using). If asked "is this IP a proxy?" the answer lives in tunnels[].
- Never extrapolate customer use-cases (ad-fraud applicability, data quality, ATO, bot detection, etc.) that Spur did not flag via a risk enum.
- When any tunnels[].anonymous=true or any tag has isAnonymous=true, lead bottom_line with "Treat with elevated scrutiny — <reason>". When CALLBACK_PROXY is in risks OR any tag has isCallbackProxy=true, lead with "Treat with extra caution — <reason>" instead.
- Density is a 0-1 decimal. Do not convert to a percentage.
- A tunnels[] entry with an empty operator (or only a type) still means the IP IS a tunnel. Describe it as "tunnel with unattributed protocol" or "unbranded VPN appliance" — do not invent an operator name, and do not downgrade it to a client-side signal.
- When tunnels[] and client.proxies[] disagree (e.g. IP is one VPN, clients on it use another proxy service), say so plainly: the IP is the tunnel; separately, users on it run a proxy SDK. Don't merge them into one claim.
- Calibrate confidence in the assessment: lower it when operator attribution is missing, when infrastructure/services are absent, or when only a single weak signal is present; raise it when tunnels, services, category, and client signals agree.`

// newMCPCmd builds `spur mcp`: a stdio Model Context Protocol server that
// exposes the Spur Context API as MCP tools, resources, and prompts, all backed
// by the same shared client the rest of the CLI uses. MCP clients launch it via
// `claude mcp add spur -e SPUR_TOKEN=… -- spur mcp`, so the token is read from
// the environment.
func newMCPCmd(cf *configFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the Spur Model Context Protocol server (stdio)",
		Long: "Run a stdio Model Context Protocol (MCP) server exposing the Spur Context " +
			"API to AI clients such as Claude Desktop.\n\n" +
			"It serves the get_ip_context (single or batch), get_tag_metadata, " +
			"get_api_status, explain_enum, and format_assessment tools; the feed tools " +
			"list_feeds, feed_status, and feed_download; the bounded export tool; the " +
			"Spur schema/doc resources; and the assess_ip prompt — all backed by the " +
			"same client as the `spur` commands.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain. Clients typically pass the token in the environment:\n" +
			"  claude mcp add spur -e SPUR_TOKEN=… -- spur mcp",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken(cf.resolved.Token)
			if err != nil {
				return err
			}
			client := cf.newClient(token)
			client.Surface = "mcp"
			return runMCPServer(cmd, client)
		},
	}
}

// runMCPServer constructs the MCP server, registers every Spur tool/resource/
// prompt against client, and serves stdio until the command context is
// cancelled (Ctrl-C or the client closing the transport).
func runMCPServer(cmd *cobra.Command, client *spur.Client) error {
	schemasSub, err := fs.Sub(spur.SchemasFS(), "schemas")
	if err != nil {
		return fmt.Errorf("open embedded schemas: %w", err)
	}
	dict, err := enumdict.Build(schemasSub)
	if err != nil {
		return fmt.Errorf("build enum dictionary: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: mcpVersionString(),
	}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})

	tools.Register(server, client, dict)
	if err := resources.Register(server); err != nil {
		return fmt.Errorf("register resources: %w", err)
	}
	if err := prompts.Register(server); err != nil {
		return fmt.Errorf("register prompts: %w", err)
	}

	if err := server.Run(cmd.Context(), &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server run: %w", err)
	}
	return nil
}

// mcpVersionString returns the serverInfo.version reported in the MCP
// initialize response. It bakes in the build stamp plus a short hash of the
// analyst playbook content so a client can tell whether both the binary and the
// embedded docs are fresh.
func mcpVersionString() string {
	base := fmt.Sprintf("%s+%s", buildinfo.Version, buildinfo.BuildStamp)
	docs, err := fs.Sub(spur.DocsFS(), "docs")
	if err != nil {
		return base
	}
	playbook, err := fs.ReadFile(docs, "analyst-playbook.md")
	if err != nil {
		return base
	}
	sum := sha256.Sum256(playbook)
	return fmt.Sprintf("%s+playbook.%s", base, hex.EncodeToString(sum[:])[:8])
}
