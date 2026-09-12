package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/seif/token-usage-service/internal/adapter/codex"
	"github.com/seif/token-usage-service/internal/model"
)

func TestParseUsesLastThreadTokenUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-test.jsonl")
	content := `{"type":"session_meta","timestamp":"2026-09-09T20:00:00Z","payload":{"session_id":"sess-1","cwd":"/tmp/proj","model_provider":"openai"}}
{"type":"token_usage_record","timestamp":"2026-09-09T20:01:00Z","payload":{"session_id":"sess-1","thread_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":10,"reasoning_output_tokens":2,"total_tokens":110}}}
{"type":"token_usage_record","timestamp":"2026-09-09T20:02:00Z","payload":{"session_id":"sess-1","thread_token_usage":{"input_tokens":500,"cached_input_tokens":400,"cache_write_input_tokens":0,"output_tokens":50,"reasoning_output_tokens":5,"total_tokens":550}}}
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
	if res.Snapshot.Tokens.Input == nil || *res.Snapshot.Tokens.Input != 500 {
		t.Fatalf("input=%v want last cumulative 500", res.Snapshot.Tokens.Input)
	}
	if res.Snapshot.Tokens.Total == nil || *res.Snapshot.Tokens.Total != 550 {
		t.Fatalf("total=%v", res.Snapshot.Tokens.Total)
	}
	if res.Snapshot.CWD != "/tmp/proj" {
		t.Fatalf("cwd=%s", res.Snapshot.CWD)
	}
}
