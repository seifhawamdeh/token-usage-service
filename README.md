# Token Usage Service

[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)](https://www.postgresql.org/)

**Your local AI usage, in PostgreSQL.**

A Go CLI that collects provider-reported usage from agent transcripts, local databases, and Cursor CSV exports. Run it manually or on a schedule to keep queryable snapshots of model usage, token counts, and source metadata.

[Quick start](#quick-start) · [Supported sources](#supported-sources) · [Dashboard](#dashboard) · [Configuration](#configuration) · [Scheduling](#scheduling) · [Documentation](#documentation)

## How it works

```text
Discover enabled sources → Compare checkpoints → Parse changed sources → Upsert → Exit
```

Each invocation completes one pass. Snapshots and checkpoints live in PostgreSQL; an external scheduler controls the next run. Discovery work varies by adapter—database and CSV adapters may read contents while discovering sources.

Missing token values remain `null`. Vendors may embed `provider_cost`; optional **list-price** USD is computed in `v_burn_usage_rated` from the `model_costs` catalog — see [cost and accuracy](docs/cost-and-accuracy.md).

## Quick start

Prerequisites: **Go 1.25+**, a reachable **PostgreSQL database**, and local data from a supported source. Default source paths follow Linux conventions; overrides are available.

### 1. Get the project

```bash
git clone https://github.com/seifhawamdeh/token-usage-service.git
cd token-usage-service
cp .env.example .env
```

### 2. Configure your database and sources

Edit `.env`. Point `DATABASE_URL` at an existing database, choose a stable machine ID, and enable the adapters you want.

```dotenv
DATABASE_URL=postgres://USER:PASSWORD@localhost:5432/token_usage?sslmode=disable
HOST_ID=laptop-main
ENABLED_VENDORS=claude-code,codex
SINK=postgres
```

This URL is a local-development placeholder. Use your database's required TLS settings for remote connections. Keep `HOST_ID` stable: changing it creates new source identities.

### 3. Initialize and ingest

```bash
go run ./cmd/ingest validate-config
go run ./cmd/ingest migrate
go run ./cmd/ingest ingest
```

Validation checks required configuration and database connectivity. Migration creates or updates the schema inside your existing database.

An illustrative unchanged run:

```text
scanned=12 unchanged_skipped=12 parsed=0 upserted=0 deferred=0 errors=0 path_remotes=0
```

## Dashboard

Local read-only UI over Postgres (`v_burn_usage_rated` + snapshots):

```bash
go run ./cmd/dashboard
# open http://127.0.0.1:8080
```

Optional: `DASHBOARD_ADDR=:9090`. Uses the same `DATABASE_URL` / `.env` as ingest.

## Supported sources

| Adapter | Default source | Usage behavior |
| --- | --- | --- |
| `claude-code` | `~/.claude/projects/**/*.jsonl` | Dedupes streamed-chunk lines by `message.id`, then sums usage |
| `codex` | `~/.codex/sessions/**/rollout-*.jsonl` | Sums the peak `total_token_usage` of each compaction segment; input excludes cache overlap |
| `opencode` | `~/.local/share/opencode/opencode.db` | Reads session token fields |
| `github-copilot` | `~/.copilot/session-store.db` | Aggregates events with date-aware billing semantics |
| `gemini` | `~/.gemini/tmp/**/session-*` | Reads local CLI session usage |
| `antigravity` | `~/.gemini/antigravity-cli/conversations/*.db` | Extracts usage from `gen_metadata` protobuf data |
| `cursor` | `~/.config/Cursor/User/globalStorage/state.vscdb` | Inventories composer sessions; tokens remain `null` |
| `cursor-usage` | `cursor-usage-reports/*.csv` | Imports dashboard usage-event exports |

See [provider usage semantics](docs/provider-usage-semantics.md) for accounting rules and format limitations. Adapter availability does not guarantee every provider version exposes complete usage data.

### Cursor: sessions and usage are separate

The `cursor` adapter reads local session metadata. The `cursor-usage` adapter reads manually downloaded dashboard exports; it does not fetch account usage automatically.

1. Export usage events from your Cursor dashboard.
2. Save the CSV in `cursor-usage-reports/`, or set `CURSOR_USAGE_REPORTS_DIR`.
3. Add `cursor-usage` to `ENABLED_VENDORS` and run ingest.

Exports describe account activity, not necessarily activity on this machine. They are not joined to composer sessions. Overlapping exports under different filenames can double-count events; avoid importing duplicate reporting periods.

The [example CSV](cursor-usage-reports/usage-events.example.csv) documents expected columns. Keep personal exports outside version control.

## Configuration

The CLI loads `.env` from its current working directory. Existing environment variables take precedence.

| Variable | Purpose | Default when unset |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL connection string | Required for PostgreSQL |
| `HOST_ID` | Stable machine identity | Falls back to `HOST`; otherwise required |
| `ENABLED_VENDORS` | Comma-separated adapter names | `claude-code` |
| `SINK` | Storage backend | `postgres`, the only implemented sink |
| `CURSOR_USAGE_REPORTS_DIR` | Directory containing exports | `cursor-usage-reports`, relative to the working directory |

The supplied `.env.example` enables all adapters; narrow that list during setup. Source-path overrides and the Copilot legacy-billing switch are listed in [`.env.example`](.env.example).

## Commands

| Command | Action |
| --- | --- |
| `go run ./cmd/ingest validate-config` | Check configuration and database connectivity |
| `go run ./cmd/ingest migrate` | Apply embedded schema migrations |
| `go run ./cmd/ingest ingest` | Run one ingestion pass |
| `go run ./cmd/ingest ingest --force` | Reprocess sources regardless of checkpoints |

To install a local executable:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/token-usage-ingest ./cmd/ingest
~/.local/bin/token-usage-ingest ingest
```

Run from the directory containing `.env`, or supply configuration through environment variables.

The summary reports scanned, unchanged, parsed, upserted, deferred, and failed sources. Per-source parse errors can still produce exit code `0`; inspect `errors` and `deferred` when checking coverage. Configuration errors return `2`; database and fatal ingestion failures return `3`.

## Storage and accounting

| Table | Contents |
| --- | --- |
| `burn_snapshots` | Latest per-source usage, model metadata, nullable token counts, and optional reported cost |
| `burn_checkpoints` | Source comparison values and processing signature |
| `path_remotes` | Source paths, repository roots, and discovered Git remotes |
| `model_costs` | USD-per-1M-token rate card by `model_key` with dated `[effective_from, effective_to)` periods |
| `model_cost_aliases` | Vendor/display labels → `model_key` |

Query **`v_burn_usage_rated`** for snapshots plus `rated_cost_usd`. Details: catalog periods, Anthropic 5m/1h cache windows, long-context tiers, and accuracy bands — [cost and accuracy](docs/cost-and-accuracy.md).

Source identity includes the host, adapter, and source path. Reprocessing updates the existing snapshot for that identity. Adapter and schema versions participate in checkpoint comparison.

**Snapshots represent individual sources, not exact deduplicated machine-wide totals.** Copies, branches, resumed files, and overlapping exports can repeat usage. Cross-source deduplication is not implemented. Missing values mean unknown usage, not zero consumption.

## Scheduling

Use the included [systemd user service and 12-hour timer](deploy/systemd/README.md). Follow the installation guide and update the service's `WorkingDirectory` and `EnvironmentFile` to match your clone before enabling it.

After installing the units:

```bash
systemctl --user enable --now token-usage-ingest.timer
journalctl --user -u token-usage-ingest.service -n 50
```

Cron can also invoke the executable. Use an explicit working directory so `.env` and relative export paths resolve correctly.

## Privacy

Keep `.env`, personal usage exports, local agent configuration, and database dumps private. The repository ignores `.env`, machine-specific MCP configuration, and non-example CSVs in `cursor-usage-reports/`.

Stored data includes full source paths, optional workspace metadata, and Git remotes. Remote URLs are currently stored without credential redaction, so avoid credential-bearing origin URLs. Treat the PostgreSQL database and diagnostic logs as sensitive.

## Development

Run adapter tests and build the CLI and dashboard:

```bash
go test ./...
go build -o ./bin/token-usage-ingest ./cmd/ingest
go build -o ./bin/token-usage-dashboard ./cmd/dashboard
```

## Documentation

| Guide | What it covers |
| --- | --- |
| [Architecture](docs/architecture.md) | Current design, packages, and schema |
| [Cost and accuracy](docs/cost-and-accuracy.md) | Rate catalog, cache windows, accuracy expectations |
| [Provider usage semantics](docs/provider-usage-semantics.md) | Field mappings and adapter accounting contracts |
| [Product requirements](docs/prd-local-burn-ingest.md) | Scope, goals, and acceptance status |
| [Scheduling guide](deploy/systemd/README.md) | Timer installation and operations |

Current implementation includes the one-shot CLI, PostgreSQL storage, the adapters above, the read-only dashboard, and systemd units. Git storage, cross-source deduplication, and Cursor session-to-usage linking remain deferred.
