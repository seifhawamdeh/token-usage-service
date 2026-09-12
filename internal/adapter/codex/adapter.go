package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "codex"
	Version    = "1"
)

type Adapter struct {
	SessionsRoot string
}

func New(sessionsRoot string) *Adapter {
	if sessionsRoot == "" {
		home, _ := os.UserHomeDir()
		sessionsRoot = filepath.Join(home, ".codex", "sessions")
	}
	return &Adapter{SessionsRoot: sessionsRoot}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	root := a.SessionsRoot
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
		base := d.Name()
		if !strings.HasPrefix(base, "rollout-") || filepath.Ext(base) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: abs,
			StableID:   abs, // refined in Parse from session_id
			MtimeNs:    info.ModTime().UnixNano(),
			SizeBytes:  info.Size(),
		})
		return nil
	})
	return out, err
}

type envelope struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	SessionID      string `json:"session_id"`
	ID             string `json:"id"`
	CWD            string `json:"cwd"`
	ModelProvider  string `json:"model_provider"`
	Timestamp      string `json:"timestamp"`
}

type tokenUsagePayload struct {
	ThreadID          string      `json:"thread_id"`
	SessionID         string      `json:"session_id"`
	ThreadTokenUsage  *tokenBucket `json:"thread_token_usage"`
}

type tokenBucket struct {
	InputTokens            *int64 `json:"input_tokens"`
	CachedInputTokens      *int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens  *int64 `json:"cache_write_input_tokens"`
	OutputTokens           *int64 `json:"output_tokens"`
	ReasoningOutputTokens  *int64 `json:"reasoning_output_tokens"`
	TotalTokens            *int64 `json:"total_tokens"`
}

type turnContext struct {
	Model string `json:"model"`
}

func (a *Adapter) Parse(_ context.Context, src model.SourceDescriptor) model.ParseResult {
	f, err := os.Open(src.SourcePath)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer f.Close()

	var (
		sessionID, cwd, modelProvider, lastModel string
		models                                   = map[string]struct{}{}
		startedAt, lastAt                        *time.Time
		lastThread                               *tokenBucket
		usageEvents                              int
	)

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 64*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return model.ParseResult{Error: fmt.Errorf("malformed jsonl: %w", err), Warning: "malformed_record"}
		}
		if ts := parseTime(env.Timestamp); ts != nil {
			if startedAt == nil {
				startedAt = ts
			}
			lastAt = ts
		}
		switch env.Type {
		case "session_meta":
			var p sessionMeta
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			if p.SessionID != "" {
				sessionID = p.SessionID
			} else if p.ID != "" {
				sessionID = p.ID
			}
			if p.CWD != "" {
				cwd = p.CWD
			}
			if p.ModelProvider != "" {
				modelProvider = p.ModelProvider
			}
			if ts := parseTime(p.Timestamp); ts != nil && startedAt == nil {
				startedAt = ts
			}
		case "turn_context":
			var p turnContext
			if err := json.Unmarshal(env.Payload, &p); err == nil && p.Model != "" {
				lastModel = p.Model
				models[p.Model] = struct{}{}
			}
		case "token_usage_record":
			var p tokenUsagePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			if p.SessionID != "" && sessionID == "" {
				sessionID = p.SessionID
			}
			if p.ThreadID != "" && sessionID == "" {
				sessionID = p.ThreadID
			}
			if p.ThreadTokenUsage != nil {
				lastThread = p.ThreadTokenUsage
				usageEvents++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return model.ParseResult{Error: err}
	}

	stable := src.SourcePath
	if sessionID != "" {
		stable = sessionID
	}
	modelList := make([]string, 0, len(models))
	for m := range models {
		modelList = append(modelList, m)
	}
	detail, _ := json.Marshal(map[string]any{
		"aggregation":            "last_thread_token_usage",
		"token_usage_records":    usageEvents,
		"rollout_path":           src.SourcePath,
	})

	tok := model.Tokens{}
	if lastThread != nil {
		tok.Input = lastThread.InputTokens
		tok.CacheRead = lastThread.CachedInputTokens
		tok.CacheWrite = lastThread.CacheWriteInputTokens
		tok.Output = lastThread.OutputTokens
		tok.Reasoning = lastThread.ReasoningOutputTokens
		tok.Total = lastThread.TotalTokens
	}

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          stable,
		ProviderSessionID: sessionID,
		CWD:               cwd,
		StartedAt:         startedAt,
		LastEventAt:       lastAt,
		Model:             lastModel,
		Models:            modelList,
		ModelProvider:     modelProvider,
		Tokens:            tok,
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
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

var _ adapter.VendorAdapter = (*Adapter)(nil)
