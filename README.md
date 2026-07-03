# spur

`spur` is a command-line client for the [Spur](https://spur.us) Context API — the
successor to `spurctx`. This repository builds a single `spur` binary.

> Status: tracer bullet. Today it can look up IP context and report its version;
> `status`, `tag`, `auth`, shell completion, and an embedded MCP server are
> planned (see the parent epic in Linear).

## Build

```
go build ./cmd/spur
```

## Usage

```
export SPUR_TOKEN=<your token>   # get one at https://app.spur.us/settings/api

spur context 8.8.8.8   # print the Spur IP context as JSON
spur version           # print the build version
```

Authentication uses the `SPUR_TOKEN` environment variable. An invalid, forbidden,
or rate-limited token produces a clear, distinct message rather than a stack trace.

## Layout

Follows [Spur Go standards](https://github.com/spurintel/spur-superpowers/blob/main/plugins/spur-superpowers/skills/spur-go-standards/reference.md):
a thin `cmd/spur/main.go` hands off to `internal/app.Run`, which builds the Cobra
command tree. `internal/spur` is the shared Spur API client (lifted from
`spur-mcp`); `internal/buildinfo` holds the ldflags-injected version.
```
cmd/spur/           thin entry point
internal/app/       cobra root + commands (context, version)
internal/spur/      Spur Context API client, types, tolerant decoding, errors
internal/buildinfo/ link-time version metadata
```
