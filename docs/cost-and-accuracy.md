# Cost catalog, cache windows, and accuracy

How rated USD works, what the catalog contains, how Anthropic cache TTLs are priced, and what accuracy to expect.

Related: [architecture.md](architecture.md) · [provider-usage-semantics.md](provider-usage-semantics.md)

---

## 1. Two different “costs”

| Signal | Where | Meaning |
| --- | --- | --- |
| `provider_cost` | `burn_snapshots` | Whatever the **vendor embedded** (OpenCode `cost`, Cursor CSV numeric `Cost`, etc.). Untouched by our rate card. |
| `rated_cost_usd` | `v_burn_usage_rated` | **Operator list-price math**: tokens × `model_costs` rates in force at the burn timestamp. |

Ingest never invents USD at parse time. Rating is a SQL view over snapshots + the catalog.

```sql
SELECT vendor, model, rated_cost_usd, provider_cost, rate_as_of, rate_effective_from
FROM v_burn_usage_rated
WHERE rated_cost_usd IS NOT NULL
ORDER BY last_event_at DESC
LIMIT 50;
```

---

## 2. Cost catalog (`model_costs`)

### Shape

| Column | Role |
| --- | --- |
| `model_key` | Stable id (after aliases) |
| `input_usd_per_mtok` / `output_usd_per_mtok` / `cache_read_usd_per_mtok` / `reasoning_usd_per_mtok` | USD per **1M** tokens |
| `cache_write_usd_per_mtok` | Fallback write rate (Anthropic: **5m** tier) |
| `cache_write_5m_usd_per_mtok` / `cache_write_1h_usd_per_mtok` | Anthropic ephemeral TTL write rates |
| `long_context_token_threshold` + `*_usd_per_mtok_long` | Optional ≥N-token list tier (Gemini / xAI style) |
| `effective_from` / `effective_to` | Half-open period **`[from, to)`**; `to` null = open-ended |
| `source` / `notes` | Provenance |

`model_cost_aliases` maps vendor/display strings → `model_key` (e.g. `Claude Sonnet 4.6 (Thinking)` → `claude-sonnet-4-6`).

### As-of resolution

For each snapshot:

1. `rate_model_key = COALESCE(alias.model_key, snapshot.model)`
2. `rate_as_of = COALESCE(last_event_at, started_at, ingested_at)`
3. Pick the `model_costs` row with matching `model_key` where `rate_as_of >= effective_from` and (`effective_to` is null or `rate_as_of < effective_to`), latest `effective_from` wins.

Price cuts are **new rows**, not overwrites (e.g. GPT-5.6 Terra/Luna, Opus 4 → 4.5).

### Seed catalog

Migrations `002`–`004` seed Anthropic, OpenAI, Google Gemini, and xAI list prices (plus Cursor/OpenCode placeholders). Treat the seed as a **starting card**:

- Extend with `INSERT` when a provider changes list prices.
- Close the old period: set `effective_to` on the prior row; insert a new row with the new `effective_from`.
- Fill null-rate keys (Composer, `auto`, etc.) only if you have a deliberate $/MTok policy.

---

## 3. Cache windows (Anthropic)

Claude usage often includes:

```json
"cache_creation": {
  "ephemeral_5m_input_tokens": …,
  "ephemeral_1h_input_tokens": …
}
```

| Snapshot field | Source | Typical list multiplier vs input |
| --- | --- | --- |
| `tokens_cache_write_5m` | `ephemeral_5m_input_tokens` | ~**1.25×** (`cache_write_5m_usd_per_mtok`) |
| `tokens_cache_write_1h` | `ephemeral_1h_input_tokens` | ~**2×** (`cache_write_1h_usd_per_mtok`) |
| `tokens_cache_write` | sum of windows, or legacy `cache_creation_input_tokens` | fallback × 5m rate if no breakdown |

`v_burn_usage_rated` prefers the 5m/1h breakdown when either column is non-null; otherwise it rates total `tokens_cache_write` at the 5m/fallback write rate.

**Re-ingest Claude** after adapter/schema v2 (processing signature change) so older snapshots gain the breakdown. Until then, 1h writes may be under-priced as 5m.

### Long-context windows (Gemini / xAI)

Catalog rows may set `long_context_token_threshold` (often `200000`) and `*_long` rates. The view switches only when:

```text
usage_detail.prompt_tokens_for_rate >= long_context_token_threshold
```

Adapters do **not** set that field today → long tier is unused unless you populate it. Short-tier rates apply by default.

---

## 4. Accuracy expectations

### Token counts (ingest)

| Source | Expectation |
| --- | --- |
| Claude Code (adapter v3+) | **High** — sums assistant `message.usage` deduped by `message.id`. Before v3, raw streamed-chunk lines were summed without dedup: 46% of lines in a real corpus were duplicate chunks of the same message, inflating `tokens_cache_read` ~1.75×. Re-ingest to pick this up (checkpoint signature forces reparse automatically). |
| Codex (adapter v5+) | **High** — sums each compaction segment's peak `total_token_usage`, and stores fresh-only input (`input_tokens - cached_input_tokens`) so `tokens_input` and `tokens_cache_read` no longer overlap. Before v5, `tokens_input` stored Codex's raw `input_tokens`, which already includes cached tokens — `v_burn_usage_rated` then billed the cached slice twice (once as input, once as cache_read), a **7× cost overstatement** in one real corpus ($6,958 → $986). See [semantics §3.2](provider-usage-semantics.md#32-codex) for the full history. |
| OpenCode, Copilot, Antigravity | **High** fidelity to on-disk provider fields, given correct mapping ([semantics](provider-usage-semantics.md)). |
| Claude 5m/1h cache | **High after re-ingest** with breakdown present; legacy rows: total write only. |
| Cursor (`cursor`) | Sessions only — tokens **null** by design. |
| Cursor usage CSV (`cursor-usage`) | **High for the export**; account-level, not machine-bound; duplicate files double-count. |
| Gemini CLI | High when `~/.gemini/tmp` has data; empty root → no rows. |

`null` token components mean **unknown**, not zero. Avoid `SUM` without `COALESCE`/`FILTER` if you intend “unknown as zero.”

### Rated USD

| Situation | Expectation |
| --- | --- |
| Claude + matched `model_key` + correct rate period + cache breakdown re-ingested | Roughly **±5–15%** vs **API list** math for that model/time — not vs a flat subscription. |
| Codex, adapter v5+ | Input/cache-read no longer overlap (see accuracy table above) — rely on current numbers; anything computed before the v5 re-ingest overstated Codex cost ~7×. |
| Model string unmatched / null rates | `rated_cost_usd` **null**. |
| Cursor Included/Free, Copilot plan, Composer | List $/MTok ≠ cash paid; rated cost is **illustrative** at best. |
| Long-context ≥200k without `prompt_tokens_for_rate` | Short tier always — can **understate**. |
| Batch / Priority / Bedrock / Vertex / unlisted promos | Not in card — **gap**. |
| Mid-generation launch dates in the seed | Approximate — tighten periods when you have better dates. |

`provider_cost` beats rated USD when the vendor reported an authoritative amount for that source.

### Aggregates

- **Per-source** snapshots are the unit of truth.
- **Cross-source / cross-vendor totals are not bill-accurate** (copies, resumes, same work in Claude + Cursor, CSV overlapping CLI).
- No Cursor session ↔ usage join; no cross-source dedup.

### Practical guidance

1. Compare **within one vendor** (or one `model_key`) over time.
2. Keep `model_costs` periods updated when providers publish price changes.
3. After cache-window / schema bumps, run Claude ingest so signatures force reparse.
4. Use `rated_cost_usd` for trends and relative model cost; use invoices/`provider_cost` for cash reconciliation.

---

## 5. Migrations

| File | Adds |
| --- | --- |
| `002_model_costs.sql` | Tables + view + initial Anthropic seed |
| `003_model_costs_catalog.sql` | Broad historical catalog + aliases + as-of view |
| `004_cache_windows.sql` | `tokens_cache_write_5m`/`_1h`, 5m/1h rate columns, long-context columns, rated view update |

Apply with `go run ./cmd/ingest migrate`.
