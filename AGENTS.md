# AGENTS.md

Context for AI coding agents working in this repository.

## Who we are

Spur Intelligence Corporation. Small eng team. Production systems that require ISO 27001 and SOC 2 alignment.

## Our merge-to-production process

Every code change follows this path. The process is enforced by GitHub branch protection, CODEOWNERS, and required status checks — not by convention.

1. **Branch from `main`** (or `develop` in a grandfathered legacy repo — see repo README).
2. **Name the branch** exactly `<type>/<linear-id>` (e.g. `feat/dev-123`). Types: `feat`, `fix`, `chore`, `refactor`, `docs`, `test`, `sec`, `hotfix`. Linear ID is required and lowercase by default; legacy uppercase (`feat/DEV-123`) still validates. **No description segment.** The Linear ticket holds the title; do not duplicate it in the branch. Personal prefixes are forbidden.
3. **Commit** using `<type>(<linear-id>): <description>` (Conventional Commits with the Linear ID as the scope, e.g. `feat(dev-123): add asn filter`). Type must match branch prefix (`sec/` and `hotfix/` map to `chore:` and `fix:` respectively). Either case in the scope is accepted to match the branch. Local `commit-msg` and `pre-push` hooks (installed by `spur-bootstrap`) enforce this on every commit/push, not just when Claude is composing.
4. **Open a PR** with: a `<type>(<linear-id>): <description>` title (Linear ID lives in the title, not the body), a completed template (Context / Test evidence / Risk), target `main` (or `develop` for legacy).
5. **CI must pass** — branch name check, lint, unit/integration tests, build, Dependabot, secret scanning.
6. **Get CODEOWNER approval from someone other than the author.** Self-approval is prohibited.
7. **Squash or rebase merge.** Merge commits are disabled.
8. Most services deploy automatically on merge to `main`.

## Hotfix / break-glass (§6)

Only during active SEV-1 incidents. Branch prefix `hotfix/`. A repo admin bypasses branch protection to merge. Within 24 hours, open a retrospective PR and file a post-mortem in the engineering post-mortem repo.

## The `spur-superpowers` plugin

If you are Claude Code, the [`spur-superpowers`](https://github.com/spurintel/spur-superpowers) plugin provides skills that automate most of this process:

- **spur-branch-name** — validate/suggest branch names.
- **spur-commit-message** — format Conventional Commits matching the branch prefix.
- **spur-pr-description** — seed the PR body.
- **spur-pre-merge-check** — run the pre-merge checklist.
- **spur-codeowners** — resolve required approvers.
- **spur-hotfix** / **spur-retrospective-pr** — break-glass and its follow-up.
- **spur-repo-onboarding** — audit / install these controls in a repo.
- **spur-process-audit** — full compliance report on a PR or repo.
- **spur-linear-link** — pull Linear ticket data.

Prefer these skills over hand-writing regex, GitHub Actions, or CODEOWNERS logic.

## Repo-specific context

<!-- Add anything agent-relevant about THIS repo below: architecture, quirks, where to find config, how to run tests locally, etc. Keep it tight. -->

`spur` is the Spur command-line client (successor to `spurctx`) — a single binary
built from `./cmd/spur`. Tracer-bullet status: `context` and `version` today;
`status`, `tag`, `auth`, completion, and an embedded MCP server are planned.

### Go conventions

This repo follows [Spur Go standards](https://github.com/spurintel/spur-superpowers/blob/main/plugins/spur-superpowers/skills/spur-go-standards/reference.md) — golang-standards/project-layout, `log/slog` with GCP-compatible JSON, thin `main.go` into `internal/app` (config + logging live there too), hexagonal (ports + adapters), cobra for CLIs, Pulumi (Go) + Ansible in `deployments/`, minimal external dependencies.

Read the reference before making structural changes. The `spur-go-standards` skill in `spur-superpowers` will audit deviations.

### Layout

- `cmd/spur/main.go` — thin entry point; hands off to `internal/app.Run(ctx)`.
- `internal/app/` — cobra root command and subcommands (`context`, `version`).
- `internal/spur/` — the shared Spur Context API client, lifted from `spur-mcp`:
  HTTP + `Token` header, tolerant decoding (`FlexBool`/`FlexNumber`), quota-header
  parsing, structured error classes (unauthorized / forbidden / throttled / bad-request).
- `internal/buildinfo/` — ldflags-injected version, consumed by `spur version` and the API User-Agent.

### Running locally

- Build: `go build ./cmd/spur`
- Test: `go test ./...`
- Auth: export `SPUR_TOKEN` (config-file / keychain resolution is a later ticket).

## Canonical process document

The full merge-to-production process lives in the Spur Engineering Wiki at `docs/conventions/merge-to-production.md`. Use the **spur-wiki** skill to fetch the current version — it resolves the local checkout and pulls the latest automatically. This AGENTS.md is a summary for quick reference — when they disagree, trust the canonical doc.
