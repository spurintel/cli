---
status: accepted
---

# Consolidate the spur-mcp server into the spur CLI

The standalone `spurintel/spur-mcp` server and the `spur` CLI both need the same
Spur Context API client and both ship to the same analysts, so we transplanted the
MCP server into this repository and expose it as the `spur mcp` subcommand (stdio),
backed by the CLI's shared `internal/spur` client, and deprecate the standalone
`spur-mcp` repo. This gives us one binary to build and release, one API client to
maintain, and guaranteed behavioral parity between the CLI and the AI-facing surface.

## Considered Options

- **Keep `spur-mcp` separate.** Rejected: two repos, two release cadences, and a
  duplicated API client that would drift from the CLI's — the exact divergence that
  motivated the consolidation.
- **Import `spur-mcp` as a Go module dependency of the CLI.** Rejected: the shared
  client is the thing both surfaces must agree on, so it belongs in this repo as the
  single source; depending on the old module would invert that and keep the client's
  home in a deprecated repo.

## Consequences

- The MCP server no longer versions independently; its releases are now coupled to
  the CLI's. This is the accepted cost of a single binary and a single client.
- This is a one-way, hard-to-reverse decision once customers are told to register the
  CLI-embedded server (`claude mcp add spur -e SPUR_TOKEN=… -- spur mcp`).
- Deprecation path: the standalone `spur-mcp` repo is archived with a pointer to this
  one, and setup docs guide users to the embedded server.
- `spur mcp` remains stdio-only, matching the old server; HTTP/SSE transport is a
  possible later addition.
