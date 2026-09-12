package githubcopilot_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/seif/token-usage-service/internal/adapter/githubcopilot"
	"github.com/seif/token-usage-service/internal/model"
)

func TestBillingRegimeByDate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "session-store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd TEXT);
		CREATE TABLE assistant_usage_events (
			id INTEGER PRIMARY KEY,
			session_id TEXT,
			model TEXT,
			input_tokens INTEGER,
			output_tokens INTEGER,
			cache_read_tokens INTEGER,
			cache_write_tokens INTEGER,
			reasoning_tokens INTEGER,
			total_nano_aiu INTEGER,
			request_multiplier REAL,
			created_at TEXT
		);
		INSERT INTO sessions VALUES ('s1', '/tmp/x');
		INSERT INTO assistant_usage_events VALUES
		 (1,'s1','gpt',10,2,0,0,0,1000,1.0,'2026-05-01T12:00:00Z'),
		 (2,'s1','gpt',5,1,0,0,0,500,2.0,'2026-05-01T13:00:00Z');
	`)
	if err != nil {
		t.Fatal(err)
	}

	ad := githubcopilot.New(dbPath, time.Time{}, false)
	res := ad.Parse(context.Background(), model.SourceDescriptor{
		SourcePath: dbPath + "#session:s1",
		StableID:   "s1",
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.Snapshot.BillingRegime != "legacy-premium-requests" {
		t.Fatalf("regime=%s", res.Snapshot.BillingRegime)
	}
	if res.Snapshot.Tokens.Input == nil || *res.Snapshot.Tokens.Input != 15 {
		t.Fatalf("input=%v", res.Snapshot.Tokens.Input)
	}

	// Post-cutover
	_, _ = db.Exec(`UPDATE assistant_usage_events SET created_at='2026-07-01T12:00:00Z'`)
	res = ad.Parse(context.Background(), model.SourceDescriptor{SourcePath: dbPath + "#session:s1", StableID: "s1"})
	if res.Snapshot.BillingRegime != "ai-credits" {
		t.Fatalf("regime=%s", res.Snapshot.BillingRegime)
	}
}
