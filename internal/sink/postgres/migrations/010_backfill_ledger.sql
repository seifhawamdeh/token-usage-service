-- +migrate Up
-- Sources that haven't changed on disk since before the ledger existed
-- (migration 007) never got re-parsed, so they never got a ledger row even
-- though burn_snapshots holds their full history. Backfill one ledger row
-- per pre-existing snapshot, treating its current totals as the first delta.
INSERT INTO burn_usage_ledger (
    source_id, host_id, vendor, source_path, provider_session_id, cwd, model, occurred_at, recorded_at,
    delta_tokens_input, delta_tokens_output, delta_tokens_cache_read, delta_tokens_cache_write,
    delta_tokens_cache_write_5m, delta_tokens_cache_write_1h, delta_tokens_reasoning, delta_tokens_total,
    delta_provider_cost, delta_rated_cost_usd,
    total_tokens_input, total_tokens_output, total_tokens_cache_read, total_tokens_cache_write,
    total_tokens_cache_write_5m, total_tokens_cache_write_1h, total_tokens_reasoning, total_tokens_total,
    total_provider_cost, total_rated_cost_usd, usage_detail
)
SELECT
    s.source_id, s.host_id, s.vendor, s.source_path, NULLIF(s.provider_session_id, ''), NULLIF(s.cwd, ''), NULLIF(s.model, ''),
    COALESCE(s.last_event_at, s.started_at, s.ingested_at), s.ingested_at,
    s.tokens_input, s.tokens_output, s.tokens_cache_read, s.tokens_cache_write,
    s.tokens_cache_write_5m, s.tokens_cache_write_1h, s.tokens_reasoning, s.tokens_total,
    s.provider_cost, r.rated_cost_usd,
    s.tokens_input, s.tokens_output, s.tokens_cache_read, s.tokens_cache_write,
    s.tokens_cache_write_5m, s.tokens_cache_write_1h, s.tokens_reasoning, s.tokens_total,
    s.provider_cost, r.rated_cost_usd, COALESCE(s.usage_detail, '{}')
FROM burn_snapshots s
LEFT JOIN v_burn_usage_rated r ON r.source_id = s.source_id
WHERE NOT EXISTS (SELECT 1 FROM burn_usage_ledger l WHERE l.source_id = s.source_id);
