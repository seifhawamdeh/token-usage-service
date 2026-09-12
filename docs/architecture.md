# Architecture (as-built)

**Status:** v1 shipped (Postgres + all planned local adapters)  
**Semantics:** [provider-usage-semantics.md](provider-usage-semantics.md)  
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
| Accounting | Per-source snapshots; disclose double-count risk; no cross-source dedup |
| Paths | Full absolute paths; **no redaction**; remotes in `path_remotes` |
| Cost | Store provider-reported signals only; never estimate USD |
| Tokens | Nullable fields (`*int64`); missing ≠ zero |

## Package layout

```text
cmd/ingest/                 CLI: migrate | validate-config | ingest
internal/
  config/                   env loading
  model/                    BurnSnapshot, Checkpoint, PathRemote
  identity/                 source_id + processing signature
  adapter/                  VendorAdapter interface
    claudecode/ codex/ opencode/ githubcopilot/
    antigravity/ gemini/ cursor/ cursorusage/
  pipeline/                 discover → compare → parse → upsert
  pathremote/               git rev-parse / remote get-url
  sink/postgres/            DB + embedded migrations
deploy/systemd/             user timer + oneshot service
```

## Postgres schema

| Table | Role |
| --- | --- |
| `burn_snapshots` | Latest usage fact per `(host_id, vendor, source_path)` / `source_id` |
| `burn_checkpoints` | Watermark after successful snapshot write |
| `path_remotes` | `(host_id, source_path)` → `remote_url`, `repo_root` |
| `schema_migrations` | Applied migration filenames |

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
| `CLAUDE_PROJECTS_ROOT`, `CODEX_SESSIONS_ROOT`, `OPENCODE_DB_PATH`, `COPILOT_DB_PATH`, `GEMINI_TMP_ROOT`, `ANTIGRAVITY_ROOT`, `CURSOR_STATE_DB`, `CURSOR_USAGE_REPORTS_DIR` | Optional path overrides |
| `COPILOT_FORCE_LEGACY_PREMIUM_REQUESTS` | `true` → PRU regime after cutover |

### Cursor split

| Vendor | Fact type | Tokens |
| --- | --- | --- |
| `cursor` | Local composer sessions (`state.vscdb`) | Always null (local `tokenCount` unreliable) |
| `cursor-usage` | Dashboard usage-events CSV rows | Filled from export; **not** joined to composers |

See `.env.example`.

## Non-goals (still)

- Desktop app SQLite as system of record
- Rate tables / invoice reconciliation / Analytics UI
- Whole-disk scan, realtime watchers, multi-writer Git
- Exact deduplicated machine-wide billing totals

## Deferred

1. Git remote sink (shared pipeline already sink-shaped)
2. Cross-source session linking / dedup
3. Cursor session ↔ usage-event join (CSV lacks composer ids)
4. Richer active-file / bounded-prefix freshness policies
