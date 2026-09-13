package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/adapter/antigravity"
	"github.com/seif/token-usage-service/internal/adapter/claudecode"
	"github.com/seif/token-usage-service/internal/adapter/codex"
	"github.com/seif/token-usage-service/internal/adapter/cursor"
	"github.com/seif/token-usage-service/internal/adapter/cursorusage"
	"github.com/seif/token-usage-service/internal/adapter/gemini"
	"github.com/seif/token-usage-service/internal/adapter/githubcopilot"
	"github.com/seif/token-usage-service/internal/adapter/opencode"
	"github.com/seif/token-usage-service/internal/config"
	"github.com/seif/token-usage-service/internal/pipeline"
	"github.com/seif/token-usage-service/internal/sink/postgres"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	force := hasFlag("--force")

	cfg, err := config.Load(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(2)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := context.Background()

	switch cmd {
	case "migrate":
		if cfg.Sink != "postgres" {
			fmt.Fprintln(os.Stderr, "migrate only supported for SINK=postgres")
			os.Exit(2)
		}
		s, err := postgres.Open(cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "postgres: %v\n", err)
			os.Exit(3)
		}
		defer s.Close()
		if err := s.Migrate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
			os.Exit(3)
		}
		fmt.Println("migrate: ok")
	case "validate-config":
		fmt.Printf("host_id=%s sink=%s vendors=%s\n", cfg.HostID, cfg.Sink, strings.Join(cfg.EnabledVendors, ","))
		if cfg.Sink == "postgres" {
			s, err := postgres.Open(cfg.DatabaseURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "postgres: %v\n", err)
				os.Exit(3)
			}
			_ = s.Close()
			fmt.Println("postgres: reachable")
		}
		fmt.Println("validate-config: ok")
	case "ingest", "run", "--once":
		if cmd == "--once" {
			// allow `ingest --once` style when binary is named ingest; here subcommand is first arg
		}
		adapters, err := buildAdapters(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "adapters: %v\n", err)
			os.Exit(2)
		}
		if cfg.Sink != "postgres" {
			fmt.Fprintf(os.Stderr, "unsupported sink %q (v1: postgres only)\n", cfg.Sink)
			os.Exit(2)
		}
		s, err := postgres.Open(cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "postgres: %v\n", err)
			os.Exit(3)
		}
		defer s.Close()

		r := &pipeline.Runner{
			HostID:   cfg.HostID,
			Adapters: adapters,
			Sink:     s,
			Force:    force,
			Log:      log,
		}
		sum, err := r.Run(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest failed: %v\n", err)
			os.Exit(3)
		}
		fmt.Printf("scanned=%d unchanged_skipped=%d parsed=%d upserted=%d deferred=%d errors=%d path_remotes=%d\n",
			sum.Scanned, sum.UnchangedSkipped, sum.Parsed, sum.Upserted, sum.Deferred, sum.Errors, sum.PathRemotesUpsert)
		// Per-source errors still exit 0 (PRD)
		os.Exit(0)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func buildAdapters(cfg *config.Config) ([]adapter.VendorAdapter, error) {
	var out []adapter.VendorAdapter
	for _, name := range cfg.EnabledVendors {
		switch name {
		case "claude-code":
			out = append(out, claudecode.New(cfg.ClaudeProjectsRoot))
		case "codex":
			out = append(out, codex.New(cfg.CodexRoot))
		case "opencode":
			out = append(out, opencode.New(cfg.OpenCodeDBPath))
		case "github-copilot":
			out = append(out, githubcopilot.New(cfg.CopilotDBPath, time.Time{}, cfg.CopilotForceLegacy))
		case "gemini":
			out = append(out, gemini.New(cfg.GeminiTmpRoot))
		case "antigravity":
			out = append(out, antigravity.New(cfg.AntigravityRoot))
		case "cursor":
			out = append(out, cursor.New(cfg.CursorStateDB))
		case "cursor-usage":
			out = append(out, cursorusage.New(cfg.CursorUsageReportsDir))
		default:
			return nil, fmt.Errorf("unknown adapter %q (known: claude-code, codex, opencode, github-copilot, gemini, antigravity, cursor, cursor-usage)", name)
		}
	}
	return out, nil
}

func hasFlag(name string) bool {
	for _, a := range os.Args[2:] {
		if a == name {
			return true
		}
	}
	return false
}

func usage() {
	fmt.Fprintf(os.Stderr, `token-usage-service ingest CLI

Usage:
  go run ./cmd/ingest migrate
  go run ./cmd/ingest validate-config
  go run ./cmd/ingest ingest [--force]

Environment: load from .env (see .env.example)
`)
}
