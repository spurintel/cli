# spur

The official command-line client for [Spur](https://spur.us) — IP context lookups, service-tag intelligence, data feeds, and filtered exports from your terminal, plus a built-in [Model Context Protocol](https://modelcontextprotocol.io) server that puts the same data in Claude and other AI clients.

`spur` is the successor to `spurctx` — see [Migrating from spurctx](#migrating-from-spurctx).

## Quick start

```bash
# 1. Install
brew install spurintel/tap/spur

# 2. Authenticate (get a token at https://app.spur.us/settings/api)
spur auth

# 3. Look up an IP
spur context 1.2.3.4
```

You get a styled, human-readable report at the terminal and newline-delimited JSON when you pipe it — so it drops straight into scripts:

```bash
spur context 1.2.3.4 | jq .as.organization
```

Using Claude? Register the built-in MCP server and ask it about IPs directly:

```bash
claude mcp add spur -e SPUR_TOKEN=your-token-here -- spur mcp
```

See [MCP server (Claude Desktop and friends)](#mcp-server-claude-desktop-and-friends) for Claude Desktop and other clients.

## Demos

| Authenticate & check status | Token status |
| - | - |
| ![spur auth demo](docs/demos/auth.gif) | ![spur status demo](docs/demos/status.gif) |

| Look up an IP (`context`) | Inspect a service tag (`tag`) |
| - | - |
| ![spur context demo](docs/demos/context.gif) | ![spur tag demo](docs/demos/tag.gif) |

<sub>GIFs are generated with [VHS](https://github.com/charmbracelet/vhs) from the tapes in [`docs/demos/`](docs/demos/) — run `vhs docs/demos/context.tape` to re-record.</sub>

## Install

**Homebrew** (macOS and Linux; bundles bash/zsh/fish completions):

```bash
brew install spurintel/tap/spur
```

**Go** (requires Go 1.25+):

```bash
go install github.com/spurintel/cli/cmd/spur@latest
```

**Prebuilt binaries** for linux/darwin/windows on amd64/arm64, with a `checksums.txt`, are on the [releases page](https://github.com/spurintel/cli/releases/latest).

**From source:**

```bash
git clone https://github.com/spurintel/cli
cd cli
make build   # builds bin/spur
```

## Authentication

Every command that talks to the Spur API needs a token from <https://app.spur.us/settings/api>. `spur` resolves it from three places, highest precedence first:

1. The `SPUR_TOKEN` environment variable — for CI and existing setups.
2. The [config file](#configuration) — for per-profile tokens.
3. The OS keychain — for interactive use, stored by `spur auth`.

For day-to-day use, store it once in your keychain; the prompt hides your input and nothing lands in shell history:

```bash
spur auth            # prompts for the token, stores it securely
spur auth --logout   # removes the stored token
```

Then confirm your access is healthy:

```bash
spur status          # active flag, queries remaining, service tier
```

An invalid, forbidden, or rate-limited token produces a clear, actionable error — not a stack trace.

## Commands

```text
spur
	context        Look up Spur context for one or more IPs
	status         Report token status: active, queries remaining, tier
	tag            Look up metadata for a service tag (proxy/VPN brand)
	feed list      List the supported data-feed types
	feed status    Show a feed's current metadata (date, timestamps, size)
	feed download  Download a feed's data file
	export         Export a filtered subset of a feed (Exports API)
	mcp            Run the Model Context Protocol server (stdio)
	auth           Store or remove your token in the OS keychain
	completion     Generate a shell completion script
	version        Print the build version
```

Run `spur <command> --help` for the full flag reference on any command.

### `spur context` — IP lookups

Pass one IP, several, or pipe them in (newline, whitespace, or comma separated). Lookups run concurrently and render one report per IP, in input order.

```bash
spur context 1.2.3.4
spur context 1.2.3.4 5.6.7.8 9.10.11.12
cat ips.txt | spur context
```

Pulling IPs out of messy input — log lines, alert payloads, email bodies — is built in with `--extract` (the old `spurctx` "garbage" mode):

```bash
grep 'auth failure' /var/log/auth.log | spur context --extract
```

Reproduce what Spur reported on a given day (Enterprise subscriptions):

```bash
spur context 1.2.3.4 --date 20260601
```

Tune concurrency for very large batches with `--parallelism` (default 36).

### `spur status` — token health

```bash
spur status
```

Reports whether your token is active, the queries remaining in the current billing cycle, and your service tier.

### `spur tag` — service-tag intelligence

Tags like `OXYLABS_PROXY` or `PROTON_VPN` show up in context results; `spur tag` tells you what they mean — categories, whether the service anonymizes traffic, platforms, protocols, and aggregate metrics across observed endpoints.

```bash
spur tag OXYLABS_PROXY
```

### `spur feed` — bulk data feeds

Feeds deliver Spur's bulk intelligence as newline-delimited JSON (gzip); some are also offered as a MaxMind DB (`.mmdb`).

```bash
spur feed list                        # the feed types you can pass
spur feed status anonymous            # current file's date, generation time, size
spur feed download anonymous          # stream the feed; decompressed NDJSON on stdout
spur feed download anonymous -o anonymous.json.gz   # raw gzip to a file
spur feed download ipgeo -o ipgeo.mmdb              # ipgeo defaults to MMDB
```

Useful flags: `--date YYYYMMDD` for a historical release, `--ipv6` and `--realtime` for a feed's variants, `--mmdb`/`--json` to pick the artifact, `--decompress` to expand the gzip when writing to a file. Download progress goes to stderr, so stdout stays a clean data channel.

### `spur export` — filtered exports

Where `feed download` fetches a whole feed, `export` asks the Exports API for a dynamically filtered and reformatted subset. Feeds: `anonymous`, `anonymous-residential`, `ztna`, `dch`, `ipgeo` (add `--ipv6` where offered).

```bash
# US-only anonymous-residential IPs as CSV with a header
spur export anonymous-residential --ccs US --csv-header -o us-resi.csv

# Everything Spur tags as NORD_VPN, as JSON
spur export anonymous --tags NORD_VPN --format json

# What changed since yesterday
spur export anonymous --diff
```

Filters include `--ccs`, `--asns`, `--tags`, `--attributed`, `--callback-proxies`, `--fields`, `--limit`, and `--diff`/`--diff-date`; a filter the chosen feed doesn't accept is rejected before any request is made. Here `--format` selects the *server's* output format (`csv`, `json`, or `mmdb` — mmdb is binary and must go to a file via `-o`). The Exports API is rate-limited per token; a 429 is reported clearly so you can back off and retry.

### `spur completion` — shell completion

Homebrew installs completions automatically. Otherwise, load them into the current session:

```bash
source <(spur completion bash)   # bash
source <(spur completion zsh)    # zsh
spur completion fish | source    # fish
```

To persist, write the script to your shell's completion directory.

### `spur version`

```bash
spur version
```

Reports the build you're running — include it in bug reports.

## Output formats

Every read command picks its format by where output is going: **styled text at a terminal, newline-delimited JSON when piped or redirected**. `--format text|json|csv` always overrides the auto-choice.

```bash
spur context 1.2.3.4                  # styled report
spur context 1.2.3.4 | jq .           # NDJSON, no TTY detected
spur context 1.2.3.4 --format csv -o out.csv
```

Color is suppressed automatically for non-terminals, with `--no-color`, or via the [`NO_COLOR`](https://no-color.org) environment variable. `-o <file>` writes anywhere stdout would.

## Configuration

No config file is needed for normal use — the CLI talks to production Spur endpoints out of the box. Create one for endpoint overrides or named profiles:

```text
$XDG_CONFIG_HOME/spur/config.toml    (default: ~/.config/spur/config.toml)
```

```toml
# optional: token consulted between SPUR_TOKEN and the keychain
token = "..."

[endpoints]
api = "https://api.spur.us"
feeds = "https://feeds.spur.us"
exports = "https://exports.spur.us"

[profiles.staging]
token = "..."
[profiles.staging.endpoints]
api = "https://staging-api.example.com"
```

```bash
spur --profile staging status    # apply a named profile
spur --config ./other.toml status
```

A selected profile's settings win over the top-level ones; anything unset falls back to the defaults. If the file stores a token, keep it private: `chmod 600 ~/.config/spur/config.toml` (the CLI warns if it's looser).

## MCP server (Claude Desktop and friends)

`spur mcp` runs a stdio Model Context Protocol server backed by the same client as the CLI commands, so an AI assistant can look up IPs, inspect tags, check quota, and pull feed data with live Spur intelligence.

> Previously used the standalone `spur-mcp` server? This replaces it — one binary, same tools. Re-register with the commands below; the old repo is deprecated.

### Claude Code

```bash
claude mcp add spur -e SPUR_TOKEN=your-token-here -- spur mcp
```

### Claude Desktop

Add the server to `claude_desktop_config.json` — on macOS at `~/Library/Application Support/Claude/claude_desktop_config.json`, on Windows at `%APPDATA%\Claude\claude_desktop_config.json`:

```json
{
	"mcpServers": {
		"spur": {
			"command": "spur",
			"args": ["mcp"],
			"env": { "SPUR_TOKEN": "your-token-here" }
		}
	}
}
```

Use an absolute path for `command` if `spur` isn't on the launcher's `PATH` (e.g. `/opt/homebrew/bin/spur`). The server resolves the token like every other command — `SPUR_TOKEN`, config file, then keychain — so if you've run `spur auth` you can drop the `env` block entirely. Fully quit and reopen Claude Desktop after editing the config.

Other MCP clients (Cursor, Codex CLI, LM Studio, …) work the same way: command `spur`, argument `mcp`, token in the environment.

### What the server exposes

| Tool | Purpose |
| - | - |
| `get_ip_context` | IP context for one address or a batch (risks, services, tunnels, geo, AS) |
| `get_tag_metadata` | Service-tag metadata: categories, anonymity, platforms, metrics |
| `get_api_status` | Token status: active flag, queries remaining, tier |
| `explain_enum` | Offline, schema-backed lookup of Context API enum values |
| `format_assessment` | Renders a validated, customer-facing assessment from lookup results |
| `list_feeds` / `feed_status` / `feed_download` | Feed catalog, metadata, and bounded downloads |
| `export` | Bounded filtered exports from the Exports API |

It also serves the Spur JSON schemas and API docs as resources, and an `assess_ip` prompt that runs the analyst playbook end-to-end for one IP.

## Migrating from spurctx

`spur` replaces `spurctx`. One name everywhere now — Homebrew and `go install` both produce a binary called `spur` — and lookups move from single-letter flags to a subcommand:

| Before | After |
| - | - |
| `spurctx -ip 1.2.3.4,5.6.7.8` | `spur context 1.2.3.4 5.6.7.8` |
| `spurctx -f ips.txt` | `spur context < ips.txt` |
| `echo 1.1.1.1 \| spurctx` | `echo 1.1.1.1 \| spur context` |
| `spurctx -g` (garbage mode) | `spur context --extract` |
| `spurctx -gf messy.txt` | `spur context --extract < messy.txt` |
| `spurctx -n 50` | `spur context --parallelism 50` |
| `brew install spurintel/spurintel/spurctx` | `brew install spurintel/tap/spur` |
| `go install github.com/spurintel/spurctx-cli@latest` | `go install github.com/spurintel/cli/cmd/spur@latest` |

Your pipelines keep working: piped output is still JSON (one object per IP), and `SPUR_TOKEN` is still honored. What's new on top: styled terminal output, `status`/`tag`/`feed`/`export`, secure token storage with `spur auth`, config profiles, shell completion, and the MCP server.

## Development

```bash
make build   # bin/spur
make test    # go test ./...
```

The layout follows the [Spur Go standards](https://github.com/spurintel/spur-superpowers/blob/main/plugins/spur-superpowers/skills/spur-go-standards/reference.md): a thin `cmd/spur/main.go` hands off to `internal/app`, which builds the Cobra command tree over the shared `internal/spur` API client. Architectural decisions live in [`docs/adr/`](docs/adr/).

## License

See [LICENSE](LICENSE).
