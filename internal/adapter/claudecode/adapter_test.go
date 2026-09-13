package claudecode_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/seif/token-usage-service/internal/adapter/claudecode"
	"github.com/seif/token-usage-service/internal/model"
)

func TestParseSumsAssistantUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	content := `{"type":"user","message":{}}
{"type":"assistant","message":{"model":"claude-opus","usage":{"input_tokens":2,"output_tokens":10,"cache_creation_input_tokens":100,"cache_read_input_tokens":5,"output_tokens_details":{"thinking_tokens":3}}}}
{"type":"assistant","message":{"model":"claude-opus","usage":{"input_tokens":1,"output_tokens":4,"cache_creation_input_tokens":0,"cache_read_input_tokens":50}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir, "")
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		Vendor:     "claude-code",
		SourcePath: path,
		StableID:   path,
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.Snapshot == nil {
		t.Fatal("nil snapshot")
	}
	tok := res.Snapshot.Tokens
	if tok.Input == nil || *tok.Input != 3 {
		t.Fatalf("input=%v", tok.Input)
	}
	if tok.Output == nil || *tok.Output != 14 {
		t.Fatalf("output=%v", tok.Output)
	}
	if tok.CacheWrite == nil || *tok.CacheWrite != 100 {
		t.Fatalf("cache_write=%v", tok.CacheWrite)
	}
	if tok.CacheRead == nil || *tok.CacheRead != 55 {
		t.Fatalf("cache_read=%v", tok.CacheRead)
	}
	if tok.Reasoning == nil || *tok.Reasoning != 3 {
		t.Fatalf("reasoning=%v", tok.Reasoning)
	}
}

func TestParseCacheCreationWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	content := `{"type":"assistant","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":80},"cache_read_input_tokens":3}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir, "")
	res := ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: path, StableID: path})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	tok := res.Snapshot.Tokens
	if tok.CacheWrite5m == nil || *tok.CacheWrite5m != 20 {
		t.Fatalf("5m=%v", tok.CacheWrite5m)
	}
	if tok.CacheWrite1h == nil || *tok.CacheWrite1h != 80 {
		t.Fatalf("1h=%v", tok.CacheWrite1h)
	}
	if tok.CacheWrite == nil || *tok.CacheWrite != 100 {
		t.Fatalf("cache_write total=%v", tok.CacheWrite)
	}
}

func TestParseDedupesStreamedChunksByMessageID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamed.jsonl")
	// Claude Code writes one line per streamed chunk of the same assistant
	// message: input/cache_read repeat unchanged, output grows as the
	// stream completes. Only the final (most complete) chunk per message.id
	// must count, or input/cache_read get multiplied by the chunk count.
	content := `{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":2,"output_tokens":2,"cache_read_input_tokens":32000}}}
{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":2,"output_tokens":518,"cache_read_input_tokens":32000}}}
{"type":"assistant","message":{"id":"msg_2","model":"claude-sonnet-5","usage":{"input_tokens":3,"output_tokens":40,"cache_read_input_tokens":9000}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir, "")
	res := ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: path, StableID: path})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	tok := res.Snapshot.Tokens
	if tok.Input == nil || *tok.Input != 5 {
		t.Fatalf("input=%v want 2+3=5 (msg_1 counted once)", tok.Input)
	}
	if tok.Output == nil || *tok.Output != 558 {
		t.Fatalf("output=%v want 518+40=558 (final chunk of msg_1, not both)", tok.Output)
	}
	if tok.CacheRead == nil || *tok.CacheRead != 41000 {
		t.Fatalf("cache_read=%v want 32000+9000=41000, not 32000*2+9000", tok.CacheRead)
	}
}

func TestMissingTokensStayNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir, "")
	res := ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: path, StableID: path})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	tok := res.Snapshot.Tokens
	if tok.Input != nil || tok.Output != nil {
		t.Fatalf("expected nil tokens, got %+v", tok)
	}
}

// ---------- Job state.json tests ----------

func TestParseJobState_OrphanWithTokens(t *testing.T) {
	dir := t.TempDir()
	jobDir := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"state":        "stopped",
		"tokens":       18393,
		"sessionId":    "abc123-session",
		"cwd":          "/home/user/project",
		"name":         "Build feature X",
		"intent":       "implement the new API",
		"createdAt":    "2026-07-24T21:46:03.926Z",
		"updatedAt":    "2026-08-15T11:02:10.125Z",
		"linkScanPath": "/nonexistent/path/to/deleted.jsonl",
		"respawnFlags": []string{"--model", "opus"},
	}
	data, _ := json.Marshal(state)
	statePath := filepath.Join(jobDir, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ad := claudecode.New("", dir)
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		SourcePath: statePath,
		StableID:   statePath,
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.Snapshot == nil {
		t.Fatal("nil snapshot")
	}
	snap := res.Snapshot
	if snap.ParseStatus != "partial" {
		t.Fatalf("parse_status=%q want partial", snap.ParseStatus)
	}
	if snap.Tokens.Total == nil || *snap.Tokens.Total != 18393 {
		t.Fatalf("total=%v want 18393", snap.Tokens.Total)
	}
	// Individual fields must be nil — no breakdown available.
	if snap.Tokens.Input != nil {
		t.Fatalf("input should be nil, got %v", snap.Tokens.Input)
	}
	if snap.Tokens.Output != nil {
		t.Fatalf("output should be nil, got %v", snap.Tokens.Output)
	}
	if snap.Tokens.CacheRead != nil {
		t.Fatalf("cache_read should be nil, got %v", snap.Tokens.CacheRead)
	}
	if snap.Tokens.CacheWrite != nil {
		t.Fatalf("cache_write should be nil, got %v", snap.Tokens.CacheWrite)
	}
	if snap.Model != "opus" {
		t.Fatalf("model=%q want opus", snap.Model)
	}
	if snap.CWD != "/home/user/project" {
		t.Fatalf("cwd=%q", snap.CWD)
	}
	if snap.ProviderSessionID != "abc123-session" {
		t.Fatalf("session=%q", snap.ProviderSessionID)
	}
}

func TestParseJobState_ZeroTokensSkipped(t *testing.T) {
	dir := t.TempDir()
	jobDir := filepath.Join(dir, "empty-job")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"state":     "done",
		"tokens":    0,
		"sessionId": "empty-session",
	}
	data, _ := json.Marshal(state)
	statePath := filepath.Join(jobDir, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ad := claudecode.New("", dir)
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		SourcePath: statePath,
		StableID:   statePath,
	})
	if !res.Skip {
		t.Fatal("expected skip for zero-token job")
	}
}

func TestParseJobState_NilTokensSkipped(t *testing.T) {
	dir := t.TempDir()
	jobDir := filepath.Join(dir, "nil-job")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No "tokens" field at all
	state := map[string]any{
		"state":     "done",
		"sessionId": "nil-session",
	}
	data, _ := json.Marshal(state)
	statePath := filepath.Join(jobDir, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ad := claudecode.New("", dir)
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		SourcePath: statePath,
		StableID:   statePath,
	})
	if !res.Skip {
		t.Fatal("expected skip for nil-token job")
	}
}

func TestDiscoverOrphanJobs(t *testing.T) {
	// Set up a projects root with one .jsonl file.
	projectsDir := t.TempDir()
	projFile := filepath.Join(projectsDir, "proj", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(projFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projFile, []byte(`{"type":"user"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up a jobs root with 3 jobs:
	// 1. linked to the existing project .jsonl → should NOT appear
	// 2. linked to a non-existent .jsonl → should appear (orphan)
	// 3. no linkScanPath → should appear (orphan)
	jobsDir := t.TempDir()

	// Job 1: covered
	makeJobState(t, jobsDir, "covered", map[string]any{
		"tokens":       5000,
		"linkScanPath": projFile,
		"sessionId":    "covered-session",
	})

	// Job 2: orphan (deleted transcript)
	makeJobState(t, jobsDir, "orphan-deleted", map[string]any{
		"tokens":       10000,
		"linkScanPath": "/nonexistent/deleted.jsonl",
		"sessionId":    "orphan-deleted-session",
	})

	// Job 3: orphan (no link)
	makeJobState(t, jobsDir, "orphan-nolink", map[string]any{
		"tokens":    3000,
		"sessionId": "orphan-nolink-session",
	})

	ad := claudecode.New(projectsDir, jobsDir)
	sources, err := ad.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should have: 1 project .jsonl + 2 orphan state.json = 3
	if len(sources) != 3 {
		var paths []string
		for _, s := range sources {
			paths = append(paths, s.SourcePath)
		}
		t.Fatalf("expected 3 sources, got %d: %v", len(sources), paths)
	}

	// Verify the orphans are state.json files.
	stateCount := 0
	for _, s := range sources {
		if filepath.Base(s.SourcePath) == "state.json" {
			stateCount++
		}
	}
	if stateCount != 2 {
		t.Fatalf("expected 2 state.json sources, got %d", stateCount)
	}
}

func TestDiscoverIncludesSubagentTranscripts(t *testing.T) {
	projectsDir := t.TempDir()
	jobsDir := t.TempDir()
	parentPath := filepath.Join(projectsDir, "project-slug", "session-id.jsonl")
	subagentPath := filepath.Join(projectsDir, "project-slug", "session-id", "subagents", "agent-id.jsonl")

	for path, content := range map[string]string{
		parentPath:   `{"type":"user"}` + "\n",
		subagentPath: `{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet","usage":{"input_tokens":2,"output_tokens":3}}}` + "\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ad := claudecode.New(projectsDir, jobsDir)
	sources, err := ad.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected parent and subagent sources, got %d: %+v", len(sources), sources)
	}

	var subagentSource *model.SourceDescriptor
	for i := range sources {
		if sources[i].SourcePath == subagentPath {
			subagentSource = &sources[i]
			break
		}
	}
	if subagentSource == nil {
		t.Fatalf("subagent transcript not discovered: %s", subagentPath)
	}

	res := ad.Parse(context.Background(), *subagentSource)
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.Snapshot == nil || res.Snapshot.Tokens.Input == nil || *res.Snapshot.Tokens.Input != 2 {
		t.Fatalf("subagent input tokens not calculated: %+v", res.Snapshot)
	}
	if res.Snapshot.Tokens.Output == nil || *res.Snapshot.Tokens.Output != 3 {
		t.Fatalf("subagent output tokens not calculated: %+v", res.Snapshot.Tokens)
	}
}

func makeJobState(t *testing.T, jobsDir, name string, state map[string]any) {
	t.Helper()
	dir := filepath.Join(jobsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
