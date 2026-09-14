-- +migrate Up
-- Staging table for raw work-style log lines (slash-command history, prompt
-- history, session indexes, job timelines) that carry no token/cost data but
-- are useful for later prompt/topic-level analytics. Stored as-is; parsing
-- into structured fields is deferred.
CREATE TABLE IF NOT EXISTS raw_workstyle_logs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    host_id TEXT NOT NULL,
    source_kind TEXT NOT NULL, -- e.g. claude_history, claude_caveman_history, codex_history, codex_session_index, claude_job_timeline
    source_path TEXT NOT NULL,
    line_no INT NOT NULL,
    raw JSONB NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (host_id, source_path, line_no)
);

CREATE INDEX IF NOT EXISTS raw_workstyle_logs_host_kind_idx
    ON raw_workstyle_logs (host_id, source_kind);
