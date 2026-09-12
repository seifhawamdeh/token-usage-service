# Token usage service

One-shot **Go** CLI that ingests **provider-reported** agent burn facts from local on-disk stores into **PostgreSQL**. Runs under an external scheduler (systemd timer or cron). No resident daemon; no USD rate tables.

| Doc | Role |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | As-built design, locked decisions, schema, package layout |
| [docs/provider-usage-semantics.md](docs/provider-usage-semantics.md) | Per-vendor accounting contract (how tokens are counted) |
| [docs/prd-local-burn-ingest.md](docs/prd-local-burn-ingest.md) | Original product requirements + acceptance status |
| [deploy/systemd/](deploy/systemd/) | Hourly user timer units |

## Quick start

```bash
cp .env.example .env          # set DATABASE_URL, HOST_ID
go run ./cmd/ingest migrate
go run ./cmd/ingest validate-config
go run ./cmd/ingest ingest
# second run → mostly unchanged_skipped
```

Or use the installed binary (if built):

```bash
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest
token-usage-ingest ingest
```

## Commands

| Command | Purpose |
| --- | --- |
| `migrate` | Apply embedded Postgres migrations |
| `validate-config` | Check env + DB connectivity |
| `ingest [--force]` | One ingest pass (`--force` bypasses watermarks) |

## Adapters (`ENABLED_VENDORS`)

| Vendor | Source of truth | Notes |
| --- | --- | --- |
| `claude-code` | `~/.claude/projects/**/*.jsonl` | Sum assistant `message.usage` |
| `codex` | `~/.codex/sessions/**/rollout-*.jsonl` | Last `thread_token_usage` |
| `opencode` | `~/.local/share/opencode/opencode.db` | Session `tokens_*` row |
| `github-copilot` | `~/.copilot/session-store.db` | Sum events; date-aware billing regime |
| `antigravity` | `~/.gemini/antigravity-cli/conversations/*.db` | `gen_metadata` protobuf + self-check |
| `gemini` | `~/.gemini/tmp/**/session-*` | CLI only; no-op if absent |
| `cursor` | `~/.config/Cursor/.../state.vscdb` | **Sessions only** (composers); tokens left null |
| `cursor-usage` | `cursor-usage-reports/*.csv` | **Token usage** from Cursor usage-events export |

## Schema (Postgres)

- `burn_snapshots` — latest per-source usage (**nullable** tokens: missing ≠ zero)
- `burn_checkpoints` — watermarks (`mtime_ns`, `size_bytes`, processing signature)
- `path_remotes` — **separate** table mapping full `source_path` → Git remote

## Accounting

**Per-source snapshots only.** Machine-wide sums may double-count copied, branched, or resumed transcripts. Cross-source dedup is deferred. Never invent USD from rates.

## Scheduling

See [deploy/systemd/README.md](deploy/systemd/README.md).

## Deferred

- Git remote sink
- Exact machine-wide deduplicated totals
- Cursor session ↔ usage-event join
- Analytics UI / cost engines
