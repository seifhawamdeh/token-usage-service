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
	Version    = "4"
)

type Adapter struct {
	ProjectsRoot string
	JobsRoot     string
}

func New(projectsRoot, jobsRoot string) *Adapter {
	home, _ := os.UserHomeDir()
	if projectsRoot == "" {
		projectsRoot = filepath.Join(home, ".claude", "projects")
	}
	if jobsRoot == "" {
		jobsRoot = filepath.Join(home, ".claude", "jobs")
	}
	return &Adapter{ProjectsRoot: projectsRoot, JobsRoot: jobsRoot}
}

func (a *Adapter) Name() string    { return VendorName }
func (a *Adapter) Version() string { return Version }

func (a *Adapter) Discover(ctx context.Context) ([]model.SourceDescriptor, error) {
	out, err := a.discoverProjects(ctx)
	if err != nil {
		return nil, err
	}

	// Collect the set of project .jsonl paths so we can skip jobs that
	// are already covered by a transcript file.
	projectPaths := make(map[string]struct{}, len(out))
	for _, sd := range out {
		projectPaths[sd.SourcePath] = struct{}{}
	}

	orphans, err := a.discoverOrphanJobs(ctx, projectPaths)
	if err != nil {
		return nil, err
	}
	out = append(out, orphans...)

	return out, nil
}

// discoverProjects walks ~/.claude/projects/**/*.jsonl (the existing logic).
func (a *Adapter) discoverProjects(ctx context.Context) ([]model.SourceDescriptor, error) {
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

// discoverOrphanJobs scans ~/.claude/jobs/*/state.json for background jobs
// whose transcript .jsonl either doesn't exist or was never written. These
// are jobs that would otherwise be invisible to the pipeline.
func (a *Adapter) discoverOrphanJobs(ctx context.Context, projectPaths map[string]struct{}) ([]model.SourceDescriptor, error) {
	root := a.JobsRoot
	fi, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read claude jobs dir: %w", err)
	}

	var out []model.SourceDescriptor
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if !e.IsDir() {
			continue
		}
		statePath := filepath.Join(root, e.Name(), "state.json")
		info, err := os.Stat(statePath)
		if err != nil {
			continue // no state.json in this directory
		}

		// Quick-read linkScanPath to decide if this job is already
		// covered by a project transcript.
		data, err := os.ReadFile(statePath)
		if err != nil {
			continue
		}
		var probe struct {
			LinkScanPath string `json:"linkScanPath"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			continue
		}

		// If the linked .jsonl exists among discovered project paths,
		// the main project walk already covers this job — skip it.
		if probe.LinkScanPath != "" {
			absLink, _ := filepath.Abs(probe.LinkScanPath)
			if _, covered := projectPaths[absLink]; covered {
				continue
			}
			// Also check if the file exists on disk even if not in
			// projectPaths (defensive — could happen if projects root
			// was overridden).
			if _, err := os.Stat(probe.LinkScanPath); err == nil {
				continue
			}
		}

		abs, err := filepath.Abs(statePath)
		if err != nil {
			abs = statePath
		}
		out = append(out, model.SourceDescriptor{
			Vendor:     VendorName,
			SourcePath: abs,
			StableID:   abs,
			MtimeNs:    info.ModTime().UnixNano(),
			SizeBytes:  info.Size(),
		})
	}
	return out, nil
}

// ---------- JSONL transcript parsing (unchanged) ----------

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
	// Dispatch: state.json files get the job parser; .jsonl files get the
	// transcript parser.
	if filepath.Base(src.SourcePath) == "state.json" {
		return a.parseJobState(src)
	}
	return a.parseTranscript(src)
}

func (a *Adapter) parseTranscript(src model.SourceDescriptor) model.ParseResult {
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

// ---------- Orphan job state.json parsing ----------

// jobState mirrors the fields we care about in ~/.claude/jobs/*/state.json.
type jobState struct {
	Tokens       *int64  `json:"tokens"`         // single integer — no breakdown
	SessionID    string  `json:"sessionId"`
	CWD          string  `json:"cwd"`
	Name         string  `json:"name"`
	Intent       string  `json:"intent"`
	State        string  `json:"state"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
	LinkScanPath string  `json:"linkScanPath"`
	RespawnFlags []string `json:"respawnFlags"`
}

func (a *Adapter) parseJobState(src model.SourceDescriptor) model.ParseResult {
	data, err := os.ReadFile(src.SourcePath)
	if err != nil {
		return model.ParseResult{Error: err}
	}
	var js jobState
	if err := json.Unmarshal(data, &js); err != nil {
		return model.ParseResult{
			Error:   fmt.Errorf("malformed state.json: %w", err),
			Warning: "malformed_record",
		}
	}

	// If there are no tokens recorded at all, skip — nothing to count.
	if js.Tokens == nil || *js.Tokens == 0 {
		return model.ParseResult{Skip: true, Warning: "no_tokens_in_job_state"}
	}

	startedAt := parseTime(js.CreatedAt)
	lastAt := parseTime(js.UpdatedAt)

	// Try to extract model from respawnFlags (e.g. ["--model", "sonnet"]).
	var jobModel string
	for i, flag := range js.RespawnFlags {
		if flag == "--model" && i+1 < len(js.RespawnFlags) {
			jobModel = js.RespawnFlags[i+1]
			break
		}
	}

	var models []string
	if jobModel != "" {
		models = []string{jobModel}
	}

	total := *js.Tokens
	detail, _ := json.Marshal(map[string]any{
		"source":            "job_state",
		"job_state":         js.State,
		"job_name":          js.Name,
		"aggregation":       "total_from_state_json",
		"breakdown":         "unavailable",
		"linked_transcript": js.LinkScanPath,
	})

	snap := &model.BurnSnapshot{
		Vendor:            VendorName,
		SourcePath:        src.SourcePath,
		StableID:          src.StableID,
		ProviderSessionID: js.SessionID,
		CWD:               js.CWD,
		StartedAt:         startedAt,
		LastEventAt:       lastAt,
		Model:             jobModel,
		Models:            models,
		ModelProvider:      "anthropic",
		Tokens: model.Tokens{
			Total: &total,
			// Input, Output, CacheRead, CacheWrite left nil — state.json
			// provides only an aggregate integer with no breakdown.
		},
		UsageDetail:       detail,
		AdapterVersion:    Version,
		SnapshotSchemaVer: model.SnapshotSchemaVersion,
		ParseStatus:       "partial",
	}
	return model.ParseResult{Snapshot: snap}
}

// ---------- helpers ----------

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
