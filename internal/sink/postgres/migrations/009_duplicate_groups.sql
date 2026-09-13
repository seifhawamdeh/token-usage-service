-- +migrate Up
-- Recomputed wholesale after every ingest run (see internal/dedup). Not
-- appended to like the ledger: this table always reflects the latest pass.
CREATE TABLE IF NOT EXISTS burn_duplicate_groups (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    basis TEXT NOT NULL,
    confidence NUMERIC NOT NULL,
    canonical_source_id TEXT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS burn_duplicate_members (
    group_id BIGINT NOT NULL REFERENCES burn_duplicate_groups (id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES burn_snapshots (source_id) ON DELETE CASCADE,
    is_canonical BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (source_id)
);

CREATE INDEX IF NOT EXISTS burn_duplicate_members_group_idx
    ON burn_duplicate_members (group_id);

DROP VIEW IF EXISTS v_burn_usage_rated;
CREATE VIEW v_burn_usage_rated AS
WITH resolved AS (
    SELECT
        s.*,
        pr.remote_url,
        COALESCE(proj_rem.project, proj_cwd.project) AS project,
        COALESCE(a.model_key, s.model) AS rate_model_key,
        COALESCE(s.last_event_at, s.started_at, s.ingested_at) AS rate_as_of,
        NULLIF((s.usage_detail->>'prompt_tokens_for_rate')::bigint, 0) AS prompt_tokens_for_rate,
        dm.group_id AS duplicate_group_id,
        COALESCE(dm.is_canonical, true) AS is_canonical,
        dg.basis AS duplicate_basis,
        dg.confidence AS duplicate_confidence
    FROM burn_snapshots s
    LEFT JOIN path_remotes pr ON pr.host_id = s.host_id AND pr.source_path = s.source_path
    LEFT JOIN project_remotes proj_rem ON proj_rem.remote_url = pr.remote_url
    LEFT JOIN project_cwds proj_cwd ON proj_cwd.host_id = s.host_id AND proj_cwd.cwd = s.cwd
    LEFT JOIN model_cost_aliases a ON a.alias = s.model
    LEFT JOIN burn_duplicate_members dm ON dm.source_id = s.source_id
    LEFT JOIN burn_duplicate_groups dg ON dg.id = dm.group_id
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
    r.remote_url,
    r.project,
    r.rate_model_key,
    r.rate_as_of,
    r.duplicate_group_id,
    r.is_canonical,
    r.duplicate_basis,
    r.duplicate_confidence,
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
