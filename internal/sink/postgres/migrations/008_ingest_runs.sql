-- +migrate Up
CREATE TABLE IF NOT EXISTS burn_ingest_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    host_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'running',
    scanned INT NOT NULL DEFAULT 0,
    unchanged_skipped INT NOT NULL DEFAULT 0,
    parsed INT NOT NULL DEFAULT 0,
    upserted INT NOT NULL DEFAULT 0,
    deferred INT NOT NULL DEFAULT 0,
    errors INT NOT NULL DEFAULT 0,
    path_remotes_upserted INT NOT NULL DEFAULT 0,
    fatal_error TEXT
);

CREATE INDEX IF NOT EXISTS burn_ingest_runs_host_started_at_idx
    ON burn_ingest_runs (host_id, started_at DESC);

CREATE TABLE IF NOT EXISTS burn_ingest_issues (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES burn_ingest_runs (id) ON DELETE CASCADE,
    vendor TEXT NOT NULL,
    source_path TEXT,
    severity TEXT NOT NULL CHECK (severity IN ('error', 'warning')),
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS burn_ingest_issues_run_id_idx
    ON burn_ingest_issues (run_id, severity);
