-- +migrate Up
-- Immutable revisions of the cumulative provider counters. Delta columns are
-- the change from the preceding accepted snapshot for a source.
CREATE TABLE IF NOT EXISTS burn_usage_ledger (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES burn_snapshots (source_id) ON DELETE CASCADE,
    host_id TEXT NOT NULL,
    vendor TEXT NOT NULL,
    source_path TEXT NOT NULL,
    provider_session_id TEXT,
    cwd TEXT,
    model TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,

    delta_tokens_input BIGINT,
    delta_tokens_output BIGINT,
    delta_tokens_cache_read BIGINT,
    delta_tokens_cache_write BIGINT,
    delta_tokens_cache_write_5m BIGINT,
    delta_tokens_cache_write_1h BIGINT,
    delta_tokens_reasoning BIGINT,
    delta_tokens_total BIGINT,
    delta_provider_cost NUMERIC,
    delta_rated_cost_usd NUMERIC,

    total_tokens_input BIGINT,
    total_tokens_output BIGINT,
    total_tokens_cache_read BIGINT,
    total_tokens_cache_write BIGINT,
    total_tokens_cache_write_5m BIGINT,
    total_tokens_cache_write_1h BIGINT,
    total_tokens_reasoning BIGINT,
    total_tokens_total BIGINT,
    total_provider_cost NUMERIC,
    total_rated_cost_usd NUMERIC,
    usage_detail JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS burn_usage_ledger_occurred_at_idx
    ON burn_usage_ledger (occurred_at DESC);
CREATE INDEX IF NOT EXISTS burn_usage_ledger_source_recorded_at_idx
    ON burn_usage_ledger (source_id, recorded_at);
CREATE INDEX IF NOT EXISTS burn_usage_ledger_host_occurred_at_idx
    ON burn_usage_ledger (host_id, occurred_at DESC);
