// Command loadworkstyle loads raw work-style log lines (slash-command
// history, prompt history, session indexes, job timelines) into the
// raw_workstyle_logs staging table, as-is. Parsing into structured fields is
// deferred to a later pass.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type file struct {
	kind string
	path string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	hostID := os.Getenv("HOST_ID")
	if dbURL == "" || hostID == "" {
		fmt.Fprintln(os.Stderr, "usage: DATABASE_URL=... HOST_ID=... loadworkstyle <kind>=<path> [<kind>=<path> ...]")
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: loadworkstyle <kind>=<path> [<kind>=<path> ...]")
		os.Exit(2)
	}

	var files []file
	for _, arg := range os.Args[1:] {
		i := indexByte(arg, '=')
		if i <= 0 || i == len(arg)-1 {
			fmt.Fprintf(os.Stderr, "bad arg %q, want kind=path\n", arg)
			os.Exit(2)
		}
		files = append(files, file{kind: arg[:i], path: arg[i+1:]})
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(3)
	}
	defer db.Close()

	ctx := context.Background()
	total := 0
	for _, f := range files {
		n, err := loadFile(ctx, db, hostID, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load %s (%s): %v\n", f.path, f.kind, err)
			os.Exit(3)
		}
		fmt.Printf("kind=%s path=%s lines=%d\n", f.kind, f.path, n)
		total += n
	}
	fmt.Printf("total=%d\n", total)
}

func loadFile(ctx context.Context, db *sql.DB, hostID string, f file) (int, error) {
	fh, err := os.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer fh.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO raw_workstyle_logs (host_id, source_kind, source_path, line_no, raw)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (host_id, source_path, line_no) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	lineNo := 0
	inserted := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(bytesTrim(line)) == 0 {
			continue
		}
		if !json.Valid(line) {
			continue
		}
		res, err := stmt.ExecContext(ctx, hostID, f.kind, f.path, lineNo, string(line))
		if err != nil {
			return inserted, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	if err := scanner.Err(); err != nil {
		return inserted, err
	}

	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func bytesTrim(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
