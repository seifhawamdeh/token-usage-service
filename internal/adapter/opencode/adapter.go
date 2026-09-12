package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "opencode"
	Version    = "1"
)

type Adapter struct {
	DBPath string
}

func New(dbPath string) *Adapter {
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	}
	return &Adapter{DBPath: dbPath}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

type sessionRow struct {
	ID                string
	Directory         sql.NullString
	Path              sql.NullString
	Title             sql.NullString
	ModelJSON         sql.NullString
	Cost              sql.NullFloat64
	TokensInput       sql.NullInt64
	TokensOutput      sql.NullInt64
	TokensReasoning   sql.NullInt64
	TokensCacheRead   sql.NullInt64
	TokensCacheWrite  sql.NullInt64
	TimeCreated       sql.NullInt64
	TimeUpdated       sql.NullInt64
}

func (a *Adapter) openRO() (*sql.DB, error) {
	if _, err := os.Stat(a.DBPath); err != nil {
		return nil, err
	}
	// immutable-ish read; mode=ro
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", a.DBPath)
	return sql.Open("sqlite", dsn)
}

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	db, err := a.openRO()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer db.Close()

	abs, _ := filepath.Abs(a.DBPath)
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(time_updated, time_created, 0),
		       COALESCE(tokens_input,0)+COALESCE(tokens_output,0)+COALESCE(tokens_reasoning,0)+COALESCE(tokens_cache_read,0)+COALESCE(tokens_cache_write,0)
		FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.SourceDescriptor
	for rows.Next() {
		var id string
		var updatedMs, tokenSum int64
		if err := rows.Scan(&id, &updatedMs, &tokenSum); err != nil {
			return nil, err
		}
		sourcePath := abs + "#session:" + id
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: sourcePath,
			StableID:   id,
			MtimeNs:    updatedMs * int64(time.Millisecond),
			SizeBytes:  tokenSum,
		})
	}
	return out, rows.Err()
}

func (a *Adapter) Parse(ctx context.Context, src model.SourceDescriptor) model.ParseResult {
	db, err := a.openRO()
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer db.Close()

	id := src.StableID
	if id == "" {
		return model.ParseResult{Error: fmt.Errorf("missing session id in %s", src.SourcePath)}
	}

	var r sessionRow
	err = db.QueryRowContext(ctx, `
		SELECT id, directory, path, title, model, cost,
		       tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write,
		       time_created, time_updated
		FROM session WHERE id = ?`, id).Scan(
		&r.ID, &r.Directory, &r.Path, &r.Title, &r.ModelJSON, &r.Cost,
		&r.TokensInput, &r.TokensOutput, &r.TokensReasoning, &r.TokensCacheRead, &r.TokensCacheWrite,
		&r.TimeCreated, &r.TimeUpdated,
	)
	if err != nil {
		return model.ParseResult{Error: err}
	}

	cwd := ""
	if r.Directory.Valid {
		cwd = r.Directory.String
	} else if r.Path.Valid {
		cwd = r.Path.String
	}

	modelID, providerID := "", ""
	if r.ModelJSON.Valid && r.ModelJSON.String != "" {
		var m struct {
			ID         string `json:"id"`
			ProviderID string `json:"providerID"`
		}
		if json.Unmarshal([]byte(r.ModelJSON.String), &m) == nil {
			modelID = m.ID
			providerID = m.ProviderID
		}
	}

	var started, updated *time.Time
	if r.TimeCreated.Valid && r.TimeCreated.Int64 > 0 {
		t := time.UnixMilli(r.TimeCreated.Int64).UTC()
		started = &t
	}
	if r.TimeUpdated.Valid && r.TimeUpdated.Int64 > 0 {
		t := time.UnixMilli(r.TimeUpdated.Int64).UTC()
		updated = &t
	}

	var cost *float64
	if r.Cost.Valid {
		v := r.Cost.Float64
		cost = &v
	}

	models := []string{}
	if modelID != "" {
		models = []string{modelID}
	}
	detail, _ := json.Marshal(map[string]any{
		"aggregation": "session_row_tokens",
		"title":       nullStr(r.Title),
		"db_path":     a.DBPath,
	})

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          r.ID,
		ProviderSessionID: r.ID,
		CWD:               cwd,
		StartedAt:         started,
		LastEventAt:       updated,
		Model:             modelID,
		Models:            models,
		ModelProvider:     providerID,
		Tokens: model.Tokens{
			Input:      nullInt(r.TokensInput),
			Output:     nullInt(r.TokensOutput),
			Reasoning:  nullInt(r.TokensReasoning),
			CacheRead:  nullInt(r.TokensCacheRead),
			CacheWrite: nullInt(r.TokensCacheWrite),
		},
		ProviderCost:      cost,
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func nullStr(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

var _ adapter.VendorAdapter = (*Adapter)(nil)
