package cursor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "cursor"
	Version    = "2"
)

// Adapter reads Cursor global state.vscdb composer sessions.
//
// This adapter stores session facts only (identity, timestamps, workspace,
// bubble counts). Token burn belongs in the cursor-usage adapter (CSV/billing
// exports). Local bubble tokenCount is almost always zero and must not be
// treated as billed usage.
type Adapter struct {
	StateDB         string
	TranscriptsRoot string // optional; default ~/.cursor/projects
}

func New(stateDB string) *Adapter {
	if stateDB == "" {
		home, _ := os.UserHomeDir()
		stateDB = filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	}
	home, _ := os.UserHomeDir()
	return &Adapter{
		StateDB:         stateDB,
		TranscriptsRoot: filepath.Join(home, ".cursor", "projects"),
	}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) openRO() (*sql.DB, error) {
	if _, err := os.Stat(a.StateDB); err != nil {
		return nil, err
	}
	// Do not use immutable=1 — need WAL when Cursor is running.
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", a.StateDB)
	return sql.Open("sqlite", dsn)
}

type composerSession struct {
	id               string
	workspaceID      string
	createdAt        int64
	updatedAt        int64
	bubbles          int
	positiveBubbles  int
	models           map[string]struct{}
	lastModel        string
	transcriptPath   string
	localInput       int64
	localOutput      int64
	localCacheRead   int64
	localCacheWrite  int64
	haveLocalTokens  bool
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

	sessions, err := a.loadSessions(ctx, db)
	if err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(a.StateDB)
	out := make([]model.SourceDescriptor, 0, len(sessions))
	for id, s := range sessions {
		mtime := s.updatedAt
		if mtime == 0 {
			mtime = s.createdAt
		}
		size := int64(s.bubbles)*1000 + int64(s.positiveBubbles)
		if mtime == 0 {
			// Still discover; watermark on bubble count only.
			mtime = 1
		}
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: abs + "#composer:" + id,
			StableID:   id,
			MtimeNs:    mtime * int64(time.Millisecond),
			SizeBytes:  size,
		})
	}
	return out, nil
}

func (a *Adapter) Parse(ctx context.Context, src model.SourceDescriptor) model.ParseResult {
	db, err := a.openRO()
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer db.Close()

	sessions, err := a.loadSessions(ctx, db)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	s, ok := sessions[src.StableID]
	if !ok {
		return model.ParseResult{Skip: true, Warning: "composer_not_found"}
	}

	modelList := make([]string, 0, len(s.models))
	for m := range s.models {
		modelList = append(modelList, m)
	}

	detail, _ := json.Marshal(map[string]any{
		"data_kind":            "session",
		"workspace_id":         s.workspaceID,
		"bubbles_seen":         s.bubbles,
		"bubbles_local_tokens": s.positiveBubbles,
		"transcript_path":      s.transcriptPath,
		"local_tokenCount": map[string]any{
			"note":        "sparse/unreliable; not used as billed burn",
			"input":       nullIfZero(s.haveLocalTokens, s.localInput),
			"output":      nullIfZero(s.haveLocalTokens, s.localOutput),
			"cache_read":  nullIfZero(s.haveLocalTokens, s.localCacheRead),
			"cache_write": nullIfZero(s.haveLocalTokens, s.localCacheWrite),
		},
		"state_db": a.StateDB,
	})

	var started, updated *time.Time
	if s.createdAt > 0 {
		t := time.UnixMilli(s.createdAt).UTC()
		started = &t
	}
	if s.updatedAt > 0 {
		t := time.UnixMilli(s.updatedAt).UTC()
		updated = &t
	}

	cwd := ""
	if s.transcriptPath != "" {
		// .../projects/<project-slug>/agent-transcripts/<uuid>/<uuid>.jsonl
		parts := strings.Split(filepath.ToSlash(s.transcriptPath), "/")
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				cwd = parts[i+1] // project slug hint only
				break
			}
		}
	}

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          src.StableID,
		ProviderSessionID: src.StableID,
		CWD:               cwd,
		StartedAt:         started,
		LastEventAt:       updated,
		Model:             s.lastModel,
		Models:            modelList,
		ModelProvider:     "cursor",
		// Tokens intentionally empty: billed usage is cursor-usage CSV.
		Tokens:            model.Tokens{},
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func nullIfZero(have bool, v int64) any {
	if !have || v == 0 {
		return nil
	}
	return v
}

func (a *Adapter) loadSessions(ctx context.Context, db *sql.DB) (map[string]*composerSession, error) {
	out := map[string]*composerSession{}
	transcripts := a.indexTranscripts()

	if rows, err := db.QueryContext(ctx, `SELECT composerId, workspaceId, createdAt, lastUpdatedAt FROM composerHeaders`); err == nil {
		for rows.Next() {
			var id string
			var ws sql.NullString
			var created, updated sql.NullInt64
			if err := rows.Scan(&id, &ws, &created, &updated); err != nil {
				rows.Close()
				return nil, err
			}
			s := &composerSession{id: id, models: map[string]struct{}{}}
			if ws.Valid {
				s.workspaceID = ws.String
			}
			if created.Valid {
				s.createdAt = created.Int64
			}
			if updated.Valid {
				s.updatedAt = updated.Int64
			}
			if tp, ok := transcripts[id]; ok {
				s.transcriptPath = tp
			}
			out[id] = s
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT key, value FROM cursorDiskKV WHERE key LIKE 'bubbleId:%'`)
	if err != nil {
		// Headers alone are enough for session inventory.
		if len(out) > 0 {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var key string
		var val sql.NullString
		if err := rows.Scan(&key, &val); err != nil {
			return nil, err
		}
		parts := strings.SplitN(key, ":", 3)
		if len(parts) < 3 {
			continue
		}
		composerID := parts[1]
		s := out[composerID]
		if s == nil {
			s = &composerSession{id: composerID, models: map[string]struct{}{}}
			if tp, ok := transcripts[composerID]; ok {
				s.transcriptPath = tp
			}
			out[composerID] = s
		}
		if !val.Valid || val.String == "" {
			continue
		}
		var o map[string]any
		if err := json.Unmarshal([]byte(val.String), &o); err != nil {
			continue
		}
		s.bubbles++
		tc, _ := o["tokenCount"].(map[string]any)
		inn := intFrom(tc, "inputTokens", "input_tokens")
		outn := intFrom(tc, "outputTokens", "output_tokens")
		cr := intFrom(tc, "cacheReadTokens", "cache_read_tokens", "cacheRead")
		cw := intFrom(tc, "cacheWriteTokens", "cache_write_tokens", "cacheWrite")
		if inn > 0 || outn > 0 || cr > 0 || cw > 0 {
			s.positiveBubbles++
			s.haveLocalTokens = true
			s.localInput += inn
			s.localOutput += outn
			s.localCacheRead += cr
			s.localCacheWrite += cw
		}
		if m := modelFromBubble(o); m != "" {
			s.lastModel = m
			s.models[m] = struct{}{}
		}
	}
	return out, rows.Err()
}

func (a *Adapter) indexTranscripts() map[string]string {
	out := map[string]string{}
	root := a.TranscriptsRoot
	if root == "" {
		return out
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") || !strings.Contains(path, string(filepath.Separator)+"agent-transcripts"+string(filepath.Separator)) {
			return nil
		}
		id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if id != "" {
			if abs, err := filepath.Abs(path); err == nil {
				out[id] = abs
			} else {
				out[id] = path
			}
		}
		return nil
	})
	return out
}

func intFrom(m map[string]any, keys ...string) int64 {
	if m == nil {
		return 0
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return int64(t)
		case json.Number:
			n, _ := t.Int64()
			return n
		case int64:
			return t
		case int:
			return int64(t)
		}
	}
	return 0
}

func modelFromBubble(o map[string]any) string {
	for _, k := range []string{"model", "modelName", "modelId"} {
		if s, ok := o[k].(string); ok && s != "" && s != "default" {
			return s
		}
	}
	if mi, ok := o["modelInfo"].(map[string]any); ok {
		for _, k := range []string{"modelId", "modelName", "name"} {
			if s, ok := mi[k].(string); ok && s != "" && s != "default" {
				return s
			}
		}
	}
	return ""
}

var _ adapter.VendorAdapter = (*Adapter)(nil)
