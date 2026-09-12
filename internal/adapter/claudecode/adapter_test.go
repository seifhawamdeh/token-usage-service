package claudecode_test

import (
	"context"
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
	ad := claudecode.New(dir)
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

func TestMissingTokensStayNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir)
	res := ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: path, StableID: path})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	tok := res.Snapshot.Tokens
	if tok.Input != nil || tok.Output != nil {
		t.Fatalf("expected nil tokens, got %+v", tok)
	}
}
