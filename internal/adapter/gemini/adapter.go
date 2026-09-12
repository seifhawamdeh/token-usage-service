package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "gemini"
	Version    = "1"
)

// Adapter reads Google Gemini CLI sessions under ~/.gemini/tmp/<hash>/chats/.
// CLI-only — does not walk Antigravity paths.
type Adapter struct {
	TmpRoot string
}

func New(tmpRoot string) *Adapter {
	if tmpRoot == "" {
		home, _ := os.UserHomeDir()
		tmpRoot = filepath.Join(home, ".gemini", "tmp")
	}
	return &Adapter{TmpRoot: tmpRoot}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	root := a.TmpRoot
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []model.SourceDescriptor
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "session-") {
			return nil
		}
		ext := filepath.Ext(name)
		if ext != ".jsonl" && ext != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		abs, _ := filepath.Abs(path)
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: abs,
			StableID:   strings.TrimSuffix(name, ext),
			MtimeNs:    info.ModTime().UnixNano(),
			SizeBytes:  info.Size(),
		})
		return nil
	})
	return out, err
}

type tokensObj struct {
	Input     *int64 `json:"input"`
	Output    *int64 `json:"output"`
	Cached    *int64 `json:"cached"`
	Thoughts  *int64 `json:"thoughts"`
	Tool      *int64 `json:"tool"`
	Total     *int64 `json:"total"`
}

type msgLine struct {
	Type      string     `json:"type"`
	Model     string     `json:"model"`
	Timestamp string     `json:"timestamp"`
	Tokens    *tokensObj `json:"tokens"`
	SessionID string     `json:"sessionId"`
	StartTime string     `json:"startTime"`
}

func (a *Adapter) Parse(_ context.Context, src model.SourceDescriptor) model.ParseResult {
	f, err := os.Open(src.SourcePath)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer f.Close()

	var (
		sumIn, sumOut, sumCache, sumThink, sumTotal, sumTool int64
		haveIn, haveOut, haveCache, haveThink, haveTotal, haveTool bool
		lastModel string
		models    = map[string]struct{}{}
		started, last *time.Time
		sessionID string
		msgCount  int
	)

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 32*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var o msgLine
		if err := json.Unmarshal(line, &o); err != nil {
			// older .json may be a single document — try once at EOF handled below
			continue
		}
		if o.Type == "$set" {
			continue
		}
		if o.SessionID != "" {
			sessionID = o.SessionID
		}
		if ts := parseTime(firstNonEmpty(o.Timestamp, o.StartTime)); ts != nil {
			if started == nil {
				started = ts
			}
			last = ts
		}
		// Model messages with tokens
		if o.Tokens == nil {
			continue
		}
		if o.Type != "" && o.Type != "gemini" && o.Type != "model" {
			// still accept if tokens present
		}
		msgCount++
		if o.Model != "" {
			lastModel = o.Model
			models[o.Model] = struct{}{}
		}
		add := func(have *bool, sum *int64, v *int64) {
			if v == nil {
				return
			}
			*sum += *v
			*have = true
		}
		add(&haveIn, &sumIn, o.Tokens.Input)
		add(&haveOut, &sumOut, o.Tokens.Output)
		add(&haveCache, &sumCache, o.Tokens.Cached)
		add(&haveThink, &sumThink, o.Tokens.Thoughts)
		add(&haveTotal, &sumTotal, o.Tokens.Total)
		add(&haveTool, &sumTool, o.Tokens.Tool)
	}
	if err := sc.Err(); err != nil {
		return model.ParseResult{Error: err}
	}

	// Fallback: whole-file JSON with messages array
	if msgCount == 0 {
		raw, err := os.ReadFile(src.SourcePath)
		if err == nil {
			var doc struct {
				SessionID string `json:"sessionId"`
				StartTime string `json:"startTime"`
				Messages  []struct {
					Type      string     `json:"type"`
					Model     string     `json:"model"`
					Timestamp string     `json:"timestamp"`
					Tokens    *tokensObj `json:"tokens"`
				} `json:"messages"`
			}
			if json.Unmarshal(raw, &doc) == nil {
				sessionID = doc.SessionID
				if ts := parseTime(doc.StartTime); ts != nil {
					started = ts
				}
				for _, m := range doc.Messages {
					if m.Tokens == nil {
						continue
					}
					msgCount++
					if m.Model != "" {
						lastModel = m.Model
						models[m.Model] = struct{}{}
					}
					if m.Tokens.Input != nil {
						sumIn += *m.Tokens.Input
						haveIn = true
					}
					if m.Tokens.Output != nil {
						sumOut += *m.Tokens.Output
						haveOut = true
					}
					if m.Tokens.Cached != nil {
						sumCache += *m.Tokens.Cached
						haveCache = true
					}
					if m.Tokens.Thoughts != nil {
						sumThink += *m.Tokens.Thoughts
						haveThink = true
					}
					if m.Tokens.Total != nil {
						sumTotal += *m.Tokens.Total
						haveTotal = true
					}
					if m.Tokens.Tool != nil {
						sumTool += *m.Tokens.Tool
						haveTool = true
					}
				}
			}
		}
	}

	stable := src.StableID
	if sessionID != "" {
		stable = sessionID
	}
	modelList := make([]string, 0, len(models))
	for m := range models {
		modelList = append(modelList, m)
	}
	detail, _ := json.Marshal(map[string]any{
		"aggregation":   "sum_message_tokens",
		"message_count": msgCount,
		"tool_tokens":   ptrIf(haveTool, sumTool),
	})

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          stable,
		ProviderSessionID: sessionID,
		StartedAt:         started,
		LastEventAt:       last,
		Model:             lastModel,
		Models:            modelList,
		ModelProvider:     "google",
		Tokens: model.Tokens{
			Input:     ptrIf(haveIn, sumIn),
			Output:    ptrIf(haveOut, sumOut),
			CacheRead: ptrIf(haveCache, sumCache),
			Reasoning: ptrIf(haveThink, sumThink),
			Total:     ptrIf(haveTotal, sumTotal),
		},
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
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

var _ adapter.VendorAdapter = (*Adapter)(nil)
