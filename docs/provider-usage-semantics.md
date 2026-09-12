# Provider usage semantics

**Status:** Living adapter contract (matches shipped code)  
**Architecture:** [architecture.md](architecture.md)  
**Cost / accuracy:** [cost-and-accuracy.md](cost-and-accuracy.md)

How each vendor maps on-disk usage into a normalized `BurnSnapshot`. Adapters must follow these rules; they must **not** invent USD at parse time. Optional rated USD lives in `model_costs` / `v_burn_usage_rated`.

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
| `tokens.input` / `output` / `cache_read` / `cache_write` / `cache_write_5m` / `cache_write_1h` / `reasoning` / `total` | Components | null (never invent) |
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

- Prefer **cumulative** fields when the provider already aggregates, but check whether the counter can reset mid-file (Codex `total_token_usage` resets on context compaction — sum the peak of each reset-delimited segment, don't take one global last/max; see §3.2).
- Prefer **sum of additive events** otherwise (Claude, Copilot, Gemini messages, Antigravity gens) — but dedupe first if the source can emit more than one event per logical unit (Claude Code streams one JSONL line per chunk of the same assistant message, all sharing `message.id`; summing raw lines double/triple-counts the chunk's input/cache tokens, see §3.1).
- Never sum a field that is already cumulative within a segment.
- Subagents = separate rows unless a vendor section says otherwise.
- `gemini` adapter = CLI paths only; `antigravity` = AG conversation DBs only (no double walk).

---

## 2. Capability matrix

| Vendor | SoR | Status | Rule of thumb |
| --- | --- | --- | --- |
| `claude-code` | JSONL | ready | Dedupe by `message.id`, then sum |
| `codex` | JSONL | ready | Sum of `token_count` peak per compaction segment |
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
- **Aggregate — dedupe by `message.id`, then sum:** Claude Code writes one JSONL line per *streamed chunk* of an assistant message, not one line per message. All chunks of one message share the same `message.id`; `input_tokens` / `cache_creation_input_tokens` / `cache_read_input_tokens` repeat unchanged across chunks (they describe the prompt, fixed once the request starts) while `output_tokens` grows as the stream completes. Summing every raw line therefore multiplies input/cache tokens by however many chunks that message had.
  Algorithm: keep a map keyed by `message.id`, overwrite with each line's `usage` object as seen (file order is chronological, so the last write per id is the most complete chunk), then sum once across the deduped map's values. Lines with no `message.id` are each treated as their own unique message (never merged) since there's nothing safe to dedupe them by. `usage_detail.aggregation` records `"sum_assistant_message_usage_deduped_by_message_id"`; `usage_detail.raw_assistant_lines` keeps the pre-dedupe line count for comparison.
- **Map (per deduped message):** `input_tokens` → input; `cache_creation_input_tokens` → cache_write; `cache_creation.ephemeral_5m_input_tokens` → cache_write_5m; `cache_creation.ephemeral_1h_input_tokens` → cache_write_1h; `cache_read_input_tokens` → cache_read; `output_tokens` → output; `output_tokens_details.thinking_tokens` → reasoning
- **Rates:** Anthropic prices 5m writes at ~1.25× input and 1h writes at ~2× input; `v_burn_usage_rated` applies those when the breakdown is present
- **History (why this matters):** the adapter summed every raw `type:"assistant"` line's usage with no dedup. In real transcripts, 30,061 of 65,443 assistant lines (46%) were duplicate-`message.id` streaming chunks, inflating `tokens_cache_read` alone to ~7.33B across the local corpus. Deduping by `message.id` before summing corrected `claude-code`'s reported total from 7,559,096,690 to 4,183,210,294 (all-fields sum), landing where the user's independent ~4B estimate expected it.

### 3.2 `codex`

- **Roots:** `~/.codex/sessions/**/rollout-*.jsonl`
- **Identity:** prefer `session_id` (from `session_meta.payload.session_id`/`.id`); watermark on rollout path
- **Line shape (verified against real rollout files, 2026-09):** envelope `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{...},"last_token_usage":{...}}}}`. There is no `token_usage_record` / `thread_token_usage` envelope in real Codex output — an earlier adapter version invented that shape and matched almost nothing (see "History" below).
- **Map (from `info.total_token_usage`):** `input_tokens - cached_input_tokens` → input (fresh-only, see below); `cached_input_tokens` → cache_read; `cache_write_input_tokens` → cache_write; `output_tokens` → output; `reasoning_output_tokens` → reasoning; `total_tokens` → total (raw, unadjusted — still `input_tokens + output_tokens`). `info.last_token_usage` is per-turn-since-last-report and is not used.
- **`input_tokens` includes `cached_input_tokens` — it is not additive with cache, unlike Anthropic:** verified against real rollout files that `total_tokens == input_tokens + output_tokens` always holds and `cached_input_tokens` never exceeds `input_tokens`, i.e. Codex's `input_tokens` is the *whole* prompt (cache hits + fresh) and `cached_input_tokens` is just a sub-count of it. This is the opposite of Anthropic's Claude, where `usage.input_tokens` is fresh-only and `cache_read_input_tokens` is a separate additive pool. `v_burn_usage_rated` bills `tokens_input` and `tokens_cache_read` as two independent additive pools (`tokens_input × input_rate + tokens_cache_read × cache_rate`), so storing Codex's raw `input_tokens` there double-billed every cached token: once at full input price (hidden inside the raw total) and again at cache price. Storing `input_tokens - cached_input_tokens` (clamped to ≥0) removes the overlap. `tokens_total` is left as the raw `total_tokens` value — it is not billed and is only used for the segment-peak-detection/session-total math above, where the double-count doesn't matter.
- **Aggregate — sum of segment peaks, not a single last/max value:**
  `total_token_usage` is a cumulative counter *within* one context window, but **resets to near-zero whenever Codex compacts the context** (observed 1–3 resets per session in real data, detected as the value dropping between consecutive `token_count` events). The counter's peak just before a reset represents real tokens spent that a naive "last value" or "single file-wide max" read would silently discard.
  Algorithm: walk `token_count` events in file order; track the running peak of the current segment (a record's own field values, kept together as one struct — never mix fields from different events); when a total drop is seen, add the just-closed segment's peak into a running sum and start a new segment; after the last line, flush the final segment's peak into the sum. The per-vendor `Tokens.{input,output,cache_read,cache_write,reasoning,total}` are each that running sum. `usage_detail.aggregation` records `"sum_of_segment_peak_total_token_usage"`.
  - Sessions with **zero** resets: this reduces to "take the last event's total" (equivalent to a single-segment sum), since Codex's per-segment counter only increases while a segment is open.
  - Do not sum every `token_count` event directly — most events repeat the same cumulative snapshot with small deltas; summing them all would wildly overcount.
- **History (why this matters):** four fix rounds against the same local corpus (436 rollout files, 429 with any `token_count`):
  1. The adapter originally listened for a fictional `token_usage_record`/`thread_token_usage` envelope that never occurs in real Codex output → only 79/436 real sessions got any tokens.
  2. Fixed to read the real `event_msg`/`token_count` shape, taking the **last** event in the file → 429/436 sessions got tokens, but sessions with a mid-file compaction reset undercounted (worst observed case: last=2,984,508 vs. true segment total 11,068,048, a 3.7× miss).
  3. Fixed to take a single file-wide **max** → closer (`sum(tokens_total)` = 2,069,216,450, within 0.04% of an independent third-party tool's 2,070,067,808), but multi-reset sessions (some real sessions reset up to 3 times) still dropped every segment but the largest.
  4. Fixed to **sum of segment peaks** (§ above) → `sum(tokens_total)` = 2,098,278,000, recovering the tokens burned in every pre-compaction segment, not just the biggest one.
  5. Separately, fixed the input/cache-read **double-billing** overlap described above → codex's total rated cost dropped from $6,958.27 to $986.38 (7×) with no change to the underlying usage, because the fix only removed a billing overlap, not real tokens.
  Each step is a separate adapter `Version` bump (`1`→`5`) so the checkpoint signature forces reprocessing of every existing row on the next `ingest` run — no manual backfill script needed.

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
