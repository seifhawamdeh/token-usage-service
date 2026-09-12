-- Historical multi-provider model_costs catalog (USD / 1M tokens).
-- Each row is a dated price period: [effective_from, effective_to).
-- v_burn_usage_rated picks the period covering last_event_at/started_at/ingested_at.
-- Replaces prior catalog/seed rows so ranges stay consistent.

DELETE FROM model_costs WHERE source IN (
  'anthropic-docs', 'seed', 'catalog-2026-09', 'catalog-hist-2026-09',
  'openai-api', 'openai-chatgpt-rate-card', 'google-ai', 'google-blog', 'xai-docs'
);

INSERT INTO model_costs (
    model_key, provider, display_name,
    input_usd_per_mtok, output_usd_per_mtok,
    cache_read_usd_per_mtok, cache_write_usd_per_mtok, reasoning_usd_per_mtok,
    effective_from, effective_to, source, notes
) VALUES
    ('<synthetic>', 'anthropic', 'Synthetic', 0, 0, 0, 0, 0, TIMESTAMPTZ '2024-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'non-billable'),
    ('Claude Sonnet 4.6 (Thinking)', 'anthropic', 'Claude Sonnet 4.6 (Thinking)', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-3-5-haiku-20241022', 'anthropic', 'Claude 3.5 Haiku', 0.8, 4, 0.08, 1, 4, TIMESTAMPTZ '2024-10-22 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-3-5-sonnet-20240620', 'anthropic', 'Claude 3.5 Sonnet', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2024-06-20 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-3-5-sonnet-20241022', 'anthropic', 'Claude 3.5 Sonnet', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2024-10-22 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-3-haiku-20240307', 'anthropic', 'Claude 3 Haiku', 0.25, 1.25, 0.03, 0.3, 1.25, TIMESTAMPTZ '2024-03-07 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-3-opus-20240229', 'anthropic', 'Claude 3 Opus', 15, 75, 1.5, 18.75, 75, TIMESTAMPTZ '2024-02-29 00:00:00+00', NULL, 'catalog-hist-2026-09', 'list until retired'),
    ('claude-3-sonnet-20240229', 'anthropic', 'Claude 3 Sonnet', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2024-02-29 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-4-sonnet', 'anthropic', 'Claude Sonnet 4', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-05-22 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Sonnet 4 list'),
    ('claude-4-sonnet-thinking', 'anthropic', 'Claude Sonnet 4 Thinking', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-05-22 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Sonnet 4 list'),
    ('claude-4.5-sonnet-thinking', 'anthropic', 'Claude 4.5 Sonnet Thinking', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-09-29 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list; stable'),
    ('claude-fable-5', 'anthropic', 'Claude Fable 5', 10, 50, 1, 12.5, 50, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-fable-5.1', 'anthropic', 'Claude Fable 5.1', 10, 50, 0.25, 12.5, 50, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-haiku-3-5', 'anthropic', 'Claude Haiku 3.5', 0.8, 4, 0.08, 1, 4, TIMESTAMPTZ '2024-10-22 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-haiku-4-5', 'anthropic', 'Claude Haiku 4.5', 1, 5, 0.1, 1.25, 5, TIMESTAMPTZ '2025-10-15 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list'),
    ('claude-haiku-4-5-20251001', 'anthropic', 'Claude Haiku 4.5 (20251001)', 1, 5, 0.1, 1.25, 5, TIMESTAMPTZ '2025-10-15 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list'),
    ('claude-mythos-5', 'anthropic', 'Claude Mythos 5', 10, 50, 1, 12.5, 50, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-mythos-5.1', 'anthropic', 'Claude Mythos 5.1', 10, 50, 0.25, 12.5, 50, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-opus-4', 'anthropic', 'Claude Opus 4', 15, 75, 1.5, 18.75, 75, TIMESTAMPTZ '2025-05-22 00:00:00+00', TIMESTAMPTZ '2025-11-24 00:00:00+00', 'catalog-hist-2026-09', 'pre-Opus-4.5 list'),
    ('claude-opus-4', 'anthropic', 'Claude Opus 4', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'aligned to Opus 4.5 list after cut'),
    ('claude-opus-4-1', 'anthropic', 'Claude Opus 4.1', 15, 75, 1.5, 18.75, 75, TIMESTAMPTZ '2025-08-01 00:00:00+00', TIMESTAMPTZ '2025-11-24 00:00:00+00', 'catalog-hist-2026-09', 'pre-Opus-4.5 list'),
    ('claude-opus-4-1', 'anthropic', 'Claude Opus 4.1', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'aligned to Opus 4.5 list after cut'),
    ('claude-opus-4-5', 'anthropic', 'Claude Opus 4.5', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-5-thinking', 'anthropic', 'Claude Opus 4.5 Thinking', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-6', 'anthropic', 'Claude Opus 4.6', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-6-thinking', 'anthropic', 'Claude Opus 4.6 Thinking', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-7', 'anthropic', 'Claude Opus 4.7', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-8', 'anthropic', 'Claude Opus 4.8', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4-thinking', 'anthropic', 'Claude Opus 4 Thinking', 15, 75, 1.5, 18.75, 75, TIMESTAMPTZ '2025-05-22 00:00:00+00', TIMESTAMPTZ '2025-11-24 00:00:00+00', 'catalog-hist-2026-09', 'pre-Opus-4.5 list'),
    ('claude-opus-4-thinking', 'anthropic', 'Claude Opus 4 Thinking', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'aligned to Opus 4.5 list after cut'),
    ('claude-opus-4.5', 'anthropic', 'Claude Opus 4.5', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2025-11-24 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4.6', 'anthropic', 'Claude Opus 4.6', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4.7', 'anthropic', 'Claude Opus 4.7', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-4.8', 'anthropic', 'Claude Opus 4.8', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-opus-5', 'anthropic', 'Claude Opus 5', 5, 25, 0.5, 6.25, 25, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Opus $5/$25 list'),
    ('claude-sonnet-4', 'anthropic', 'Claude Sonnet 4', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-05-22 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Sonnet 4 list'),
    ('claude-sonnet-4-5', 'anthropic', 'Claude Sonnet 4.5', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-09-29 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list; stable'),
    ('claude-sonnet-4-5-thinking', 'anthropic', 'Claude Sonnet 4.5 Thinking', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-09-29 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list; stable'),
    ('claude-sonnet-4-6', 'anthropic', 'Claude Sonnet 4.6', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-sonnet-4-6-thinking', 'anthropic', 'Claude Sonnet 4.6 Thinking', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-sonnet-4-thinking', 'anthropic', 'Claude Sonnet 4 Thinking', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-05-22 00:00:00+00', NULL, 'catalog-hist-2026-09', 'Sonnet 4 list'),
    ('claude-sonnet-4.5', 'anthropic', 'Claude Sonnet 4.5', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2025-09-29 00:00:00+00', NULL, 'catalog-hist-2026-09', 'launch list; stable'),
    ('claude-sonnet-4.6', 'anthropic', 'Claude Sonnet 4.6', 3, 15, 0.3, 3.75, 15, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('claude-sonnet-5', 'anthropic', 'Claude Sonnet 5', 2, 10, 0.2, 2.5, 10, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'intro became standard; scheduled $3/$15 hike cancelled'),
    ('claude-sonnet-5-thinking', 'anthropic', 'Claude Sonnet 5 Thinking', 2, 10, 0.2, 2.5, 10, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'intro became standard; scheduled $3/$15 hike cancelled'),
    ('claude-sonnet-5-thinking-high', 'anthropic', 'Claude Sonnet 5 Thinking High', 2, 10, 0.2, 2.5, 10, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'intro became standard; scheduled $3/$15 hike cancelled'),
    ('claude-sonnet-5-thinking-low', 'anthropic', 'Claude Sonnet 5 Thinking Low', 2, 10, 0.2, 2.5, 10, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'intro became standard; scheduled $3/$15 hike cancelled'),
    ('claude-sonnet-5-thinking-medium', 'anthropic', 'Claude Sonnet 5 Thinking Medium', 2, 10, 0.2, 2.5, 10, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'intro became standard; scheduled $3/$15 hike cancelled'),
    ('auto', 'cursor', 'auto', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('composer-1', 'cursor', 'composer-1', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('composer-1.5', 'cursor', 'composer-1.5', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('composer-2', 'cursor', 'composer-2', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('composer-2.5', 'cursor', 'composer-2.5', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('composer-2.5-fast', 'cursor', 'composer-2.5-fast', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('cursor-grok-4.6-high', 'cursor', 'cursor-grok-4.6-high', 2, 6, 0.5, NULL, 6, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'priced as grok-4.6 API'),
    ('cursor-grok-4.6-low', 'cursor', 'cursor-grok-4.6-low', 2, 6, 0.5, NULL, 6, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'priced as grok-4.6 API'),
    ('cursor-grok-4.6-medium', 'cursor', 'cursor-grok-4.6-medium', 2, 6, 0.5, NULL, 6, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'priced as grok-4.6 API'),
    ('default', 'cursor', 'default', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'subscription SKU; no public $/MTok'),
    ('copilot', 'github', 'GitHub Copilot', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2024-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'plan-based'),
    ('Gemini 3.1 Pro (High)', 'google', 'Gemini 3.1 Pro (High)', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('Gemini 3.1 Pro (Low)', 'google', 'Gemini 3.1 Pro (Low)', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('Gemini 3.6 Flash (High)', 'google', 'Gemini 3.6 Flash (High)', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('Gemini 3.6 Flash (Low)', 'google', 'Gemini 3.6 Flash (Low)', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('Gemini 3.6 Flash (Medium)', 'google', 'Gemini 3.6 Flash (Medium)', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-1.5-flash', 'google', 'Gemini 1.5 Flash', 0.075, 0.3, NULL, NULL, 0.3, TIMESTAMPTZ '2024-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-1.5-pro', 'google', 'Gemini 1.5 Pro', 1.25, 5, NULL, NULL, 5, TIMESTAMPTZ '2024-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-2.0-flash', 'google', 'Gemini 2.0 Flash', 0.1, 0.4, NULL, NULL, 0.4, TIMESTAMPTZ '2024-12-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-2.0-flash-lite', 'google', 'Gemini 2.0 Flash-Lite', 0.075, 0.3, NULL, NULL, 0.3, TIMESTAMPTZ '2024-12-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-2.5-flash', 'google', 'Gemini 2.5 Flash', 0.3, 2.5, NULL, NULL, 2.5, TIMESTAMPTZ '2025-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-2.5-flash-lite', 'google', 'Gemini 2.5 Flash-Lite', 0.1, 0.4, NULL, NULL, 0.4, TIMESTAMPTZ '2025-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-2.5-pro', 'google', 'Gemini 2.5 Pro', 1.25, 10, NULL, NULL, 10, TIMESTAMPTZ '2025-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '≤200k tier'),
    ('gemini-3-flash', 'google', 'Gemini 3 Flash', 0.5, 3, NULL, NULL, 3, TIMESTAMPTZ '2025-12-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3-flash-preview', 'google', 'Gemini 3 Flash Preview', 0.5, 3, NULL, NULL, 3, TIMESTAMPTZ '2025-12-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.1-flash-lite', 'google', 'Gemini 3.1 Flash-Lite', 0.25, 1.5, NULL, NULL, 1.5, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.1-pro', 'google', 'Gemini 3.1 Pro', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '≤200k tier'),
    ('gemini-3.1-pro-high', 'google', 'Gemini 3.1 Pro High', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.1-pro-low', 'google', 'Gemini 3.1 Pro Low', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.1-pro-preview', 'google', 'Gemini 3.1 Pro Preview', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.5-flash', 'google', 'Gemini 3.5 Flash', 1.5, 9, NULL, NULL, 9, TIMESTAMPTZ '2026-05-19 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.5-flash-lite', 'google', 'Gemini 3.5 Flash-Lite', 0.3, 2.5, NULL, NULL, 2.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.6-flash', 'google', 'Gemini 3.6 Flash', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.6-flash-tiered', 'google', 'Gemini 3.6 Flash tiered', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gemini-3.8-flash', 'google', 'Gemini 3.8 Flash', 1.5, 7.5, NULL, NULL, 7.5, TIMESTAMPTZ '2026-09-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'assumed 3.6 Flash list until published'),
    ('gemini-pro', 'google', 'Gemini Pro', 0.5, 1.5, NULL, NULL, 1.5, TIMESTAMPTZ '2024-01-01 00:00:00+00', TIMESTAMPTZ '2025-05-01 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('gemini-pro-default', 'google', 'Gemini Pro default', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-02-01 00:00:00+00', NULL, 'catalog-hist-2026-09', 'priced as 3.1 Pro'),
    ('daybreak-blue', 'openai', 'Daybreak Blue', 4, 20, 0.4, NULL, 20, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('daybreak-red', 'openai', 'Daybreak Red', 12.5, 75, 1.25, NULL, 75, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-3.5-turbo', 'openai', 'GPT-3.5 Turbo', 0.5, 1.5, NULL, NULL, 1.5, TIMESTAMPTZ '2024-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4', 'openai', 'GPT-4', 30, 60, NULL, NULL, 60, TIMESTAMPTZ '2023-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4-turbo', 'openai', 'GPT-4 Turbo', 10, 30, NULL, NULL, 30, TIMESTAMPTZ '2024-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4.1', 'openai', 'GPT-4.1', 2, 8, 0.5, NULL, 8, TIMESTAMPTZ '2025-04-14 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4.1-mini', 'openai', 'GPT-4.1 mini', 0.4, 1.6, 0.1, NULL, 1.6, TIMESTAMPTZ '2025-04-14 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4.1-nano', 'openai', 'GPT-4.1 nano', 0.1, 0.4, 0.025, NULL, 0.4, TIMESTAMPTZ '2025-04-14 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4o', 'openai', 'GPT-4o', 2.5, 10, 1.25, NULL, 10, TIMESTAMPTZ '2024-05-13 00:00:00+00', NULL, 'openai-api', ''),
    ('gpt-4o-2024-08-06', 'openai', 'GPT-4o 2024-08-06', 2.5, 10, 1.25, NULL, 10, TIMESTAMPTZ '2024-08-06 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-4o-mini', 'openai', 'GPT-4o mini', 0.15, 0.6, 0.075, NULL, 0.6, TIMESTAMPTZ '2024-07-18 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5', 'openai', 'GPT-5', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5-chat-latest', 'openai', 'GPT-5 chat latest', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5-codex', 'openai', 'GPT-5 Codex', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5-mini', 'openai', 'GPT-5 mini', 0.25, 2, 0.025, NULL, 2, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5-nano', 'openai', 'GPT-5 nano', 0.05, 0.4, 0.005, NULL, 0.4, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5-pro', 'openai', 'GPT-5 pro', 15, 120, NULL, NULL, 120, TIMESTAMPTZ '2025-08-07 00:00:00+00', NULL, 'catalog-hist-2026-09', 'GPT-5 launch'),
    ('gpt-5.1', 'openai', 'GPT-5.1', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-11-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.1-codex', 'openai', 'GPT-5.1 Codex', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-11-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.1-codex-max', 'openai', 'GPT-5.1 Codex Max', 1.25, 10, 0.125, NULL, 10, TIMESTAMPTZ '2025-11-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.2', 'openai', 'GPT-5.2', 1.75, 14, 0.175, NULL, 14, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.2-codex', 'openai', 'GPT-5.2 Codex', 1.75, 14, 0.175, NULL, 14, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.2-pro', 'openai', 'GPT-5.2 Pro', 21, 168, NULL, NULL, 168, TIMESTAMPTZ '2026-01-15 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.3', 'openai', 'GPT-5.3', 1.75, 14, 0.175, NULL, 14, TIMESTAMPTZ '2026-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.3-codex', 'openai', 'GPT-5.3 Codex', 1.75, 14, 0.175, NULL, 14, TIMESTAMPTZ '2026-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.4', 'openai', 'GPT-5.4', 2.5, 15, 0.25, NULL, 15, TIMESTAMPTZ '2026-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.4-mini', 'openai', 'GPT-5.4 Mini', 0.75, 4.5, 0.075, NULL, 4.5, TIMESTAMPTZ '2026-05-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.5', 'openai', 'GPT-5.5', 5, 30, 0.5, NULL, 30, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.5-pro', 'openai', 'GPT-5.5 Pro', 30, 180, NULL, NULL, 180, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('gpt-5.6', 'openai', 'GPT-5.6', 5, 30, 0.5, NULL, 30, TIMESTAMPTZ '2026-07-09 00:00:00+00', TIMESTAMPTZ '2026-08-21 00:00:00+00', 'catalog-hist-2026-09', 'alias Sol'),
    ('gpt-5.6', 'openai', 'GPT-5.6', 4, 20, 0.4, NULL, 20, TIMESTAMPTZ '2026-08-21 00:00:00+00', NULL, 'catalog-hist-2026-09', 'alias Sol promo'),
    ('gpt-5.6-luna', 'openai', 'GPT-5.6 Luna', 1, 6, 0.1, NULL, 6, TIMESTAMPTZ '2026-07-09 00:00:00+00', TIMESTAMPTZ '2026-07-30 00:00:00+00', 'catalog-hist-2026-09', 'launch list'),
    ('gpt-5.6-luna', 'openai', 'GPT-5.6 Luna', 0.2, 1.2, 0.02, NULL, 1.2, TIMESTAMPTZ '2026-07-30 00:00:00+00', NULL, 'catalog-hist-2026-09', '-80% from 2026-07-30'),
    ('gpt-5.6-sol', 'openai', 'GPT-5.6 Sol', 5, 30, 0.5, NULL, 30, TIMESTAMPTZ '2026-07-09 00:00:00+00', TIMESTAMPTZ '2026-08-21 00:00:00+00', 'catalog-hist-2026-09', 'launch list'),
    ('gpt-5.6-sol', 'openai', 'GPT-5.6 Sol', 4, 20, 0.4, NULL, 20, TIMESTAMPTZ '2026-08-21 00:00:00+00', NULL, 'catalog-hist-2026-09', '≥20% promo / rate-card'),
    ('gpt-5.6-terra', 'openai', 'GPT-5.6 Terra', 2.5, 15, 0.25, NULL, 15, TIMESTAMPTZ '2026-07-09 00:00:00+00', TIMESTAMPTZ '2026-07-30 00:00:00+00', 'catalog-hist-2026-09', 'launch list'),
    ('gpt-5.6-terra', 'openai', 'GPT-5.6 Terra', 2, 12, 0.2, NULL, 12, TIMESTAMPTZ '2026-07-30 00:00:00+00', NULL, 'catalog-hist-2026-09', '-20% from 2026-07-30'),
    ('gpt-6-astra', 'openai', 'GPT-6 Astra', 10, 50, 1, NULL, 50, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o1', 'openai', 'o1', 15, 60, 7.5, NULL, 60, TIMESTAMPTZ '2024-12-05 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o1-mini', 'openai', 'o1-mini', 1.1, 4.4, NULL, NULL, 4.4, TIMESTAMPTZ '2024-09-12 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o1-pro', 'openai', 'o1-pro', 150, 600, NULL, NULL, 600, TIMESTAMPTZ '2024-12-05 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o3', 'openai', 'o3', 2, 8, 0.5, NULL, 8, TIMESTAMPTZ '2025-04-16 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o3-deep-research', 'openai', 'o3-deep-research', 10, 40, 2.5, NULL, 40, TIMESTAMPTZ '2025-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o3-mini', 'openai', 'o3-mini', 1.1, 4.4, 0.55, NULL, 4.4, TIMESTAMPTZ '2025-01-31 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o3-pro', 'openai', 'o3-pro', 20, 80, NULL, NULL, 80, TIMESTAMPTZ '2025-04-16 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o4-mini', 'openai', 'o4-mini', 1.1, 4.4, 0.275, NULL, 4.4, TIMESTAMPTZ '2025-04-16 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('o4-mini-deep-research', 'openai', 'o4-mini-deep-research', 2, 8, 0.5, NULL, 8, TIMESTAMPTZ '2025-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('big-pickle', 'opencode', 'big-pickle', NULL, NULL, NULL, NULL, NULL, TIMESTAMPTZ '2025-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', ''),
    ('grok-2', 'xai', 'Grok 2', 2, 10, NULL, NULL, 10, TIMESTAMPTZ '2024-08-01 00:00:00+00', TIMESTAMPTZ '2025-07-01 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('grok-2-mini', 'xai', 'Grok 2 Mini', 0.3, 0.5, NULL, NULL, 0.5, TIMESTAMPTZ '2024-08-01 00:00:00+00', TIMESTAMPTZ '2025-07-01 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('grok-3', 'xai', 'Grok 3', 3, 15, NULL, NULL, 15, TIMESTAMPTZ '2025-02-01 00:00:00+00', TIMESTAMPTZ '2025-07-09 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('grok-3', 'xai', 'Grok 3', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-3-mini', 'xai', 'Grok 3 Mini', 0.3, 0.5, NULL, NULL, 0.5, TIMESTAMPTZ '2025-02-01 00:00:00+00', TIMESTAMPTZ '2025-07-09 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('grok-3-mini', 'xai', 'Grok 3 Mini', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4', 'xai', 'Grok 4', 3, 15, NULL, NULL, 15, TIMESTAMPTZ '2025-07-09 00:00:00+00', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'catalog-hist-2026-09', 'early grok-4 list'),
    ('grok-4', 'xai', 'Grok 4', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-0709', 'xai', 'Grok 4 0709', 3, 15, NULL, NULL, 15, TIMESTAMPTZ '2025-07-09 00:00:00+00', TIMESTAMPTZ '2026-01-01 00:00:00+00', 'catalog-hist-2026-09', ''),
    ('grok-4-0709', 'xai', 'Grok 4 0709', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-1-fast', 'xai', 'Grok 4.1 Fast', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-1-fast-reasoning', 'xai', 'Grok 4.1 Fast Reasoning', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-fast', 'xai', 'Grok 4 Fast', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-fast-non-reasoning', 'xai', 'Grok 4 Fast Non-Reasoning', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4-fast-reasoning', 'xai', 'Grok 4 Fast Reasoning', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.20-0309-non-reasoning', 'xai', 'Grok 4.20 Non-Reasoning', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.20-0309-reasoning', 'xai', 'Grok 4.20 Reasoning', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.20-multi-agent-0309', 'xai', 'Grok 4.20 Multi-Agent', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.3', 'xai', 'Grok 4.3', 1.25, 2.5, 0.2, NULL, 2.5, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.5', 'xai', 'Grok 4.5', 2, 6, 0.3, NULL, 6, TIMESTAMPTZ '2026-06-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-4.6', 'xai', 'Grok 4.6', 2, 6, 0.5, NULL, 6, TIMESTAMPTZ '2026-08-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '<200k tier'),
    ('grok-build-0.1', 'xai', 'Grok Build 0.1', 1, 2, 0.2, NULL, 2, TIMESTAMPTZ '2026-03-01 00:00:00+00', NULL, 'catalog-hist-2026-09', '');

INSERT INTO model_cost_aliases (alias, model_key) VALUES
    ('Claude Sonnet 4.5 (Thinking)', 'claude-sonnet-4-5'),
    ('Claude Sonnet 4.6 (Thinking)', 'claude-sonnet-4-6'),
    ('Gemini 3.1 Pro (High)', 'gemini-3.1-pro'),
    ('Gemini 3.1 Pro (Low)', 'gemini-3.1-pro'),
    ('Gemini 3.6 Flash (High)', 'gemini-3.6-flash'),
    ('Gemini 3.6 Flash (Low)', 'gemini-3.6-flash'),
    ('Gemini 3.6 Flash (Medium)', 'gemini-3.6-flash'),
    ('claude-3-5-haiku-latest', 'claude-3-5-haiku-20241022'),
    ('claude-3-5-sonnet-latest', 'claude-3-5-sonnet-20241022'),
    ('claude-4.5-sonnet-thinking', 'claude-sonnet-4-5'),
    ('claude-haiku-4-5-20251001', 'claude-haiku-4-5'),
    ('claude-opus-4-5-thinking', 'claude-opus-4-5'),
    ('claude-opus-4-6-thinking', 'claude-opus-4-6'),
    ('claude-sonnet-5-thinking-high', 'claude-sonnet-5'),
    ('claude-sonnet-5-thinking-low', 'claude-sonnet-5'),
    ('claude-sonnet-5-thinking-medium', 'claude-sonnet-5'),
    ('cursor-grok-4.6-high', 'grok-4.6'),
    ('cursor-grok-4.6-low', 'grok-4.6'),
    ('cursor-grok-4.6-medium', 'grok-4.6'),
    ('gemini-3.1-pro-high', 'gemini-3.1-pro'),
    ('gemini-3.1-pro-low', 'gemini-3.1-pro'),
    ('gemini-3.6-flash-tiered', 'gemini-3.6-flash'),
    ('gemini-pro-default', 'gemini-3.1-pro'),
    ('gpt-5.6', 'gpt-5.6-sol'),
    ('grok-4-latest', 'grok-4.6'),
    ('grok-latest', 'grok-4.6')
ON CONFLICT (alias) DO UPDATE SET model_key = EXCLUDED.model_key;

-- Resolve rate as-of burn time (exclusive effective_to).
DROP VIEW IF EXISTS v_burn_usage_rated;
CREATE VIEW v_burn_usage_rated AS
WITH resolved AS (
    SELECT
        s.*,
        COALESCE(a.model_key, s.model) AS rate_model_key,
        COALESCE(s.last_event_at, s.started_at, s.ingested_at) AS rate_as_of
    FROM burn_snapshots s
    LEFT JOIN model_cost_aliases a ON a.alias = s.model
)
SELECT
    r.source_id, r.host_id, r.vendor, r.source_path, r.stable_id,
    r.provider_session_id, r.cwd, r.started_at, r.last_event_at,
    r.model, r.models, r.model_provider,
    r.tokens_input, r.tokens_output, r.tokens_cache_read,
    r.tokens_cache_write, r.tokens_reasoning, r.tokens_total,
    r.provider_cost, r.billing_regime, r.usage_detail, r.ingested_at,
    r.rate_model_key, r.rate_as_of,
    c.id AS model_cost_id,
    c.provider AS rate_provider,
    c.display_name AS rate_display_name,
    c.effective_from AS rate_effective_from,
    c.effective_to AS rate_effective_to,
    c.input_usd_per_mtok, c.output_usd_per_mtok,
    c.cache_read_usd_per_mtok, c.cache_write_usd_per_mtok,
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

