package cursorusage

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "cursor-usage"
	Version    = "1"
)

// Adapter ingests Cursor dashboard/admin usage-events CSV exports.
// These are billed/included token facts and are intentionally separate from
// local composer session rows (vendor "cursor").
type Adapter struct {
	ReportsDir string
}

func New(reportsDir string) *Adapter {
	if reportsDir == "" {
		reportsDir = "cursor-usage-reports"
	}
	return &Adapter{ReportsDir: reportsDir}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

type eventRow struct {
	file       string
	date       time.Time
	dateRaw    string
	cloudAgent string
	automation string
	kind       string
	model      string
	maxMode    string
	inCacheW   int64
	inNoCacheW int64
	cacheRead  int64
	output     int64
	total      int64
	costRaw    string
	cost       *float64
	lineNo     int
}

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	files, err := a.listCSVs()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []model.SourceDescriptor
	for _, f := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		events, err := a.parseFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		abs, _ := filepath.Abs(f)
		for _, e := range events {
			stable := e.dateRaw
			out = append(out, model.SourceDescriptor{
				Vendor:     VendorName,
				SourcePath: abs + "#event:" + stable,
				StableID:   stable,
				MtimeNs:    e.date.UnixNano(),
				SizeBytes:  e.total + e.output + e.cacheRead + e.inNoCacheW + e.inCacheW,
			})
		}
	}
	return out, nil
}

func (a *Adapter) Parse(ctx context.Context, src model.SourceDescriptor) model.ParseResult {
	file, dateRaw, ok := splitEventPath(src.SourcePath)
	if !ok {
		return model.ParseResult{Error: fmt.Errorf("bad source_path %q", src.SourcePath)}
	}
	events, err := a.parseFile(file)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	var e *eventRow
	for i := range events {
		if events[i].dateRaw == dateRaw {
			e = &events[i]
			break
		}
	}
	if e == nil {
		return model.ParseResult{Skip: true, Warning: "event_not_found"}
	}

	// Input (w/o Cache Write) is the billed new-input signal; cache write is separate.
	input := e.inNoCacheW
	detail, _ := json.Marshal(map[string]any{
		"data_kind":              "usage_event",
		"report_file":            filepath.Base(file),
		"cloud_agent_id":         e.cloudAgent,
		"automation_id":          e.automation,
		"kind":                   e.kind,
		"max_mode":               e.maxMode,
		"input_with_cache_write": e.inCacheW,
		"input_without_cache_write": e.inNoCacheW,
		"cost_raw":               e.costRaw,
		"csv_line":               e.lineNo,
		"join_note":              "no composer/session id in Cursor usage-events CSV; not linked to cursor sessions",
	})

	ts := e.date.UTC()
	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          e.dateRaw,
		StartedAt:         &ts,
		LastEventAt:       &ts,
		Model:             e.model,
		Models:            []string{e.model},
		ModelProvider:     "cursor",
		Tokens: model.Tokens{
			Input:      int64Ptr(input),
			Output:     int64Ptr(e.output),
			CacheRead:  int64Ptr(e.cacheRead),
			CacheWrite: int64Ptr(e.inCacheW),
			Total:      int64Ptr(e.total),
		},
		ProviderCost:      e.cost,
		BillingRegime:     e.kind,
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func splitEventPath(sourcePath string) (file, dateRaw string, ok bool) {
	const mark = "#event:"
	i := strings.LastIndex(sourcePath, mark)
	if i < 0 {
		return "", "", false
	}
	return sourcePath[:i], sourcePath[i+len(mark):], true
}

func (a *Adapter) listCSVs() ([]string, error) {
	st, err := os.Stat(a.ReportsDir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("CURSOR_USAGE_REPORTS_DIR is not a directory: %s", a.ReportsDir)
	}
	entries, err := os.ReadDir(a.ReportsDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".csv") {
			continue
		}
		out = append(out, filepath.Join(a.ReportsDir, name))
	}
	return out, nil
}

func (a *Adapter) parseFile(path string) ([]eventRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, nil
	}
	header := map[string]int{}
	for i, h := range records[0] {
		header[strings.TrimSpace(h)] = i
	}
	required := []string{"Date", "Model", "Total Tokens"}
	for _, k := range required {
		if _, ok := header[k]; !ok {
			return nil, fmt.Errorf("missing column %q", k)
		}
	}

	abs, _ := filepath.Abs(path)
	var out []eventRow
	for i, rec := range records[1:] {
		get := func(col string) string {
			idx, ok := header[col]
			if !ok || idx >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[idx])
		}
		dateRaw := get("Date")
		if dateRaw == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, dateRaw)
		if err != nil {
			ts, err = time.Parse(time.RFC3339, dateRaw)
			if err != nil {
				return nil, fmt.Errorf("line %d date %q: %w", i+2, dateRaw, err)
			}
		}
		costRaw := get("Cost")
		var cost *float64
		if costRaw != "" && !strings.EqualFold(costRaw, "Included") && !strings.EqualFold(costRaw, "Free") {
			if v, err := strconv.ParseFloat(costRaw, 64); err == nil {
				cost = &v
			}
		}
		out = append(out, eventRow{
			file:       abs,
			date:       ts.UTC(),
			dateRaw:    dateRaw,
			cloudAgent: get("Cloud Agent ID"),
			automation: get("Automation ID"),
			kind:       get("Kind"),
			model:      get("Model"),
			maxMode:    get("Max Mode"),
			inCacheW:   parseInt(get("Input (w/ Cache Write)")),
			inNoCacheW: parseInt(get("Input (w/o Cache Write)")),
			cacheRead:  parseInt(get("Cache Read")),
			output:     parseInt(get("Output Tokens")),
			total:      parseInt(get("Total Tokens")),
			costRaw:    costRaw,
			cost:       cost,
			lineNo:     i + 2,
		})
	}
	return out, nil
}

func parseInt(s string) int64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func int64Ptr(v int64) *int64 { return &v }

var _ adapter.VendorAdapter = (*Adapter)(nil)
