-- Cache TTL windows (Anthropic 5m vs 1h writes) + long-context list tiers.
-- Claude JSONL exposes cache_creation.ephemeral_{5m,1h}_input_tokens.

ALTER TABLE burn_snapshots
    ADD COLUMN IF NOT EXISTS tokens_cache_write_5m BIGINT,
    ADD COLUMN IF NOT EXISTS tokens_cache_write_1h BIGINT;

ALTER TABLE model_costs
    ADD COLUMN IF NOT EXISTS cache_write_5m_usd_per_mtok NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS cache_write_1h_usd_per_mtok NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS long_context_token_threshold BIGINT,
    ADD COLUMN IF NOT EXISTS input_usd_per_mtok_long NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS output_usd_per_mtok_long NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS cache_read_usd_per_mtok_long NUMERIC(12, 6);

COMMENT ON COLUMN model_costs.cache_write_usd_per_mtok IS
    'Fallback / default cache-write rate (Anthropic: 5m tier). Prefer cache_write_5m/1h when token breakdown exists.';
COMMENT ON COLUMN model_costs.cache_write_5m_usd_per_mtok IS
    'Anthropic ephemeral 5-minute cache write USD per 1M tokens (typically 1.25x input).';
COMMENT ON COLUMN model_costs.cache_write_1h_usd_per_mtok IS
    'Anthropic ephemeral 1-hour cache write USD per 1M tokens (typically 2x input).';
COMMENT ON COLUMN model_costs.long_context_token_threshold IS
    'When set, prompts at/above this size use *_long rates (e.g. 200000 for Gemini/xAI). Applied only when usage_detail.prompt_tokens_for_rate is present.';

-- Anthropic: derive 5m/1h write rates from base input when missing.
UPDATE model_costs
SET
    cache_write_5m_usd_per_mtok = COALESCE(
        cache_write_5m_usd_per_mtok,
        cache_write_usd_per_mtok,
        CASE WHEN input_usd_per_mtok IS NOT NULL THEN input_usd_per_mtok * 1.25 END
    ),
    cache_write_1h_usd_per_mtok = COALESCE(
        cache_write_1h_usd_per_mtok,
        CASE WHEN input_usd_per_mtok IS NOT NULL THEN input_usd_per_mtok * 2 END
    ),
    cache_write_usd_per_mtok = COALESCE(
        cache_write_usd_per_mtok,
        cache_write_5m_usd_per_mtok,
        CASE WHEN input_usd_per_mtok IS NOT NULL THEN input_usd_per_mtok * 1.25 END
    )
WHERE provider = 'anthropic'
  AND input_usd_per_mtok IS NOT NULL;

-- Long-context list tiers (<threshold uses base; ≥threshold uses *_long).
UPDATE model_costs SET
    long_context_token_threshold = 200000,
    input_usd_per_mtok_long = 4.00,
    output_usd_per_mtok_long = 18.00,
    cache_read_usd_per_mtok_long = 0.40
WHERE model_key IN ('gemini-3.1-pro', 'gemini-3.1-pro-preview', 'gemini-3.1-pro-low', 'gemini-3.1-pro-high', 'gemini-pro-default')
  AND long_context_token_threshold IS NULL;

UPDATE model_costs SET
    long_context_token_threshold = 200000,
    input_usd_per_mtok_long = 2.50,
    output_usd_per_mtok_long = 15.00
WHERE model_key = 'gemini-2.5-pro'
  AND long_context_token_threshold IS NULL;

UPDATE model_costs SET
    long_context_token_threshold = 200000,
    input_usd_per_mtok_long = 4.00,
    output_usd_per_mtok_long = 12.00,
    cache_read_usd_per_mtok_long = COALESCE(cache_read_usd_per_mtok, 0.50) * 2
WHERE model_key IN ('grok-4.6', 'cursor-grok-4.6-high', 'cursor-grok-4.6-medium', 'cursor-grok-4.6-low')
  AND long_context_token_threshold IS NULL;

UPDATE model_costs SET
    long_context_token_threshold = 200000,
    input_usd_per_mtok_long = 4.00,
    output_usd_per_mtok_long = 12.00,
    cache_read_usd_per_mtok_long = COALESCE(cache_read_usd_per_mtok, 0.30) * 2
WHERE model_key = 'grok-4.5'
  AND long_context_token_threshold IS NULL;

UPDATE model_costs SET
    long_context_token_threshold = 200000,
    input_usd_per_mtok_long = 2.50,
    output_usd_per_mtok_long = 5.00,
    cache_read_usd_per_mtok_long = COALESCE(cache_read_usd_per_mtok, 0.20) * 2
WHERE provider = 'xai'
  AND model_key IN (
    'grok-4', 'grok-4-0709', 'grok-4.3', 'grok-4-fast', 'grok-4-fast-reasoning',
    'grok-4-fast-non-reasoning', 'grok-4-1-fast', 'grok-4-1-fast-reasoning',
    'grok-4.20-0309-reasoning', 'grok-4.20-0309-non-reasoning',
    'grok-4.20-multi-agent-0309', 'grok-3', 'grok-3-mini', 'grok-build-0.1'
  )
  AND effective_from >= TIMESTAMPTZ '2026-01-01 00:00:00+00'
  AND long_context_token_threshold IS NULL;

UPDATE schema_meta SET value = '2' WHERE key = 'snapshot_schema_version';
INSERT INTO schema_meta (key, value) VALUES ('snapshot_schema_version', '2')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DROP VIEW IF EXISTS v_burn_usage_rated;
CREATE VIEW v_burn_usage_rated AS
WITH resolved AS (
    SELECT
        s.*,
        COALESCE(a.model_key, s.model) AS rate_model_key,
        COALESCE(s.last_event_at, s.started_at, s.ingested_at) AS rate_as_of,
        NULLIF((s.usage_detail->>'prompt_tokens_for_rate')::bigint, 0) AS prompt_tokens_for_rate
    FROM burn_snapshots s
    LEFT JOIN model_cost_aliases a ON a.alias = s.model
)
SELECT
    r.source_id,
    r.host_id,
    r.vendor,
    r.source_path,
    r.stable_id,
    r.provider_session_id,
    r.cwd,
    r.started_at,
    r.last_event_at,
    r.model,
    r.models,
    r.model_provider,
    r.tokens_input,
    r.tokens_output,
    r.tokens_cache_read,
    r.tokens_cache_write,
    r.tokens_cache_write_5m,
    r.tokens_cache_write_1h,
    r.tokens_reasoning,
    r.tokens_total,
    r.provider_cost,
    r.billing_regime,
    r.usage_detail,
    r.ingested_at,
    r.rate_model_key,
    r.rate_as_of,
    c.id AS model_cost_id,
    c.provider AS rate_provider,
    c.display_name AS rate_display_name,
    c.effective_from AS rate_effective_from,
    c.effective_to AS rate_effective_to,
    c.input_usd_per_mtok,
    c.output_usd_per_mtok,
    c.cache_read_usd_per_mtok,
    c.cache_write_usd_per_mtok,
    c.cache_write_5m_usd_per_mtok,
    c.cache_write_1h_usd_per_mtok,
    c.reasoning_usd_per_mtok,
    c.long_context_token_threshold,
    c.input_usd_per_mtok_long,
    c.output_usd_per_mtok_long,
    CASE
        WHEN c.id IS NULL THEN NULL
        WHEN c.input_usd_per_mtok IS NULL
         AND c.output_usd_per_mtok IS NULL
         AND c.cache_read_usd_per_mtok IS NULL
         AND c.cache_write_usd_per_mtok IS NULL
         AND c.cache_write_5m_usd_per_mtok IS NULL
         AND c.cache_write_1h_usd_per_mtok IS NULL
         AND c.reasoning_usd_per_mtok IS NULL THEN NULL
        ELSE (
            COALESCE(r.tokens_input, 0) * COALESCE(
                CASE
                    WHEN r.prompt_tokens_for_rate IS NOT NULL
                     AND c.long_context_token_threshold IS NOT NULL
                     AND r.prompt_tokens_for_rate >= c.long_context_token_threshold
                    THEN c.input_usd_per_mtok_long
                    ELSE c.input_usd_per_mtok
                END,
                0
            )
          + COALESCE(r.tokens_output, 0) * COALESCE(
                CASE
                    WHEN r.prompt_tokens_for_rate IS NOT NULL
                     AND c.long_context_token_threshold IS NOT NULL
                     AND r.prompt_tokens_for_rate >= c.long_context_token_threshold
                    THEN c.output_usd_per_mtok_long
                    ELSE c.output_usd_per_mtok
                END,
                0
            )
          + COALESCE(r.tokens_cache_read, 0) * COALESCE(
                CASE
                    WHEN r.prompt_tokens_for_rate IS NOT NULL
                     AND c.long_context_token_threshold IS NOT NULL
                     AND r.prompt_tokens_for_rate >= c.long_context_token_threshold
                    THEN COALESCE(c.cache_read_usd_per_mtok_long, c.cache_read_usd_per_mtok)
                    ELSE c.cache_read_usd_per_mtok
                END,
                0
            )
          + CASE
                -- Prefer explicit 5m/1h breakdown when present.
                WHEN r.tokens_cache_write_5m IS NOT NULL OR r.tokens_cache_write_1h IS NOT NULL THEN
                    COALESCE(r.tokens_cache_write_5m, 0) * COALESCE(
                        c.cache_write_5m_usd_per_mtok, c.cache_write_usd_per_mtok, 0
                    )
                  + COALESCE(r.tokens_cache_write_1h, 0) * COALESCE(
                        c.cache_write_1h_usd_per_mtok,
                        c.cache_write_5m_usd_per_mtok,
                        c.cache_write_usd_per_mtok,
                        0
                    )
                ELSE
                    COALESCE(r.tokens_cache_write, 0) * COALESCE(
                        c.cache_write_usd_per_mtok,
                        c.cache_write_5m_usd_per_mtok,
                        0
                    )
            END
          + COALESCE(r.tokens_reasoning, 0) * COALESCE(
                c.reasoning_usd_per_mtok, c.output_usd_per_mtok, 0
            )
        ) / 1000000.0
    END AS rated_cost_usd
FROM resolved r
LEFT JOIN LATERAL (
    SELECT c.*
    FROM model_costs c
    WHERE c.model_key = r.rate_model_key
      AND r.rate_as_of >= c.effective_from
      AND (c.effective_to IS NULL OR r.rate_as_of < c.effective_to)
    ORDER BY c.effective_from DESC
    LIMIT 1
) c ON TRUE;
