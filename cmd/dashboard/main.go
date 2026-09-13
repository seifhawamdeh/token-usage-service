package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/seif/token-usage-service/internal/dashboard"
)

func main() {
	_ = godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	addr := os.Getenv("DASHBOARD_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dayOffset := 3 * time.Hour
	if v := os.Getenv("DASHBOARD_DAY_OFFSET_HOURS"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("DASHBOARD_DAY_OFFSET_HOURS: %v", err)
		}
		dayOffset = time.Duration(hours) * time.Hour
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("postgres: %v", err)
	}

	api := &dashboard.API{DB: db, DayOffset: dayOffset}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.Health)
	mux.HandleFunc("GET /api/ingest-health", api.IngestHealth)
	mux.HandleFunc("GET /api/machines", api.Machines)
	mux.HandleFunc("GET /api/projects", api.Projects)
	mux.HandleFunc("GET /api/remotes", api.Remotes)
	mux.HandleFunc("POST /api/remotes/map", api.MapRemote)
	mux.HandleFunc("GET /api/cwds", api.Cwds)
	mux.HandleFunc("POST /api/cwds/map", api.MapCwd)
	mux.HandleFunc("GET /api/summary", api.Summary)
	mux.HandleFunc("GET /api/by-vendor", api.ByVendor)
	mux.HandleFunc("GET /api/by-model", api.ByModel)
	mux.HandleFunc("GET /api/by-project", api.ByProject)
	mux.HandleFunc("GET /api/daily", api.Daily)
	mux.HandleFunc("GET /api/snapshots", api.Snapshots)
	mux.HandleFunc("GET /api/ledger", api.Ledger)
	mux.HandleFunc("GET /api/duplicates", api.Duplicates)
	mux.Handle("/", dashboard.StaticHandler())

	network := "tcp"
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatalf("invalid DASHBOARD_ADDR %q: %v", addr, err)
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}
	listener, err := net.Listen(network, addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dashboard listening on %s (%s)", listener.Addr(), network)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatal(err)
	}
}
