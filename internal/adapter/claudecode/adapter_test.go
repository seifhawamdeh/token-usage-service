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

func TestParseCacheCreationWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	content := `{"type":"assistant","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":80},"cache_read_input_tokens":3}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := claudecode.New(dir)
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
	ad := claudecode.New(dir)
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
