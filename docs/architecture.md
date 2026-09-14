# Architecture (as-built)

**Status:** v1 shipped (Postgres + all planned local adapters)  
**Semantics:** [provider-usage-semantics.md](provider-usage-semantics.md)  
**Cost / accuracy:** [cost-and-accuracy.md](cost-and-accuracy.md)  
**Requirements origin:** [prd-local-burn-ingest.md](prd-local-burn-ingest.md)

## What it is

A **stateless one-shot CLI** (`cmd/ingest`) that:

1. Loads config from env / `.env`
2. For each enabled vendor adapter: discover → watermark compare → parse if changed
3. Upserts snapshot + checkpoint atomically in Postgres
4. Upserts path → Git remote in a **separate** table
5. Prints a run summary and exits

External scheduling (systemd timer / cron) owns recurrence. No long-lived process.

```text
schedule → load config → for each enabled adapter:
  discover → stat/compare watermarks →
  [if changed] parse → BurnSnapshot → sink.upsert (+ path_remotes)
→ log summary → exit
```

## Locked decisions

| Topic | Choice |
| --- | --- |
| Process | One-shot CLI; modular `adapters` / `pipeline` / `sinks` |
| Runtime | **Go** |
| Sink (v1) | **PostgreSQL only** (Git later) |
| Accounting | Per-source snapshots; duplicate groups detected and flagged (`internal/dedup`, recomputed every ingest) but not auto-excluded from totals |
| Paths | Full absolute paths; **no redaction**; remotes in `path_remotes` |
| Cost | `provider_cost` = vendor-reported only; optional `model_costs` rate card → `v_burn_usage_rated.rated_cost_usd` |
| Tokens | Nullable fields (`*int64`); missing ≠ zero |

## Package layout

```text
cmd/ingest/                 CLI: migrate | validate-config | ingest
cmd/loadworkstyle/          CLI: stage raw slash-command/prompt history lines (no token/cost data)
cmd/dashboard/              Local HTTP UI + JSON API
internal/
  config/                   env loading
  model/                    BurnSnapshot, Checkpoint, PathRemote
  identity/                 source_id + processing signature
  adapter/                  VendorAdapter interface
    claudecode/ codex/ opencode/ githubcopilot/
    antigravity/ gemini/ cursor/ cursorusage/
  pipeline/                 discover → compare → parse → upsert
  dedup/                    duplicate-group detection (provider_session_id, host+cwd+time-window)
  pathremote/               git rev-parse / remote get-url
  sink/postgres/            DB + embedded migrations
  dashboard/                API handlers + embedded static UI
deploy/systemd/             user timer + oneshot service
```

## Postgres schema

| Table | Role |
| --- | --- |
| `burn_snapshots` | Latest usage fact per `(host_id, vendor, source_path)` / `source_id` |
| `burn_checkpoints` | Watermark after successful snapshot write |
| `path_remotes` | `(host_id, source_path)` → `remote_url`, `repo_root` |
| `model_costs` | Operator rate card: USD per 1M tokens by `model_key` + **dated** `[effective_from, effective_to)` periods |
| `model_cost_aliases` | Display / vendor labels → `model_key` |
| `schema_migrations` | Applied migration filenames |
| `burn_duplicate_groups` / `burn_duplicate_members` | Recomputed wholesale every ingest run (`internal/dedup`); groups likely-duplicate snapshots, marks one canonical |
| `raw_workstyle_logs` | Raw slash-command/prompt history lines, staged as-is by `cmd/loadworkstyle`; parsing deferred |

View **`v_burn_usage_rated`**: joins each snapshot to the rate row covering `COALESCE(last_event_at, started_at, ingested_at)` → `rated_cost_usd`. Full catalog / cache-window / accuracy notes: [cost-and-accuracy.md](cost-and-accuracy.md). `provider_cost` stays vendor-reported.

Identity: `source_id = sha256(v1 ‖ host_id ‖ vendor ‖ source_path)`.  
Checkpoints also store `processing_signature = adapter@version|schema=N` so parser upgrades force reparse.

## Sink contract

- Lookup checkpoints in batches by vendor + paths
- **Atomic** replace of snapshot + matching checkpoint in one transaction
- Path remote upsert is separate (best-effort after snapshot; failure fails the run)
- Per-source parse errors: log, continue, exit 0; config/sink failures: nonzero

## Config (env)

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | Postgres URL (required) |
| `HOST_ID` | Stable machine identity (required) |
| `ENABLED_VENDORS` | Comma-separated adapter ids |
| `SINK` | `postgres` (only value in v1) |
| `CLAUDE_PROJECTS_ROOT`, `CODEX_ROOT`, `OPENCODE_DB_PATH`, `COPILOT_DB_PATH`, `GEMINI_TMP_ROOT`, `ANTIGRAVITY_ROOT`, `CURSOR_STATE_DB` | Optional path overrides (`CODEX_SESSIONS_ROOT` remains a compatibility alias) |
| `CURSOR_USAGE_REPORTS_DIR` | Cursor usage-events CSV directory (default `cursor-usage-reports`) |
| `COPILOT_FORCE_LEGACY_PREMIUM_REQUESTS` | `true` → PRU regime after cutover |

### Cursor split

| Vendor | Fact type | Tokens |
| --- | --- | --- |
| `cursor` | Local composer sessions (`state.vscdb`) | Always null (local `tokenCount` unreliable) |
| `cursor-usage` | Dashboard usage-events CSV rows | Filled from export; **not** joined to composers |

See `.env.example`.

## Non-goals (still)

- Desktop app SQLite as system of record
- Invoice reconciliation / Analytics UI
- Whole-disk scan, realtime watchers, multi-writer Git
- Automatic exclusion of detected duplicates from aggregate totals (detection exists; totals still require filtering on `is_canonical`)
- Cursor session ↔ usage-event join

## Deferred

1. Git remote sink (shared pipeline already sink-shaped)
2. Automatic totals adjustment from detected duplicate groups (detection itself is shipped — see `internal/dedup`, `burn_duplicate_groups`)
3. Cursor session ↔ usage-event join (CSV lacks composer ids)
4. Richer active-file / bounded-prefix freshness policies
