package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/seif/token-usage-service/internal/model"
	"github.com/seif/token-usage-service/internal/sink"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Sink struct {
	db *sql.DB
}

func Open(databaseURL string) (*Sink, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Sink{db: db}, nil
}

func (s *Sink) Name() string { return "postgres" }

func (s *Sink) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Sink) Migrate(ctx context.Context) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}

	for _, name := range names {
		var exists bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sink) LookupCheckpoints(ctx context.Context, hostID, vendor string, paths []string) (map[string]model.Checkpoint, error) {
	out := make(map[string]model.Checkpoint)
	if len(paths) == 0 {
		return out, nil
	}
	const chunk = 200
	for i := 0; i < len(paths); i += chunk {
		j := i + chunk
		if j > len(paths) {
			j = len(paths)
		}
		part := paths[i:j]
		placeholders := make([]string, len(part))
		args := make([]any, 0, 2+len(part))
		args = append(args, hostID, vendor)
		for k, p := range part {
			placeholders[k] = fmt.Sprintf("$%d", k+3)
			args = append(args, p)
		}
		q := fmt.Sprintf(`
			SELECT source_id, host_id, vendor, source_path, mtime_ns, size_bytes, processing_signature
			FROM burn_checkpoints
			WHERE host_id=$1 AND vendor=$2 AND source_path IN (%s)`, strings.Join(placeholders, ","))
		rows, err := s.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cp model.Checkpoint
			if err := rows.Scan(&cp.SourceID, &cp.HostID, &cp.Vendor, &cp.SourcePath, &cp.MtimeNs, &cp.SizeBytes, &cp.ProcessingSignature); err != nil {
				rows.Close()
				return nil, err
			}
			out[cp.SourcePath] = cp
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Sink) UpsertSnapshotAndCheckpoint(ctx context.Context, snap model.BurnSnapshot, cp model.Checkpoint) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	previous, err := loadPreviousUsage(ctx, tx, snap.SourceID)
	if err != nil {
		return fmt.Errorf("load previous usage: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO burn_snapshots (
			source_id, identity_version, host_id, vendor, source_path, stable_id,
			provider_session_id, cwd, started_at, last_event_at,
			model, models, model_provider,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_cache_write_5m, tokens_cache_write_1h,
			tokens_reasoning, tokens_total,
			provider_cost, billing_regime, usage_detail,
			adapter_version, snapshot_schema_version, parse_status, ingested_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,
			NULLIF($7,''), NULLIF($8,''), $9, $10,
			NULLIF($11,''), $12, NULLIF($13,''),
			$14,$15,$16,$17,
			$18,$19,
			$20,$21,
			$22, NULLIF($23,''), $24,
			$25,$26,$27,$28
		)
		ON CONFLICT (source_id) DO UPDATE SET
			identity_version = EXCLUDED.identity_version,
			stable_id = EXCLUDED.stable_id,
			provider_session_id = EXCLUDED.provider_session_id,
			cwd = EXCLUDED.cwd,
			started_at = EXCLUDED.started_at,
			last_event_at = EXCLUDED.last_event_at,
			model = EXCLUDED.model,
			models = EXCLUDED.models,
			model_provider = EXCLUDED.model_provider,
			tokens_input = EXCLUDED.tokens_input,
			tokens_output = EXCLUDED.tokens_output,
			tokens_cache_read = EXCLUDED.tokens_cache_read,
			tokens_cache_write = EXCLUDED.tokens_cache_write,
			tokens_cache_write_5m = EXCLUDED.tokens_cache_write_5m,
			tokens_cache_write_1h = EXCLUDED.tokens_cache_write_1h,
			tokens_reasoning = EXCLUDED.tokens_reasoning,
			tokens_total = EXCLUDED.tokens_total,
			provider_cost = EXCLUDED.provider_cost,
			billing_regime = EXCLUDED.billing_regime,
			usage_detail = EXCLUDED.usage_detail,
			adapter_version = EXCLUDED.adapter_version,
			snapshot_schema_version = EXCLUDED.snapshot_schema_version,
			parse_status = EXCLUDED.parse_status,
			ingested_at = EXCLUDED.ingested_at
	`,
		snap.SourceID, snap.IdentityVersion, snap.HostID, snap.Vendor, snap.SourcePath, snap.StableID,
		snap.ProviderSessionID, snap.CWD, snap.StartedAt, snap.LastEventAt,
		snap.Model, pqTextArray(snap.Models), snap.ModelProvider,
		snap.Tokens.Input, snap.Tokens.Output, snap.Tokens.CacheRead, snap.Tokens.CacheWrite,
		snap.Tokens.CacheWrite5m, snap.Tokens.CacheWrite1h,
		snap.Tokens.Reasoning, snap.Tokens.Total,
		snap.ProviderCost, snap.BillingRegime, nullJSON(snap.UsageDetail),
		snap.AdapterVersion, snap.SnapshotSchemaVer, snap.ParseStatus, snap.IngestedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	currentRated, err := ratedCost(ctx, tx, snap.SourceID)
	if err != nil {
		return fmt.Errorf("rate snapshot: %w", err)
	}
	occurredAt := snap.IngestedAt
	if snap.LastEventAt != nil {
		occurredAt = *snap.LastEventAt
	} else if snap.StartedAt != nil {
		occurredAt = *snap.StartedAt
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO burn_usage_ledger (
			source_id, host_id, vendor, source_path, provider_session_id, cwd, model, occurred_at, recorded_at,
			delta_tokens_input, delta_tokens_output, delta_tokens_cache_read, delta_tokens_cache_write,
			delta_tokens_cache_write_5m, delta_tokens_cache_write_1h, delta_tokens_reasoning, delta_tokens_total,
			delta_provider_cost, delta_rated_cost_usd,
			total_tokens_input, total_tokens_output, total_tokens_cache_read, total_tokens_cache_write,
			total_tokens_cache_write_5m, total_tokens_cache_write_1h, total_tokens_reasoning, total_tokens_total,
			total_provider_cost, total_rated_cost_usd, usage_detail
		) VALUES (
			$1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,
			$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
			$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30
		)`,
		snap.SourceID, snap.HostID, snap.Vendor, snap.SourcePath, snap.ProviderSessionID, snap.CWD, snap.Model, occurredAt, snap.IngestedAt,
		deltaInt(snap.Tokens.Input, previous.tokens.Input), deltaInt(snap.Tokens.Output, previous.tokens.Output),
		deltaInt(snap.Tokens.CacheRead, previous.tokens.CacheRead), deltaInt(snap.Tokens.CacheWrite, previous.tokens.CacheWrite),
		deltaInt(snap.Tokens.CacheWrite5m, previous.tokens.CacheWrite5m), deltaInt(snap.Tokens.CacheWrite1h, previous.tokens.CacheWrite1h),
		deltaInt(snap.Tokens.Reasoning, previous.tokens.Reasoning), deltaInt(snap.Tokens.Total, previous.tokens.Total),
		deltaFloat(snap.ProviderCost, previous.providerCost), deltaFloat(currentRated, previous.ratedCost),
		snap.Tokens.Input, snap.Tokens.Output, snap.Tokens.CacheRead, snap.Tokens.CacheWrite,
		snap.Tokens.CacheWrite5m, snap.Tokens.CacheWrite1h, snap.Tokens.Reasoning, snap.Tokens.Total,
		snap.ProviderCost, currentRated, nullJSON(snap.UsageDetail),
	)
	if err != nil {
		return fmt.Errorf("insert usage ledger: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO burn_checkpoints (
			source_id, host_id, vendor, source_path, mtime_ns, size_bytes, processing_signature, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7, now())
		ON CONFLICT (source_id) DO UPDATE SET
			mtime_ns = EXCLUDED.mtime_ns,
			size_bytes = EXCLUDED.size_bytes,
			processing_signature = EXCLUDED.processing_signature,
			updated_at = now()
	`, cp.SourceID, cp.HostID, cp.Vendor, cp.SourcePath, cp.MtimeNs, cp.SizeBytes, cp.ProcessingSignature)
	if err != nil {
		return fmt.Errorf("upsert checkpoint: %w", err)
	}
	return tx.Commit()
}

type previousUsage struct {
	tokens       model.Tokens
	providerCost *float64
	ratedCost    *float64
}

func loadPreviousUsage(ctx context.Context, tx *sql.Tx, sourceID string) (previousUsage, error) {
	var p previousUsage
	var in, out, cacheRead, cacheWrite, cacheWrite5m, cacheWrite1h, reasoning, total sql.NullInt64
	var providerCost sql.NullFloat64
	err := tx.QueryRowContext(ctx, `
		SELECT tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_cache_write_5m, tokens_cache_write_1h, tokens_reasoning, tokens_total, provider_cost
		FROM burn_snapshots WHERE source_id = $1 FOR UPDATE`, sourceID,
	).Scan(&in, &out, &cacheRead, &cacheWrite, &cacheWrite5m, &cacheWrite1h, &reasoning, &total, &providerCost)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.tokens = model.Tokens{Input: intPtr(in), Output: intPtr(out), CacheRead: intPtr(cacheRead), CacheWrite: intPtr(cacheWrite), CacheWrite5m: intPtr(cacheWrite5m), CacheWrite1h: intPtr(cacheWrite1h), Reasoning: intPtr(reasoning), Total: intPtr(total)}
	p.providerCost = floatPtr(providerCost)
	rated, err := ratedCost(ctx, tx, sourceID)
	if err != nil {
		return p, err
	}
	p.ratedCost = rated
	return p, nil
}

func ratedCost(ctx context.Context, tx *sql.Tx, sourceID string) (*float64, error) {
	var value sql.NullFloat64
	if err := tx.QueryRowContext(ctx, `SELECT rated_cost_usd FROM v_burn_usage_rated WHERE source_id = $1`, sourceID).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return floatPtr(value), nil
}

func intPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func floatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func deltaInt(current, previous *int64) *int64 {
	if current == nil {
		return nil
	}
	base := int64(0)
	if previous != nil {
		base = *previous
	}
	delta := *current - base
	return &delta
}

func deltaFloat(current, previous *float64) *float64 {
	if current == nil {
		return nil
	}
	base := float64(0)
	if previous != nil {
		base = *previous
	}
	delta := *current - base
	return &delta
}

func (s *Sink) UpsertPathRemote(ctx context.Context, pr model.PathRemote) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO path_remotes (host_id, source_path, remote_url, remote_name, repo_root, resolved_at)
		VALUES ($1,$2,$3,$4,$5, now())
		ON CONFLICT (host_id, source_path) DO UPDATE SET
			remote_url = EXCLUDED.remote_url,
			remote_name = EXCLUDED.remote_name,
			repo_root = EXCLUDED.repo_root,
			resolved_at = now()
	`, pr.HostID, pr.SourcePath, pr.RemoteURL, pr.RemoteName, pr.RepoRoot)
	return err
}

func (s *Sink) StartRun(ctx context.Context, hostID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO burn_ingest_runs (host_id) VALUES ($1) RETURNING id`, hostID).Scan(&id)
	return id, err
}

func (s *Sink) FinishRun(ctx context.Context, runID int64, run model.IngestRun) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE burn_ingest_runs SET completed_at = now(), status = $2, scanned = $3, unchanged_skipped = $4,
			parsed = $5, upserted = $6, skipped = $7, deferred = $8, errors = $9, path_remotes_upserted = $10,
			fatal_error = NULLIF($11, '') WHERE id = $1`,
		runID, run.Status, run.Scanned, run.UnchangedSkipped, run.Parsed, run.Upserted,
		run.Skipped, run.Deferred, run.Errors, run.PathRemotesUpsert, run.FatalError)
	return err
}

func (s *Sink) RecordRunIssue(ctx context.Context, runID int64, issue model.IngestIssue) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO burn_ingest_issues (run_id, vendor, source_path, severity, message)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)`,
		runID, issue.Vendor, issue.SourcePath, issue.Severity, issue.Message)
	return err
}

func nullJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

// pqTextArray formats a Go string slice for pgx text[] via lib/pq-style literal.
// With database/sql + pgx stdlib, []string is accepted for text[].
func pqTextArray(ss []string) any {
	if ss == nil {
		return []string{}
	}
	return ss
}

var _ sink.Sink = (*Sink)(nil)
var _ sink.RunRecorder = (*Sink)(nil)
