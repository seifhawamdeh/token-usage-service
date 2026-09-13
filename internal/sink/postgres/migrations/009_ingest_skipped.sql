-- +migrate Up
ALTER TABLE burn_ingest_runs
    ADD COLUMN IF NOT EXISTS skipped INT NOT NULL DEFAULT 0;
