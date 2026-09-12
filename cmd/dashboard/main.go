package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
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

	api := &dashboard.API{DB: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.Health)
	mux.HandleFunc("GET /api/summary", api.Summary)
	mux.HandleFunc("GET /api/by-vendor", api.ByVendor)
	mux.HandleFunc("GET /api/by-model", api.ByModel)
	mux.HandleFunc("GET /api/daily", api.Daily)
	mux.HandleFunc("GET /api/snapshots", api.Snapshots)
	mux.Handle("/", dashboard.StaticHandler())

	log.Printf("dashboard listening on http://127.0.0.1%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
