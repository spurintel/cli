# AGENTS.md

Context for AI coding agents (and humans) working in this repository.

## What this is

`spur` is the Spur command-line client (successor to `spurctx`) — a single binary
built from `./cmd/spur`. Commands: `context`, `status`, `tag`, `auth`,
`completion`, `version`, and the embedded `mcp` server.

## Go conventions

This repo follows the [golang-standards/project-layout](https://github.com/golang-standards/project-layout):
`log/slog` with GCP-compatible JSON logging, a thin `main.go` that hands off to
`internal/app` (config + logging live there too), a hexagonal split (ports +
adapters), cobra for the CLI, and minimal external dependencies. Read the
surrounding code before making structural changes.

## Layout

- `cmd/spur/main.go` — thin entry point; hands off to `internal/app.Run(ctx)`.
- `cmd/cleandocs/` — `go generate` helper that regenerates the embedded MCP docs
  and schemas from the Spur documentation source. Run via `go generate ./internal/spur`.
- `internal/app/` — cobra root command and subcommands (`context`, `mcp`, `version`).
- `internal/spur/` — the shared Spur Context API client: HTTP + `Token` header,
  tolerant decoding (`FlexBool`/`FlexNumber`), quota-header parsing, structured
  error classes (unauthorized / forbidden / throttled / bad-request). Also holds
  the MCP server's embedded `schemas/` + `docs/` (`embed.go`) and the `enumdict`
  schema-walker behind `explain_enum`.
- `internal/tools/`, `internal/resources/`, `internal/prompts/` — the MCP surface
  behind `spur mcp`: five tools, schema/doc resources, and the `assess_ip` prompt,
  all wired to the shared `internal/spur` client.
- `internal/buildinfo/` — ldflags-injected version, consumed by `spur version` and the API User-Agent. The User-Agent carries a `surface/<name>` token identifying the request's origin (set via `Client.Surface`); `spur mcp` sets `mcp`, plain CLI commands leave it empty.
- `scripts/mcp-smoke.sh` — end-to-end stdio smoke test for `spur mcp`; run against
  a built binary. `SPUR_LIVE=1` adds authenticated calls.

## Running locally

- Build: `go build ./cmd/spur`
- Test: `go test ./...`
- Auth: the token resolves in precedence order `SPUR_TOKEN` env → config file
  (`token`, top-level or per-profile) → OS keychain. Run `spur auth` to store a
  token in the keychain (hidden-input prompt), `spur auth --logout` to clear it.

## Contributing

- **Branch names** follow `<type>/<short-description>`, e.g. `feat/add-asn-filter`.
  Types: `feat`, `fix`, `chore`, `refactor`, `docs`, `test`.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/):
  `<type>[(scope)]: <description>` — scope optional, subject kept to 72 chars.
- Optional local guardrails live in `.githooks/` (a `commit-msg` and a `pre-push`
  hook enforcing the above). Enable them with `git config core.hooksPath .githooks`;
  bypass any single run with `--no-verify`.
- Before opening a PR, run `go build ./...` and `go test ./...` locally — there is
  no PR CI gate yet. Open the PR against `main` with a description, test evidence,
  and any risk notes (see the PR template).
- Releases are automated: release-please opens a release PR on merge to `main`, and
  tagging `v*` triggers goreleaser to build binaries and update the Homebrew cask.
