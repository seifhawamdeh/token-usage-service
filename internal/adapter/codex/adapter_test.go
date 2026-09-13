package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/seif/token-usage-service/internal/adapter/codex"
	"github.com/seif/token-usage-service/internal/model"
)

func TestDiscoverFindsRolloutsAcrossCodexRoot(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "sessions", "2026", "rollout-normal.jsonl"),
		filepath.Join(root, "background-sessions", "rollout-background.jsonl"),
		filepath.Join(root, "future-layout", "nested", "rollout-delegated.jsonl"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "history.jsonl"),
		filepath.Join(root, ".tmp", "responses.jsonl"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sources, err := codex.New(root).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, source := range sources {
		got = append(got, source.SourcePath)
	}
	sort.Strings(got)
	sort.Strings(paths)
	if len(got) != len(paths) {
		t.Fatalf("got %d sources (%v), want %d (%v)", len(got), got, len(paths), paths)
	}
	for i := range paths {
		if got[i] != paths[i] {
			t.Fatalf("source[%d]=%q, want %q", i, got[i], paths[i])
		}
	}
}

func TestParseSumsSegmentPeaksAcrossCompactionResets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-test.jsonl")
	// Codex's total_token_usage counter is cumulative within a segment but
	// resets to near-zero on context compaction. sess-1 simulates two
	// segments: [200, 550] then a reset down to [80, 110]. The true session
	// total is the sum of each segment's peak (550 + 110 = 660), not a
	// single file-wide max (which would silently drop the first segment).
	content := `{"type":"session_meta","timestamp":"2026-09-09T20:00:00Z","payload":{"session_id":"sess-1","cwd":"/tmp/proj","model_provider":"openai"}}
{"type":"event_msg","timestamp":"2026-09-09T20:00:30Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":180,"cached_input_tokens":100,"output_tokens":15,"reasoning_output_tokens":1,"total_tokens":200}}}}
{"type":"event_msg","timestamp":"2026-09-09T20:01:00Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":500,"cached_input_tokens":400,"cache_write_input_tokens":0,"output_tokens":50,"reasoning_output_tokens":5,"total_tokens":550}}}}
{"type":"event_msg","timestamp":"2026-09-09T20:01:30Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":70,"cached_input_tokens":40,"output_tokens":8,"reasoning_output_tokens":1,"total_tokens":80}}}}
{"type":"event_msg","timestamp":"2026-09-09T20:02:00Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":10,"reasoning_output_tokens":2,"total_tokens":110}}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := codex.New(dir)
	res := ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: path, StableID: path})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.Snapshot.StableID != "sess-1" {
		t.Fatalf("stable_id=%s", res.Snapshot.StableID)
	}
	// Fresh-only input: (500-400) + (100-40) = 100+60 = 160. Codex's
	// input_tokens includes cached_input_tokens, so the cached slice must be
	// subtracted before storing tokens_input, or the rating view double-bills
	// it (once as input, once as cache_read).
	if res.Snapshot.Tokens.Input == nil || *res.Snapshot.Tokens.Input != 160 {
		t.Fatalf("input=%v want fresh-only (500-400)+(100-40)=160", res.Snapshot.Tokens.Input)
	}
	if res.Snapshot.Tokens.Output == nil || *res.Snapshot.Tokens.Output != 60 {
		t.Fatalf("output=%v want 50+10=60", res.Snapshot.Tokens.Output)
	}
	if res.Snapshot.Tokens.CacheRead == nil || *res.Snapshot.Tokens.CacheRead != 440 {
		t.Fatalf("cache_read=%v want 400+40=440", res.Snapshot.Tokens.CacheRead)
	}
	if res.Snapshot.Tokens.Total == nil || *res.Snapshot.Tokens.Total != 660 {
		t.Fatalf("total=%v want 550+110=660, not a single file-wide max", res.Snapshot.Tokens.Total)
	}
	if res.Snapshot.CWD != "/tmp/proj" {
		t.Fatalf("cwd=%s", res.Snapshot.CWD)
	}
}
