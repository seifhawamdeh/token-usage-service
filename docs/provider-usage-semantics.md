# Provider usage semantics

**Status:** Living adapter contract (matches shipped code)  
**Architecture:** [architecture.md](architecture.md)

How each vendor maps on-disk usage into a normalized `BurnSnapshot`. Adapters must follow these rules; they must **not** invent USD from rate tables.

**Accounting (v1):** per-source reported usage only — not exact deduplicated machine-wide totals. Aggregates may double-count across copied/branched/resumed sources.

---

## 1. Shared contract

### 1.1 `BurnSnapshot` fields

| Field | Meaning | Missing |
| --- | --- | --- |
| `vendor` | Adapter id (§2) | required |
| `source_path` | Absolute path or `{db}#session|composer:{id}` | required |
| `stable_id` | Upsert identity within vendor | required |
| `cwd` | Workspace if known | null |
| `started_at` / `last_event_at` | If known | null |
| `model` / `models[]` / `model_provider` | Observed models | null / `[]` |
| `tokens.input` / `output` / `cache_read` / `cache_write` / `reasoning` / `total` | Components | null (never invent) |
| `provider_cost` | Provider-embedded only | null |
| `billing_regime` | Copilot: `legacy-premium-requests` \| `ai-credits` | null |
| `usage_detail` | Allowlisted forward-compat JSON | `{}` |

### 1.2 Watermarks

| SoR | `source_path` | Compare |
| --- | --- | --- |
| JSONL / file | Absolute file path | `mtime_ns` + `size_bytes` (+ processing signature) |
| SQLite sessions | `{db}#session:{id}` or `#composer:{id}` | Session/updated fingerprint as mtime/size |
| Antigravity DB | Absolute `.db` path | DB + `-wal`/`-shm` mtime/size |

Advance watermarks **only** after a successful snapshot write.

### 1.3 Hygiene

- Prefer **cumulative** fields when the provider already aggregates (Codex `thread_token_usage`).
- Prefer **sum of additive events** otherwise (Claude, Copilot, Gemini messages, Antigravity gens).
- Never sum a field that is already cumulative.
- Subagents = separate rows unless a vendor section says otherwise.
- `gemini` adapter = CLI paths only; `antigravity` = AG conversation DBs only (no double walk).

---

## 2. Capability matrix

| Vendor | SoR | Status | Rule of thumb |
| --- | --- | --- | --- |
| `claude-code` | JSONL | ready | Sum assistant `message.usage` |
| `codex` | JSONL | ready | **Last** `thread_token_usage` |
| `opencode` | SQLite | ready | Session row `tokens_*` |
| `github-copilot` | SQLite | ready | Sum `assistant_usage_events`; date-aware regime |
| `antigravity` | SQLite protobuf | ready | `gen_metadata`; self-check `#3 == #9+#10` |
| `gemini` | JSONL under `~/.gemini/tmp` | ready | Sum message `tokens`; no-op if root missing |
| `cursor` | `state.vscdb` composers | ready | Session inventory only; tokens null |
| `cursor-usage` | usage-events CSV | ready | One row per export event; no session join |

---

## 3. Per-vendor rules

### 3.1 `claude-code`

- **Roots:** `~/.claude/projects/**/*.jsonl` (incl. subagents)
- **Ignore:** `stats-cache.json`
- **Identity:** absolute path (subagents = separate rows)
- **Aggregate:** sum every assistant `message.usage`
- **Map:** `input_tokens` → input; `cache_creation_input_tokens` → cache_write; `cache_read_input_tokens` → cache_read; `output_tokens` → output; `output_tokens_details.thinking_tokens` → reasoning

### 3.2 `codex`

- **Roots:** `~/.codex/sessions/**/rollout-*.jsonl`
- **Identity:** prefer `session_id`; watermark on rollout path
- **Aggregate:** **last** `payload.thread_token_usage` (do not sum per-call `usage`)
- **Map:** `input_tokens`, `cached_input_tokens`, `cache_write_input_tokens`, `output_tokens`, `reasoning_output_tokens`, `total_tokens`

### 3.3 `antigravity`

- **Roots:** `~/.gemini/antigravity-cli/conversations/*.db` (not brain transcripts)
- **Identity:** conversation UUID (filename stem)
- **Aggregate:** each `gen_metadata` protobuf generation:
  - input = `#1` (system) + `#2` (new); cache_read = `#5`; output = `#9`; reasoning = `#10`
  - require `#3 == #9 + #10` or skip as drift; dedupe response id `#11`
- **Model:** prefer display `#21`, else `#19`
- **CWD:** `trajectory_metadata_blob` workspace URI
- Never treat third-party agent JSON embedded in transcripts as AG burn

### 3.4 `gemini` (CLI only)

- **Roots:** `~/.gemini/tmp/<project_hash>/chats/session-*.{jsonl,json}`
- **Aggregate:** sum message `tokens.{input,output,cached,thoughts,total}`; `tool` → `usage_detail` only
- Does **not** read Antigravity paths

### 3.5 `github-copilot`

- **DB:** `~/.copilot/session-store.db` → `assistant_usage_events`
- **Identity:** `session_id`; path `{db}#session:{id}`
- **Aggregate:** **sum** additive event token columns
- **Always store** `request_multiplier` sum and `total_nano_aiu` sum in `usage_detail`
- **`total_nano_aiu`:** nano AI units (billing), not tokens/USD; credits ≈ `/ 1e9` (confirm before UI)
- **Regime:** `legacy-premium-requests` if session time &lt; `2026-06-01T00:00:00Z` (or `COPILOT_FORCE_LEGACY_PREMIUM_REQUESTS`); else `ai-credits`

### 3.6 `cursor` (sessions)

- **DB:** `~/.config/Cursor/User/globalStorage/state.vscdb` (include WAL; no `immutable=1`)
- **Identity:** `composerId`; path `{state.vscdb}#composer:{id}`
- **Fact:** session metadata (`started_at` / `last_event_at`, workspace, models, bubble counts, optional transcript path)
- **Tokens:** always null on the snapshot — local `tokenCount` is sparse/unreliable
- Agent JSONL transcripts: used only to attach `transcript_path` / project slug hint

### 3.6b `cursor-usage` (token events)

- **Roots:** `CURSOR_USAGE_REPORTS_DIR` (default `cursor-usage-reports/*.csv`)
- **Format:** Cursor usage-events export (`Date`, model, input/cache/output/total, `Cost`, `Kind`)
- **Identity:** event `Date` (unique in exports seen); path `{csv}#event:{Date}`
- **Map:** input ← Input (w/o Cache Write); cache_write ← Input (w/ Cache Write); cache_read; output; total; `Kind` → `billing_regime`; numeric `Cost` → `provider_cost` when not Included/Free
- **No join** to `cursor` composers (CSV has no session/request id)

### 3.7 `opencode`

- **DB:** `~/.local/share/opencode/opencode.db` → `session`
- **Identity:** `session.id`; path `{db}#session:{id}`
- **Aggregate:** session row `tokens_*` (+ `cost` as `provider_cost`)

---

## 4. Follow-ups

1. Gemini CLI fixtures when a machine has `~/.gemini/tmp` data
2. Copilot pre-cutover / annual-legacy PRU fixtures
3. Cursor cloud/Admin API (if product needs full Cursor billing)
4. Optional derived `tokens.input_uncached` for Codex analytics (not ingest core)
5. Official Antigravity protobuf schema if Google publishes one

---

## 5. Decision log

- Vendors: claude-code, codex, opencode, github-copilot, antigravity, gemini (CLI), cursor, cursor-usage
- Cursor = sessions; cursor-usage = CSV token events (no session join)
- Gemini CLI ≠ Antigravity path ownership
- Copilot billing is date/plan-aware; store both PRU and AIU signals
- SQLite vendors use logical `source_path` keys
- Antigravity uses unlabeled protobuf with output self-check
