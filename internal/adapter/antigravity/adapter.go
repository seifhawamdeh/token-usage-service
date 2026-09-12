package antigravity

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
	VendorName = "antigravity"
	Version    = "1"
)

// Adapter reads Antigravity CLI conversation SQLite DBs and decodes
// gen_metadata protobuf usage (not JSONL transcripts; those lack tokens).
type Adapter struct {
	Root string // ~/.gemini/antigravity-cli
}

func New(root string) *Adapter {
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".gemini", "antigravity-cli")
	}
	return &Adapter{Root: root}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	dir := filepath.Join(a.Root, "conversations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []model.SourceDescriptor
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".db") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		abs, _ := filepath.Abs(path)
		mtimeNs, size := newestDBMeta(abs)
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: abs,
			StableID:   stem,
			MtimeNs:    mtimeNs,
			SizeBytes:  size,
		})
	}
	return out, nil
}

func newestDBMeta(dbPath string) (mtimeNs, size int64) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		p := dbPath + suffix
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		ns := fi.ModTime().UnixNano()
		if ns > mtimeNs {
			mtimeNs = ns
		}
		size += fi.Size()
	}
	return mtimeNs, size
}

type genAgg struct {
	input, output, cacheRead, reasoning int64
	haveIn, haveOut, haveCR, haveReason bool
	models                              map[string]struct{}
	lastModel                           string
	modelProvider                       string
	started, last                       *time.Time
	gens, skipped, drift                int
	cwd                                 string
}

func (a *Adapter) Parse(ctx context.Context, src model.SourceDescriptor) model.ParseResult {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", src.SourcePath))
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer db.Close()

	agg := &genAgg{models: map[string]struct{}{}}
	fallbackMS, cwd := trajectoryMeta(ctx, db)
	agg.cwd = cwd

	rows, err := db.QueryContext(ctx, `SELECT data FROM gen_metadata ORDER BY idx`)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer rows.Close()

	seen := map[string]struct{}{}
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return model.ParseResult{Error: err}
		}
		switch parseGen(blob, fallbackMS, seen, agg) {
		case parseOK:
		case parseSkip:
			agg.skipped++
		case parseDrift:
			agg.drift++
		}
	}
	if err := rows.Err(); err != nil {
		return model.ParseResult{Error: err}
	}

	modelList := make([]string, 0, len(agg.models))
	for m := range agg.models {
		modelList = append(modelList, m)
	}
	detail, _ := json.Marshal(map[string]any{
		"aggregation":          "sum_gen_metadata_protobuf",
		"generations_trusted":  agg.gens,
		"generations_skipped":  agg.skipped,
		"generations_drifted":  agg.drift,
		"self_check":           "usage.#3 == #9 + #10",
		"note":                 "transcript JSONL is not used (no native tokens)",
	})

	provider := agg.modelProvider
	if provider == "" && looksLikeGemini(agg.lastModel) {
		provider = "google"
	}

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          src.StableID,
		ProviderSessionID: src.StableID,
		CWD:               agg.cwd,
		StartedAt:         agg.started,
		LastEventAt:       agg.last,
		Model:             agg.lastModel,
		Models:            modelList,
		ModelProvider:     provider,
		Tokens: model.Tokens{
			Input:     ptrIf(agg.haveIn, agg.input),
			Output:    ptrIf(agg.haveOut, agg.output),
			CacheRead: ptrIf(agg.haveCR, agg.cacheRead),
			Reasoning: ptrIf(agg.haveReason, agg.reasoning),
		},
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

type parseResult int

const (
	parseOK parseResult = iota
	parseSkip
	parseDrift
)

func parseGen(blob []byte, fallbackMS int64, seen map[string]struct{}, agg *genAgg) parseResult {
	chat := messageField(blob, 1)
	if chat == nil {
		return parseSkip
	}
	usage := messageField(chat, 4)
	if usage == nil {
		return parseSkip
	}

	system, _ := varintField(usage, 1)
	newIn, _ := varintField(usage, 2)
	cacheRead, _ := varintField(usage, 5)
	outText, _ := varintField(usage, 9)
	thinking, _ := varintField(usage, 10)

	total, hasTotal := varintField(usage, 3)
	if !hasTotal {
		return parseSkip
	}
	if clamp(total) != clamp(outText)+clamp(thinking) {
		return parseDrift
	}

	input := clamp(system) + clamp(newIn)
	out := clamp(outText)
	think := clamp(thinking)
	cr := clamp(cacheRead)
	if input == 0 && out == 0 && cr == 0 && think == 0 {
		return parseSkip
	}

	if id := stringField(usage, 11); id != "" {
		if _, ok := seen[id]; ok {
			return parseSkip
		}
		seen[id] = struct{}{}
	}

	modelName := stringField(chat, 21)
	if modelName == "" {
		modelName = stringField(chat, 19)
	}
	if modelName != "" {
		agg.lastModel = modelName
		agg.models[modelName] = struct{}{}
		if looksLikeGemini(modelName) {
			agg.modelProvider = "google"
		} else if strings.Contains(strings.ToLower(modelName), "claude") {
			agg.modelProvider = "anthropic"
		}
	}

	tsMS := fallbackMS
	if node := messageField(chat, 9); node != nil {
		if inner := messageField(node, 4); inner != nil {
			if ms := protoTimestampMS(inner); ms > 0 {
				tsMS = ms
			}
		}
	}
	if tsMS > 0 {
		t := time.UnixMilli(tsMS).UTC()
		if agg.started == nil {
			agg.started = &t
		}
		agg.last = &t
	}

	agg.input += int64(input)
	agg.haveIn = true
	agg.output += int64(out)
	agg.haveOut = true
	if cr > 0 || agg.haveCR {
		agg.cacheRead += int64(cr)
		agg.haveCR = true
	}
	if think > 0 || agg.haveReason {
		agg.reasoning += int64(think)
		agg.haveReason = true
	}
	agg.gens++
	return parseOK
}

func trajectoryMeta(ctx context.Context, db *sql.DB) (fallbackMS int64, cwd string) {
	var blob []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM trajectory_metadata_blob LIMIT 1`).Scan(&blob)
	if err != nil || len(blob) == 0 {
		return 0, ""
	}
	if ts := messageField(blob, 2); ts != nil {
		fallbackMS = protoTimestampMS(ts)
	}
	if folder := messageField(blob, 1); folder != nil {
		if uri := stringField(folder, 1); uri != "" {
			cwd = fileURIToPath(uri)
		}
	}
	return fallbackMS, cwd
}

func fileURIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' && ((path[1] >= 'A' && path[1] <= 'Z') || (path[1] >= 'a' && path[1] <= 'z')) {
		return path[1:]
	}
	return path
}

func clamp(v uint64) uint64 {
	const max = uint64(^uint64(0) >> 1) // i64 max
	if v > max {
		return max
	}
	return v
}

func looksLikeGemini(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "gemini") || strings.Contains(m, "gemma")
}

func ptrIf(ok bool, v int64) *int64 {
	if !ok {
		return nil
	}
	return &v
}

var _ adapter.VendorAdapter = (*Adapter)(nil)
