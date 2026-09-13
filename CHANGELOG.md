# Changelog

## 2026-09-14

### Dashboard rewrite + fixes (`b25d3e7`)

Rewrote the dashboard UI in React/Vite/Tailwind (tabs for Home, Daily,
Machines, Configs). Alongside the rewrite, fixed four correctness bugs
found in code review:

- **`ByProject` ignored the project filter** — every other stats endpoint
  (Summary, ByVendor, ByModel, Daily) filtered by the selected project;
  ByProject didn't, so the "By Project" table stayed unfiltered while the
  rest of the dashboard reflected the filter.
- **`project_cwds` keyed by `cwd` alone** — a work-directory path isn't
  unique across machines, so mapping a cwd to a project on one host could
  silently apply to an unrelated repo on another host with the same path.
  Table is now keyed by `(host_id, cwd)`; live table migrated in place
  (67 existing mappings backfilled with their host, zero collisions).
- **Git remote URLs not normalized** — `git remote get-url` can return the
  SSH form with a `.git` suffix while Codex's `session_meta` reports a
  plain `https://` URL with no suffix; the same repo was landing as two
  separate `project_remotes` rows. Added `normalizeRemoteURL` (strips
  `.git`/trailing slash, converts `git@host:path` and `ssh://git@host/path`
  to `https://host/path`) applied before storage.
- **Day-bucketing offset hardcoded to UTC+3** — duplicated in two queries,
  wrong for any deployment outside that timezone. Made configurable via
  `DASHBOARD_DAY_OFFSET_HOURS` (defaults to 3 to preserve current
  behavior).

### Blank-date snapshot rows (same day, follow-up)

Some snapshot rows showed no date in the UI. Root cause: `Snapshots`
returned `last_event_at`/`started_at` but never `ingested_at`, and the
frontend's date fallback chain stopped short of it, even though the SQL's
own ordering/filtering already treated `ingested_at` as the last resort.
Fixed by returning `ingested_at` from the API and extending the frontend
fallback to `last_event_at || started_at || ingested_at`.

### Empty-snapshot ingestion fix (`a21ee3e`)

Investigating further, some rows were still empty even with the date
fallback — `antigravity` and `claude-code` sources with **no usage data
at all**, not just a missing date:

- `antigravity`: a `.db` file with zero rows in `gen_metadata` (session
  created, no generation ever ran) was still producing an all-null
  snapshot. Adapter now returns `ParseResult{Skip: true}` when no
  gen_metadata usage exists (adapter version 1→2).
- `claude-code`: a transcript containing only a `bridge-session` stub
  line (no `user`/`assistant` message at all) was likewise producing an
  all-null snapshot. Adapter now skips when no message-type line is seen
  — while still keeping the legitimate case of a session with a user
  message and no reply yet, which must stay visible (adapter version
  5→6; see `TestMissingTokensStayNil`).
- Re-ran ingest after the version bump (checkpoint signatures include
  adapter version, forcing reparse): 20 sources correctly skipped
  (16 claude-code, 4 antigravity), 873 sources re-parsed/upserted.
- Manually deleted the 19 stale all-null `burn_snapshots` rows left over
  from before the fix (checkpointed skips don't retroactively remove
  already-ingested rows).

### Project-identification data sources (`a21ee3e`)

Four ways a snapshot's project can now be identified, in order:

1. **Codex** `session_meta.git.repository_url` → `BurnSnapshot.GitRemoteURL`
   → `pathremote.ResolveWithKnownRemote`, skipping the local `git`
   shell-out entirely. Works cross-machine and for Windows paths (e.g.
   ingest running on a different host than the one that recorded the
   session). Verified against real `C:\Users\...` sessions.
2. **Cursor** real workspace path read from
   `workspaceStorage/<id>/workspace.json` → `folder`, replacing the old
   string-slug guess. Adapter version 2→3.
3. **Claude Code** `gitBranch` + `aiTitle` captured into
   `usage_detail.git_branch` / `.ai_title`. Adapter version 4→5.

Each source has a passing unit test; `go build ./...`, `go vet ./...`,
and `go test ./...` clean throughout.
