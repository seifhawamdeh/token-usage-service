# PRD: Laptop-local agent burn ingest

**Status:** v1 shipped (Postgres + adapters) — see [architecture.md](architecture.md)  
**Semantics:** [provider-usage-semantics.md](provider-usage-semantics.md)

**Out of scope:** desktop-app SQLite as SoR, invoice reconciliation, Analytics UI (follow-ons).

---

## 1. Overview

Operators need a durable answer to **“what did agent CLIs burn on this machine?”** from **on-disk provider stores**, including sessions never opened in an IDE.

**v1 delivery:** hourly (or cron) **one-shot Go CLI** that discovers enabled vendors, watermark-skips unchanged sources, re-parses changes, and upserts facts into **PostgreSQL**. Git remote sink remains deferred; the pipeline is sink-shaped for later.

---

## 2. Goals

| ID | Goal | v1 |
| --- | --- | --- |
| G-1 | Capture provider-reported model + token components | Done |
| G-2 | Stateless process; watermarks in sink | Done |
| G-3 | Pluggable vendor adapters; enable subset | Done |
| G-4 | Interchangeable sinks (Postgres + Git) | Postgres done; Git deferred |
| G-5 | New / growing / idle / resume via watermarks | Done |
| G-6 | Partial failure isolation; idempotent upserts | Done |
| G-7 | No estimated USD from rates | Done |

---

## 3. User stories (acceptance)

### US-001: Stateless scheduled ingest — **done**

- [x] CLI (`migrate` / `validate-config` / `ingest`)
- [x] Exits after one pass
- [x] Config via `.env` / env
- [x] Nonzero on fatal config/sink errors; zero with per-source errors counted
- [x] Summary: scanned / unchanged_skipped / parsed / upserted / errors

### US-002: Change detection — **done**

- [x] Stat/compare watermarks each run
- [x] Skip parse when mtime+size+signature match
- [x] Persist watermarks only after successful upsert

### US-003: Disk scenarios — **done**

- [x] New / growing / idle / resume-same-path / new-path / keep history if missing on disk

### US-004: Vendor adapters — **done**

- [x] `VendorAdapter` interface
- [x] Multiple reference adapters (see semantics matrix)
- [x] `ENABLED_VENDORS`; one bad file does not abort others

### US-005: Normalized facts — **done**

- [x] Nullable token components; `usage_detail`; optional operator rate card (`model_costs`) separate from ingest

### US-006: Postgres sink — **done**

- [x] `DATABASE_URL`; migrations; upsert by identity; indexes; separate `path_remotes`

### US-007: Git remote sink — **deferred**

### US-008: Choose sink at config time — **partial**

- [x] Shared pipeline + sink interface
- [ ] Git implementation

### US-009: Operability — **done** (baseline)

- [x] Schema version / processing signature; per-file errors; idempotent re-run; README + systemd runbook

---

## 4. Functional requirements

| ID | Requirement | v1 |
| --- | --- | --- |
| FR-1 | Walk only enabled adapter roots | Done |
| FR-2–5 | Watermark compare; parse on change; upsert; watermark after write | Done |
| FR-6 | Pluggable adapter + sink | Done |
| FR-7 | Postgres + Git sinks | Postgres only |
| FR-8 | No cost-from-rates | Done |
| FR-9 | One-shot / schedulable | Done |
| FR-10 | Disk stores as SoR | Done |

---

## 5. Non-goals

- Desktop app ledger as system of record
- Estimating subscription/invoice USD
- Whole-disk scan; realtime watchers
- Multi-writer Git; Analytics UI day one
- Exact machine-wide dedup (deferred)

---

## 6. Success metrics (observed)

- Unchanged second pass → near-zero content parses (`unchanged_skipped`)
- Enabling a vendor backfills within one `--once` / scheduled run
- Same build points at Postgres via config; adapters toggle via `ENABLED_VENDORS`

---

## 7. Decision log

- SoR = on-disk provider stores; process is one-shot / externally scheduled
- Runtime **Go**; sink **Postgres first**, Git later
- Accounting = **per-source** snapshots; disclose double-count risk
- Full paths; remotes in **`path_remotes`** (not on snapshot row)
- Vendors: claude-code, codex, opencode, github-copilot, antigravity, gemini, cursor, cursor-usage
- Copilot billing regimes are date/plan-aware
- Cursor sessions vs cursor-usage CSV are separate facts; Antigravity uses protobuf self-check

Open / deferred: Git sink, cross-source dedup, Cursor session↔usage join, exact exit-code taxonomy polish.

- `project_cwds` keyed by `(host_id, cwd)`, not `cwd` alone — a cwd string isn't unique across machines (2026-09-14)
- Adapters return `ParseResult{Skip: true}` for sources with no real usage/message content (empty antigravity `gen_metadata`, claude-code `bridge-session`-only stubs) instead of writing an all-null snapshot (2026-09-14)
- Day-bucketing timezone offset is configurable (`DASHBOARD_DAY_OFFSET_HOURS`), not hardcoded UTC+3 (2026-09-14)
- Git remote URLs normalized before storage (strip `.git`, SSH→https form) so adapters that read a remote differently still resolve to one `project_remotes` row (2026-09-14)
