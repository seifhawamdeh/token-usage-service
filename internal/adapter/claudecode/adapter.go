package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/model"
)

const (
	VendorName = "claude-code"
	Version    = "3"
)

type Adapter struct {
	ProjectsRoot string
}

func New(projectsRoot string) *Adapter {
	if projectsRoot == "" {
		home, _ := os.UserHomeDir()
		projectsRoot = filepath.Join(home, ".claude", "projects")
	}
	return &Adapter{ProjectsRoot: projectsRoot}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	root := a.ProjectsRoot
	fi, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("claude projects root is not a directory: %s", root)
	}

	var out []model.SourceDescriptor
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
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
			StableID:   abs,
			MtimeNs:    info.ModTime().UnixNano(),
			SizeBytes:  info.Size(),
		})
		return nil
	})
	return out, err
}

type usageObj struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
	CacheCreation            *struct {
		Ephemeral5mInputTokens *int64 `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens *int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	OutputTokensDetails *struct {
		ThinkingTokens *int64 `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

type lineObj struct {
	Type    string `json:"type"`
	CWD     string `json:"cwd"`
	Session string `json:"sessionId"`
	Message *struct {
		ID    string    `json:"id"`
		Model string    `json:"model"`
		Usage *usageObj `json:"usage"`
	} `json:"message"`
	Timestamp string `json:"timestamp"`
}

func (a *Adapter) Parse(_ context.Context, src model.SourceDescriptor) model.ParseResult {
	f, err := os.Open(src.SourcePath)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	defer f.Close()

	var (
		cwd, session, lastModel string
		models                  = map[string]struct{}{}
		startedAt, lastAt       *time.Time
		rawAssistantLines       int
		// Claude Code writes one line per streamed chunk of an assistant
		// message, all sharing the same message.id and re-reporting the same
		// input/cache usage (only output grows as the stream completes).
		// Dedupe by id and keep the last (most complete) usage per message
		// before summing, or every raw chunk's input/cache tokens get
		// counted once per chunk instead of once per message.
		byMessage = map[string]*usageObj{}
		noIDSeq   int
	)

	sc := bufio.NewScanner(f)
	// Large JSONL lines
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 32*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var o lineObj
		if err := json.Unmarshal(line, &o); err != nil {
			// malformed line that may contain usage — do not replace last good
			return model.ParseResult{
				Error:   fmt.Errorf("malformed jsonl: %w", err),
				Warning: "malformed_record",
			}
		}
		if o.CWD != "" {
			cwd = o.CWD
		}
		if o.Session != "" {
			session = o.Session
		}
		if ts := parseTime(o.Timestamp); ts != nil {
			if startedAt == nil {
				startedAt = ts
			}
			lastAt = ts
		}
		if o.Type != "assistant" || o.Message == nil || o.Message.Usage == nil {
			continue
		}
		rawAssistantLines++
		if o.Message.Model != "" {
			lastModel = o.Message.Model
			models[o.Message.Model] = struct{}{}
		}
		id := o.Message.ID
		if id == "" {
			noIDSeq++
			id = fmt.Sprintf("__noid_%d__", noIDSeq)
		}
		byMessage[id] = o.Message.Usage
	}
	if err := sc.Err(); err != nil {
		return model.ParseResult{Error: err}
	}

	var (
		sumIn, sumOut, sumCW, sumCW5m, sumCW1h, sumCR, sumThink        int64
		haveIn, haveOut, haveCW, haveCW5m, haveCW1h, haveCR, haveThink bool
	)
	add := func(dst *int64, have *bool, v *int64) {
		if v == nil {
			return
		}
		*dst += *v
		*have = true
	}
	for _, u := range byMessage {
		add(&sumIn, &haveIn, u.InputTokens)
		add(&sumOut, &haveOut, u.OutputTokens)
		add(&sumCR, &haveCR, u.CacheReadInputTokens)
		if u.CacheCreation != nil && (u.CacheCreation.Ephemeral5mInputTokens != nil || u.CacheCreation.Ephemeral1hInputTokens != nil) {
			var w5, w1 int64
			if u.CacheCreation.Ephemeral5mInputTokens != nil {
				w5 = *u.CacheCreation.Ephemeral5mInputTokens
				sumCW5m += w5
				haveCW5m = true
			}
			if u.CacheCreation.Ephemeral1hInputTokens != nil {
				w1 = *u.CacheCreation.Ephemeral1hInputTokens
				sumCW1h += w1
				haveCW1h = true
			}
			sumCW += w5 + w1
			haveCW = true
		} else {
			add(&sumCW, &haveCW, u.CacheCreationInputTokens)
		}
		if u.OutputTokensDetails != nil {
			add(&sumThink, &haveThink, u.OutputTokensDetails.ThinkingTokens)
		}
	}
	msgCount := len(byMessage)

	modelList := make([]string, 0, len(models))
	for m := range models {
		modelList = append(modelList, m)
	}
	detail, _ := json.Marshal(map[string]any{
		"assistant_usage_messages": msgCount,
		"raw_assistant_lines":      rawAssistantLines,
		"aggregation":              "sum_assistant_message_usage_deduped_by_message_id",
		"cache_write_windows":      "ephemeral_5m_and_1h_when_present",
	})

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          src.StableID,
		ProviderSessionID: session,
		CWD:               cwd,
		StartedAt:         startedAt,
		LastEventAt:       lastAt,
		Model:             lastModel,
		Models:            modelList,
		ModelProvider:     "anthropic",
		Tokens: model.Tokens{
			Input:        ptrIf(haveIn, sumIn),
			Output:       ptrIf(haveOut, sumOut),
			CacheWrite:   ptrIf(haveCW, sumCW),
			CacheWrite5m: ptrIf(haveCW5m, sumCW5m),
			CacheWrite1h: ptrIf(haveCW1h, sumCW1h),
			CacheRead:    ptrIf(haveCR, sumCR),
			Reasoning:    ptrIf(haveThink, sumThink),
		},
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "ok",
	}
	return model.ParseResult{Snapshot: snap}
}

func ptrIf(ok bool, v int64) *int64 {
	if !ok {
		return nil
	}
	return &v
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
