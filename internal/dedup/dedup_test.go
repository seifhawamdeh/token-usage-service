package dedup

import (
	"testing"
	"time"

	"github.com/seif/token-usage-service/internal/model"
)

func ptr(v int64) *int64 { return &v }

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestDetect_ProviderSessionID(t *testing.T) {
	snaps := []model.BurnSnapshot{
		{SourceID: "a", Vendor: "claude-code", ProviderSessionID: "sess-1", Tokens: model.Tokens{Total: ptr(100)}},
		{SourceID: "b", Vendor: "claude-code", ProviderSessionID: "sess-1", Tokens: model.Tokens{Total: ptr(140)}},
		{SourceID: "c", Vendor: "claude-code", ProviderSessionID: "sess-2", Tokens: model.Tokens{Total: ptr(10)}},
	}
	groups := Detect(snaps)
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1", len(groups))
	}
	g := groups[0]
	if g.Basis != "provider_session_id" {
		t.Fatalf("basis=%s", g.Basis)
	}
	if g.CanonicalSourceID != "b" {
		t.Fatalf("canonical=%s want b (highest tokens)", g.CanonicalSourceID)
	}
	if len(g.SourceIDs) != 2 {
		t.Fatalf("members=%v", g.SourceIDs)
	}
}

func TestDetect_TimeWindowAcrossVendors(t *testing.T) {
	snaps := []model.BurnSnapshot{
		{
			SourceID: "a", HostID: "host1", Vendor: "claude-code", SourcePath: "/a",
			CWD: "/repo", StartedAt: at("2026-01-01T10:00:00Z"), LastEventAt: at("2026-01-01T10:20:00Z"),
			Tokens: model.Tokens{Total: ptr(500)},
		},
		{
			SourceID: "b", HostID: "host1", Vendor: "cursor", SourcePath: "/b",
			CWD: "/repo", StartedAt: at("2026-01-01T10:05:00Z"), LastEventAt: at("2026-01-01T10:15:00Z"),
			Tokens: model.Tokens{Total: nil},
		},
		{
			SourceID: "c", HostID: "host1", Vendor: "codex", SourcePath: "/c",
			CWD: "/repo", StartedAt: at("2026-01-01T14:00:00Z"), LastEventAt: at("2026-01-01T14:10:00Z"),
			Tokens: model.Tokens{Total: ptr(50)},
		},
	}
	groups := Detect(snaps)
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1: %+v", len(groups), groups)
	}
	g := groups[0]
	if g.Basis != "time_window" {
		t.Fatalf("basis=%s", g.Basis)
	}
	if len(g.SourceIDs) != 2 {
		t.Fatalf("members=%v, want a+b only (c is 4h later)", g.SourceIDs)
	}
	if g.CanonicalSourceID != "a" {
		t.Fatalf("canonical=%s want a (has tokens, b is nil)", g.CanonicalSourceID)
	}
}

func TestDetect_SamePathNotDuplicate(t *testing.T) {
	snaps := []model.BurnSnapshot{
		{SourceID: "a", HostID: "host1", Vendor: "claude-code", SourcePath: "/same", CWD: "/repo", StartedAt: at("2026-01-01T10:00:00Z")},
		{SourceID: "b", HostID: "host1", Vendor: "claude-code", SourcePath: "/same", CWD: "/repo", StartedAt: at("2026-01-01T10:01:00Z")},
	}
	groups := Detect(snaps)
	if len(groups) != 0 {
		t.Fatalf("groups=%d want 0 (same source_path)", len(groups))
	}
}

func TestDetect_NoFalsePositiveAcrossProjects(t *testing.T) {
	snaps := []model.BurnSnapshot{
		{SourceID: "a", HostID: "host1", Vendor: "claude-code", SourcePath: "/a", CWD: "/repo-1", StartedAt: at("2026-01-01T10:00:00Z")},
		{SourceID: "b", HostID: "host1", Vendor: "cursor", SourcePath: "/b", CWD: "/repo-2", StartedAt: at("2026-01-01T10:00:00Z")},
	}
	groups := Detect(snaps)
	if len(groups) != 0 {
		t.Fatalf("groups=%d want 0 (different cwd)", len(groups))
	}
}
