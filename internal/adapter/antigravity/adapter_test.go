package antigravity_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/seif/token-usage-service/internal/adapter/antigravity"
	"github.com/seif/token-usage-service/internal/model"
)

func encVarint(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			break
		}
	}
	return out
}

func fieldVarint(num, val uint64) []byte {
	out := encVarint(num << 3)
	out = append(out, encVarint(val)...)
	return out
}

func fieldLen(num uint64, payload []byte) []byte {
	out := encVarint((num << 3) | 2)
	out = append(out, encVarint(uint64(len(payload)))...)
	out = append(out, payload...)
	return out
}

func genBlob(system, input, cache, output, think uint64, total *uint64, respID, model string) []byte {
	var usage []byte
	usage = append(usage, fieldVarint(1, system)...)
	usage = append(usage, fieldVarint(2, input)...)
	if total != nil {
		usage = append(usage, fieldVarint(3, *total)...)
	}
	usage = append(usage, fieldVarint(5, cache)...)
	usage = append(usage, fieldVarint(9, output)...)
	usage = append(usage, fieldVarint(10, think)...)
	usage = append(usage, fieldLen(11, []byte(respID))...)
	var chat []byte
	chat = append(chat, fieldLen(4, usage)...)
	chat = append(chat, fieldLen(19, []byte(model))...)
	return fieldLen(1, chat)
}

func TestParseSumsVerifiedGens(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "conv-1.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE gen_metadata (idx INTEGER, data BLOB, size INTEGER);
		CREATE TABLE trajectory_metadata_blob (id INTEGER, data BLOB);
	`)
	if err != nil {
		t.Fatal(err)
	}
	tot := uint64(340)
	b1 := genBlob(100, 50, 10, 300, 40, &tot, "r1", "gemini-3-flash")
	b2 := genBlob(100, 20, 5, 10, 0, uint64Ptr(10), "r2", "gemini-3-flash")
	// drift row — ignored
	bBad := genBlob(1, 1, 0, 10, 0, uint64Ptr(99), "r3", "gemini-3-flash")
	for i, b := range [][]byte{b1, b2, bBad} {
		if _, err := db.Exec(`INSERT INTO gen_metadata (idx, data, size) VALUES (?, ?, ?)`, i, b, len(b)); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	ad := antigravity.New(dir) // conversations will be empty for Discover; Parse uses SourcePath
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		SourcePath: dbPath,
		StableID:   "conv-1",
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	tok := res.Snapshot.Tokens
	// inputs: (100+50)+(100+20)=270
	if tok.Input == nil || *tok.Input != 270 {
		t.Fatalf("input=%v", tok.Input)
	}
	// output text only (reasoning separate): 300+10=310
	if tok.Output == nil || *tok.Output != 310 {
		t.Fatalf("output=%v", tok.Output)
	}
	if tok.Reasoning == nil || *tok.Reasoning != 40 {
		t.Fatalf("reasoning=%v", tok.Reasoning)
	}
	if tok.CacheRead == nil || *tok.CacheRead != 15 {
		t.Fatalf("cache_read=%v", tok.CacheRead)
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }
