-- +migrate Up
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO schema_meta (key, value) VALUES ('snapshot_schema_version', '1')
ON CONFLICT (key) DO NOTHING;

-- Latest usage snapshot per source (per-source accounting; may double-count across sources)
CREATE TABLE IF NOT EXISTS burn_snapshots (
    source_id              TEXT PRIMARY KEY,
    identity_version       INT NOT NULL DEFAULT 1,
    host_id                TEXT NOT NULL,
    vendor                 TEXT NOT NULL,
    source_path            TEXT NOT NULL,
    stable_id              TEXT NOT NULL,
    provider_session_id    TEXT,
    cwd                    TEXT,
    started_at             TIMESTAMPTZ,
    last_event_at          TIMESTAMPTZ,
    model                  TEXT,
    models                 TEXT[] NOT NULL DEFAULT '{}',
    model_provider         TEXT,
    tokens_input           BIGINT,
    tokens_output          BIGINT,
    tokens_cache_read      BIGINT,
    tokens_cache_write     BIGINT,
    tokens_reasoning       BIGINT,
    tokens_total           BIGINT,
    provider_cost          NUMERIC,
    billing_regime         TEXT,
    usage_detail           JSONB NOT NULL DEFAULT '{}',
    adapter_version        TEXT NOT NULL,
    snapshot_schema_version INT NOT NULL DEFAULT 1,
    parse_status           TEXT NOT NULL DEFAULT 'ok',
    ingested_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (host_id, vendor, source_path)
);

CREATE INDEX IF NOT EXISTS burn_snapshots_vendor_idx ON burn_snapshots (vendor);
CREATE INDEX IF NOT EXISTS burn_snapshots_model_idx ON burn_snapshots (model);
CREATE INDEX IF NOT EXISTS burn_snapshots_started_at_idx ON burn_snapshots (started_at);

-- Watermarks / checkpoints (advanced only after durable snapshot write)
CREATE TABLE IF NOT EXISTS burn_checkpoints (
    source_id              TEXT PRIMARY KEY REFERENCES burn_snapshots (source_id) ON DELETE CASCADE,
    host_id                TEXT NOT NULL,
    vendor                 TEXT NOT NULL,
    source_path            TEXT NOT NULL,
    mtime_ns               BIGINT NOT NULL,
    size_bytes             BIGINT NOT NULL,
    processing_signature   TEXT NOT NULL,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (host_id, vendor, source_path)
);

-- Path ↔ Git remote mapping (separate from snapshots)
CREATE TABLE IF NOT EXISTS path_remotes (
    host_id       TEXT NOT NULL,
    source_path   TEXT NOT NULL,
    remote_url    TEXT,
    remote_name   TEXT NOT NULL DEFAULT 'origin',
    repo_root     TEXT,
    resolved_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (host_id, source_path)
);

CREATE INDEX IF NOT EXISTS path_remotes_remote_url_idx ON path_remotes (remote_url);
