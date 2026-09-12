-- Model rate card (USD per 1M tokens) + aliases for burn_snapshots.model matching.
-- Rated cost is computed in v_burn_usage_rated; ingest still stores provider_cost as-reported only.

CREATE TABLE IF NOT EXISTS model_costs (
    id                       BIGSERIAL PRIMARY KEY,
    model_key                TEXT NOT NULL,
    provider                 TEXT,
    display_name             TEXT,
    input_usd_per_mtok       NUMERIC(12, 6),
    output_usd_per_mtok      NUMERIC(12, 6),
    cache_read_usd_per_mtok  NUMERIC(12, 6),
    cache_write_usd_per_mtok NUMERIC(12, 6),
    reasoning_usd_per_mtok   NUMERIC(12, 6),
    currency                 TEXT NOT NULL DEFAULT 'USD',
    effective_from           TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00',
    effective_to             TIMESTAMPTZ,
    source                   TEXT,
    notes                    TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT model_costs_range_chk CHECK (
        effective_to IS NULL OR effective_to > effective_from
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS model_costs_model_key_from_uidx
    ON model_costs (model_key, effective_from);

CREATE INDEX IF NOT EXISTS model_costs_provider_idx ON model_costs (provider);
CREATE INDEX IF NOT EXISTS model_costs_effective_idx
    ON model_costs (model_key, effective_from DESC);

-- Map vendor-specific / display labels → model_costs.model_key
CREATE TABLE IF NOT EXISTS model_cost_aliases (
    alias     TEXT PRIMARY KEY,
    model_key TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS model_cost_aliases_model_key_idx
    ON model_cost_aliases (model_key);

-- Anthropic list prices (API, global, 5m cache writes) as of ~2026-09.
-- Source: https://docs.anthropic.com/en/docs/about-claude/pricing
INSERT INTO model_costs (
    model_key, provider, display_name,
    input_usd_per_mtok, output_usd_per_mtok,
    cache_read_usd_per_mtok, cache_write_usd_per_mtok, reasoning_usd_per_mtok,
    effective_from, source, notes
) VALUES
    ('claude-haiku-4-5', 'anthropic', 'Claude Haiku 4.5',
     1.00, 5.00, 0.10, 1.25, 5.00,
     TIMESTAMPTZ '2025-10-01 00:00:00+00', 'anthropic-docs', '5m cache write tier'),
    ('claude-sonnet-4-5', 'anthropic', 'Claude Sonnet 4.5',
     3.00, 15.00, 0.30, 3.75, 15.00,
     TIMESTAMPTZ '2025-10-01 00:00:00+00', 'anthropic-docs', '5m cache write tier'),
    ('claude-sonnet-4-6', 'anthropic', 'Claude Sonnet 4.6',
     3.00, 15.00, 0.30, 3.75, 15.00,
     TIMESTAMPTZ '2026-01-01 00:00:00+00', 'anthropic-docs', '5m cache write tier'),
    ('claude-sonnet-5', 'anthropic', 'Claude Sonnet 5',
     2.00, 10.00, 0.20, 2.50, 10.00,
     TIMESTAMPTZ '2026-01-01 00:00:00+00', 'anthropic-docs', 'standard after intro period'),
    ('claude-opus-4-5', 'anthropic', 'Claude Opus 4.5',
     5.00, 25.00, 0.50, 6.25, 25.00,
     TIMESTAMPTZ '2025-10-01 00:00:00+00', 'anthropic-docs', '5m cache write tier'),
    ('claude-opus-4-6', 'anthropic', 'Claude Opus 4.6',
     5.00, 25.00, 0.50, 6.25, 25.00,
     TIMESTAMPTZ '2026-01-01 00:00:00+00', 'anthropic-docs', '5m cache write tier'),
    ('claude-opus-5', 'anthropic', 'Claude Opus 5',
     5.00, 25.00, 0.50, 6.25, 25.00,
     TIMESTAMPTZ '2026-01-01 00:00:00+00', 'anthropic-docs', '5m cache write tier')
ON CONFLICT DO NOTHING;

-- Observed usage model strings / dated SKUs → rate keys (rates NULL until filled)
INSERT INTO model_costs (model_key, provider, display_name, effective_from, source, notes) VALUES
    ('claude-haiku-4-5-20251001', 'anthropic', 'Claude Haiku 4.5 (dated SKU)', TIMESTAMPTZ '2025-10-01 00:00:00+00', 'seed', 'alias preferred; rates via alias'),
    ('gpt-5.4', 'openai', 'GPT-5.4', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gpt-5.6-terra', 'openai', 'GPT-5.6 Terra', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gpt-5.6-sol', 'openai', 'GPT-5.6 Sol', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gpt-5.6-luna', 'openai', 'GPT-5.6 Luna', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gpt-6-astra', 'openai', 'GPT-6 Astra', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gemini-3.1-pro', 'google', 'Gemini 3.1 Pro', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gemini-3.6-flash', 'google', 'Gemini 3.6 Flash', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gemini-3.8-flash', 'google', 'Gemini 3.8 Flash', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('gemini-pro-default', 'google', 'Gemini Pro (default)', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('grok-4.6', 'xai', 'Grok 4.6', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('cursor-grok-4.6-high', 'cursor', 'Cursor Grok 4.6 High', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'subscription/CSV; fill if desired'),
    ('cursor-grok-4.6-medium', 'cursor', 'Cursor Grok 4.6 Medium', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'subscription/CSV; fill if desired'),
    ('claude-sonnet-5-thinking-medium', 'cursor', 'Claude Sonnet 5 Thinking Medium', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'maps to sonnet-5 API rates via alias if preferred'),
    ('claude-sonnet-5-thinking-high', 'cursor', 'Claude Sonnet 5 Thinking High', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'maps to sonnet-5 API rates via alias if preferred'),
    ('claude-4.5-sonnet-thinking', 'cursor', 'Claude 4.5 Sonnet Thinking', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'maps to sonnet-4-5 via alias'),
    ('composer-2.5-fast', 'cursor', 'Composer 2.5 Fast', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('auto', 'cursor', 'Cursor Auto', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'router; no stable list price'),
    ('big-pickle', 'opencode', 'big-pickle', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'seed', 'fill rates'),
    ('<synthetic>', 'anthropic', 'Synthetic', TIMESTAMPTZ '1970-01-01 00:00:00+00', 'seed', 'no billable rate')
ON CONFLICT DO NOTHING;

-- Copy Anthropic rates onto dated Haiku SKU used by claude-code
UPDATE model_costs AS d
SET
    input_usd_per_mtok = s.input_usd_per_mtok,
    output_usd_per_mtok = s.output_usd_per_mtok,
    cache_read_usd_per_mtok = s.cache_read_usd_per_mtok,
    cache_write_usd_per_mtok = s.cache_write_usd_per_mtok,
    reasoning_usd_per_mtok = s.reasoning_usd_per_mtok,
    source = s.source,
    notes = 'same as claude-haiku-4-5'
FROM model_costs s
WHERE d.model_key = 'claude-haiku-4-5-20251001'
  AND s.model_key = 'claude-haiku-4-5'
  AND d.input_usd_per_mtok IS NULL;

INSERT INTO model_cost_aliases (alias, model_key) VALUES
    ('claude-haiku-4-5-20251001', 'claude-haiku-4-5'),
    ('Claude Sonnet 4.6 (Thinking)', 'claude-sonnet-4-6'),
    ('claude-opus-4-6-thinking', 'claude-opus-4-6'),
    ('claude-4.5-sonnet-thinking', 'claude-sonnet-4-5'),
    ('claude-sonnet-5-thinking-medium', 'claude-sonnet-5'),
    ('claude-sonnet-5-thinking-high', 'claude-sonnet-5'),
    ('Gemini 3.1 Pro (High)', 'gemini-3.1-pro'),
    ('Gemini 3.1 Pro (Low)', 'gemini-3.1-pro'),
    ('gemini-3.1-pro-low', 'gemini-3.1-pro'),
    ('Gemini 3.6 Flash (High)', 'gemini-3.6-flash'),
    ('Gemini 3.6 Flash (Medium)', 'gemini-3.6-flash'),
    ('Gemini 3.6 Flash (Low)', 'gemini-3.6-flash'),
    ('gemini-3.6-flash-tiered', 'gemini-3.6-flash'),
    ('cursor-grok-4.6-high', 'grok-4.6'),
    ('cursor-grok-4.6-medium', 'grok-4.6')
ON CONFLICT (alias) DO UPDATE SET model_key = EXCLUDED.model_key;

-- Usage entries with optional rated USD from the rate card.
-- provider_cost remains the vendor-reported signal; rated_cost_usd is from model_costs.
CREATE OR REPLACE VIEW v_burn_usage_rated AS
WITH resolved AS (
    SELECT
        s.*,
        COALESCE(a.model_key, s.model) AS rate_model_key
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
    r.tokens_reasoning,
    r.tokens_total,
    r.provider_cost,
    r.billing_regime,
    r.usage_detail,
    r.ingested_at,
    r.rate_model_key,
    c.id AS model_cost_id,
    c.provider AS rate_provider,
    c.display_name AS rate_display_name,
    c.input_usd_per_mtok,
    c.output_usd_per_mtok,
    c.cache_read_usd_per_mtok,
    c.cache_write_usd_per_mtok,
    c.reasoning_usd_per_mtok,
    CASE
        WHEN c.id IS NULL THEN NULL
        WHEN c.input_usd_per_mtok IS NULL
         AND c.output_usd_per_mtok IS NULL
         AND c.cache_read_usd_per_mtok IS NULL
         AND c.cache_write_usd_per_mtok IS NULL
         AND c.reasoning_usd_per_mtok IS NULL THEN NULL
        ELSE (
            COALESCE(r.tokens_input, 0) * COALESCE(c.input_usd_per_mtok, 0)
          + COALESCE(r.tokens_output, 0) * COALESCE(c.output_usd_per_mtok, 0)
          + COALESCE(r.tokens_cache_read, 0) * COALESCE(c.cache_read_usd_per_mtok, 0)
          + COALESCE(r.tokens_cache_write, 0) * COALESCE(c.cache_write_usd_per_mtok, 0)
          + COALESCE(r.tokens_reasoning, 0) * COALESCE(
                c.reasoning_usd_per_mtok,
                c.output_usd_per_mtok,
                0
            )
        ) / 1000000.0
    END AS rated_cost_usd
FROM resolved r
LEFT JOIN LATERAL (
    SELECT c.*
    FROM model_costs c
    WHERE c.model_key = r.rate_model_key
      AND COALESCE(r.last_event_at, r.started_at, r.ingested_at) >= c.effective_from
      AND (c.effective_to IS NULL OR COALESCE(r.last_event_at, r.started_at, r.ingested_at) < c.effective_to)
    ORDER BY c.effective_from DESC
    LIMIT 1
) c ON TRUE;
