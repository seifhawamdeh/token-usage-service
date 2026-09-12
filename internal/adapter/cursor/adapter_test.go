package cursor_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/seif/token-usage-service/internal/adapter/cursor"
)

func TestDiscoverAllComposersEvenWithZeroTokens(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.vscdb")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT);
		CREATE TABLE composerHeaders (
			composerId TEXT PRIMARY KEY, workspaceId TEXT, createdAt INTEGER, lastUpdatedAt INTEGER
		);
		INSERT INTO composerHeaders (composerId, workspaceId, createdAt, lastUpdatedAt)
		VALUES ('c1', 'ws1', 1000, 2000);
		INSERT INTO cursorDiskKV VALUES
		 ('bubbleId:c1:b1', '{"tokenCount":{"inputTokens":0,"outputTokens":0},"modelInfo":{"modelName":"default"}}'),
		 ('bubbleId:c1:b2', '{"tokenCount":{"inputTokens":0,"outputTokens":0}}');
	`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	ad := cursor.New(dbPath)
	ad.TranscriptsRoot = dir
	srcs, err := ad.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 {
		t.Fatalf("want 1 session, got %d", len(srcs))
	}
	res := ad.Parse(context.Background(), srcs[0])
	if res.Error != nil || res.Skip || res.Snapshot == nil {
		t.Fatalf("parse: %+v", res)
	}
	if res.Snapshot.Tokens.Input != nil || res.Snapshot.Tokens.Output != nil {
		t.Fatalf("session adapter must leave tokens null, got %+v", res.Snapshot.Tokens)
	}
	if res.Snapshot.ProviderSessionID != "c1" {
		t.Fatalf("session id: %s", res.Snapshot.ProviderSessionID)
	}
}

func TestPositiveLocalTokensStillSessionOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.vscdb")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT);
		CREATE TABLE composerHeaders (
			composerId TEXT PRIMARY KEY, workspaceId TEXT, createdAt INTEGER, lastUpdatedAt INTEGER
		);
		INSERT INTO composerHeaders (composerId, createdAt, lastUpdatedAt) VALUES ('c1', 1000, 2000);
		INSERT INTO cursorDiskKV VALUES
		 ('bubbleId:c1:b2', '{"tokenCount":{"inputTokens":10,"outputTokens":3},"model":"gpt-5"}');
	`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	ad := cursor.New(dbPath)
	ad.TranscriptsRoot = dir
	srcs, err := ad.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	res := ad.Parse(context.Background(), srcs[0])
	if res.Snapshot == nil || res.Snapshot.Tokens.Input != nil {
		t.Fatalf("tokens must stay null on session row; detail carries local sparse counts: %+v", res.Snapshot)
	}
}
