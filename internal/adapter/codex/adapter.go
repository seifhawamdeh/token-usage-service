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
	Version    = "5"
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
	SessionID     string `json:"session_id"`
	ID            string `json:"id"`
	CWD           string `json:"cwd"`
	ModelProvider string `json:"model_provider"`
	Timestamp     string `json:"timestamp"`
}

type eventMsg struct {
	Type string          `json:"type"`
	Info json.RawMessage `json:"info"`
}

type tokenCountInfo struct {
	TotalTokenUsage *tokenBucket `json:"total_token_usage"`
}

type tokenBucket struct {
	InputTokens           *int64 `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
	TotalTokens           *int64 `json:"total_tokens"`
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
		usageEvents                              int

		// Codex's total_token_usage is cumulative within a segment but resets
		// to near-zero when the context is compacted. To recover the true
		// session total we sum the peak of each segment rather than taking a
		// single file-wide max (which would silently drop every segment but
		// the largest).
		segPeak    *tokenBucket
		prevTotal  int64
		havePrev   bool
		sum        model.Tokens
		haveAnySeg bool
	)

	commitSegment := func() {
		if segPeak == nil {
			return
		}
		addPtr := func(dst **int64, v *int64) {
			if v == nil {
				return
			}
			if *dst == nil {
				zero := int64(0)
				*dst = &zero
			}
			**dst += *v
		}
		// Codex's input_tokens INCLUDES cached_input_tokens (verified:
		// total_tokens == input_tokens + output_tokens always holds, so cache
		// is a sub-count of input, not additive — unlike Anthropic, where
		// input_tokens is fresh-only and cache_read is additive on top). The
		// rating view bills tokens_input and tokens_cache_read as separate
		// additive pools, so storing raw input_tokens here would double-bill
		// the cached slice. Store fresh-only input instead.
		var freshInput *int64
		if segPeak.InputTokens != nil {
			fresh := *segPeak.InputTokens
			if segPeak.CachedInputTokens != nil && *segPeak.CachedInputTokens > 0 {
				fresh -= *segPeak.CachedInputTokens
				if fresh < 0 {
					fresh = 0
				}
			}
			freshInput = &fresh
		}
		addPtr(&sum.Input, freshInput)
		addPtr(&sum.Output, segPeak.OutputTokens)
		addPtr(&sum.CacheRead, segPeak.CachedInputTokens)
		addPtr(&sum.CacheWrite, segPeak.CacheWriteInputTokens)
		addPtr(&sum.Reasoning, segPeak.ReasoningOutputTokens)
		addPtr(&sum.Total, segPeak.TotalTokens)
		haveAnySeg = true
		segPeak = nil
	}

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
		case "event_msg":
			var p eventMsg
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			if p.Type != "token_count" {
				continue
			}
			var info tokenCountInfo
			if err := json.Unmarshal(p.Info, &info); err != nil {
				continue
			}
			if info.TotalTokenUsage != nil && info.TotalTokenUsage.TotalTokens != nil {
				usageEvents++
				v := *info.TotalTokenUsage.TotalTokens
				if havePrev && v < prevTotal {
					commitSegment()
				}
				if segPeak == nil || segPeak.TotalTokens == nil || v > *segPeak.TotalTokens {
					segPeak = info.TotalTokenUsage
				}
				prevTotal = v
				havePrev = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return model.ParseResult{Error: err}
	}
	commitSegment()

	stable := src.SourcePath
	if sessionID != "" {
		stable = sessionID
	}
	modelList := make([]string, 0, len(models))
	for m := range models {
		modelList = append(modelList, m)
	}
	detail, _ := json.Marshal(map[string]any{
		"aggregation":        "sum_of_segment_peak_total_token_usage",
		"token_count_events": usageEvents,
		"rollout_path":       src.SourcePath,
	})

	tok := model.Tokens{}
	if haveAnySeg {
		tok = sum
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
