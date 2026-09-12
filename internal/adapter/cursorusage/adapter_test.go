package cursorusage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/seif/token-usage-service/internal/adapter/cursorusage"
)

func TestParseUsageCSV(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "usage-events-august.csv")
	body := `Date,Cloud Agent ID,Automation ID,Kind,Model,Max Mode,Input (w/ Cache Write),Input (w/o Cache Write),Cache Read,Output Tokens,Total Tokens,Cost
"2026-08-31T17:26:39.740Z","","","Included","auto","No","0","100","200","30","330","Included"
"2026-08-30T12:00:00.000Z","","","Included","claude-4.5-sonnet-thinking","No","10","50","1000","20","1080","0.15"
`
	if err := os.WriteFile(csvPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	ad := cursorusage.New(dir)
	srcs, err := ad.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 2 {
		t.Fatalf("want 2 events, got %d", len(srcs))
	}
	res := ad.Parse(context.Background(), srcs[0])
	if res.Error != nil || res.Snapshot == nil {
		t.Fatalf("parse: %+v", res)
	}
	if res.Snapshot.Tokens.Total == nil || *res.Snapshot.Tokens.Total != 330 {
		t.Fatalf("total tokens: %+v", res.Snapshot.Tokens.Total)
	}
	if res.Snapshot.BillingRegime != "Included" {
		t.Fatalf("kind/regime: %s", res.Snapshot.BillingRegime)
	}
	res2 := ad.Parse(context.Background(), srcs[1])
	if res2.Snapshot.ProviderCost == nil || *res2.Snapshot.ProviderCost != 0.15 {
		t.Fatalf("cost: %+v", res2.Snapshot.ProviderCost)
	}
}
