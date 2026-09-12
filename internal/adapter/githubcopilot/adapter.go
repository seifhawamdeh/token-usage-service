package githubcopilot

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
	VendorName = "github-copilot"
	Version    = "1"
)

// Default AI Credits cutover (UTC). Pre-cutover → legacy-premium-requests.
var DefaultBillingCutover = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type Adapter struct {
	DBPath                   string
	BillingCutover           time.Time
	ForceLegacyPremiumRequests bool
}

func New(dbPath string, cutover time.Time, forceLegacy bool) *Adapter {
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".copilot", "session-store.db")
	}
	if cutover.IsZero() {
		cutover = DefaultBillingCutover
	}
	return &Adapter{
		DBPath:                     dbPath,
		BillingCutover:             cutover,
		ForceLegacyPremiumRequests: forceLegacy,
	}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) openRO() (*sql.DB, error) {
	if _, err := os.Stat(a.DBPath); err != nil {
		return nil, err
	}
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
	// Prefer sessions that have usage events; also include sessions table for cwd later.
	rows, err := db.QueryContext(ctx, `
		SELECT session_id,
		       MAX(created_at) AS last_at,
		       COUNT(*) AS n,
		       COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0)+COALESCE(SUM(cache_read_tokens),0)+COALESCE(SUM(cache_write_tokens),0)+COALESCE(SUM(reasoning_tokens),0) AS tok
		FROM assistant_usage_events
		GROUP BY session_id`)
	if err != nil {
		// Table may be missing on older installs
		return nil, nil
	}
	defer rows.Close()

	var out []model.SourceDescriptor
	for rows.Next() {
		var sid, lastAt string
		var n, tok int64
		if err := rows.Scan(&sid, &lastAt, &n, &tok); err != nil {
			return nil, err
		}
		mtime := parseCopilotTime(lastAt)
		var mtimeNs int64
		if mtime != nil {
			mtimeNs = mtime.UnixNano()
		}
		sourcePath := abs + "#session:" + sid
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: sourcePath,
			StableID:   sid,
			MtimeNs:    mtimeNs,
			SizeBytes:  tok + n, // changes when events/tokens grow
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

	sid := src.StableID
	var cwd sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT cwd FROM sessions WHERE id = ?`, sid).Scan(&cwd)

	rows, err := db.QueryContext(ctx, `
		SELECT model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       reasoning_tokens, total_nano_aiu, request_multiplier, created_at
		FROM assistant_usage_events
		WHERE session_id = ?
		ORDER BY created_at ASC`, sid)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer rows.Close()

	var (
		sumIn, sumOut, sumCR, sumCW, sumReason int64
		haveIn, haveOut, haveCR, haveCW, haveReason bool
		sumNano                                    int64
		sumMult                                    float64
		haveNano, haveMult                         bool
		models                                     = map[string]struct{}{}
		lastModel                                  string
		started, last                              *time.Time
		n                                          int
	)

	for rows.Next() {
		var modelName sql.NullString
		var in, out, cr, cw, reason, nano sql.NullInt64
		var mult sql.NullFloat64
		var created string
		if err := rows.Scan(&modelName, &in, &out, &cr, &cw, &reason, &nano, &mult, &created); err != nil {
			return model.ParseResult{Error: err}
		}
		n++
		if modelName.Valid && modelName.String != "" {
			lastModel = modelName.String
			models[modelName.String] = struct{}{}
		}
		addI := func(have *bool, sum *int64, v sql.NullInt64) {
			if v.Valid {
				*sum += v.Int64
				*have = true
			}
		}
		addI(&haveIn, &sumIn, in)
		addI(&haveOut, &sumOut, out)
		addI(&haveCR, &sumCR, cr)
		addI(&haveCW, &sumCW, cw)
		addI(&haveReason, &sumReason, reason)
		if nano.Valid {
			sumNano += nano.Int64
			haveNano = true
		}
		if mult.Valid {
			sumMult += mult.Float64
			haveMult = true
		}
		if t := parseCopilotTime(created); t != nil {
			if started == nil {
				started = t
			}
			last = t
		}
	}
	if err := rows.Err(); err != nil {
		return model.ParseResult{Error: err}
	}

	regime := a.regimeFor(last)
	modelList := make([]string, 0, len(models))
	for m := range models {
		modelList = append(modelList, m)
	}
	detail := map[string]any{
		"aggregation":              "sum_assistant_usage_events",
		"event_count":              n,
		"request_multiplier_sum":   nil,
		"total_nano_aiu":           nil,
		"billing_cutover":          a.BillingCutover.Format(time.RFC3339),
		"force_legacy_premium":     a.ForceLegacyPremiumRequests,
	}
	if haveMult {
		detail["request_multiplier_sum"] = sumMult
	}
	if haveNano {
		detail["total_nano_aiu"] = sumNano
	}
	raw, _ := json.Marshal(detail)

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          sid,
		ProviderSessionID: sid,
		CWD:               nullStr(cwd),
		StartedAt:         started,
		LastEventAt:       last,
		Model:             lastModel,
		Models:            modelList,
		Tokens: model.Tokens{
			Input:      ptrIf(haveIn, sumIn),
			Output:     ptrIf(haveOut, sumOut),
			CacheRead:  ptrIf(haveCR, sumCR),
			CacheWrite: ptrIf(haveCW, sumCW),
			Reasoning:  ptrIf(haveReason, sumReason),
		},
		BillingRegime:     regime,
		UsageDetail:       raw,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func (a *Adapter) regimeFor(sessionTime *time.Time) string {
	if a.ForceLegacyPremiumRequests {
		return "legacy-premium-requests"
	}
	if sessionTime == nil {
		return "ai-credits"
	}
	if sessionTime.Before(a.BillingCutover) {
		return "legacy-premium-requests"
	}
	return "ai-credits"
}

func parseCopilotTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func ptrIf(ok bool, v int64) *int64 {
	if !ok {
		return nil
	}
	return &v
}

func nullStr(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

var _ adapter.VendorAdapter = (*Adapter)(nil)
