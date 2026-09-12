package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type API struct {
	DB *sql.DB
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.DB.PingContext(ctx); err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) Summary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	row := a.DB.QueryRowContext(ctx, `
		SELECT
			count(*)::bigint,
			count(*) FILTER (WHERE vendor <> 'cursor')::bigint,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(tokens_cache_read), 0),
			COALESCE(sum(tokens_cache_write), 0),
			COALESCE(sum(tokens_cache_write_5m), 0),
			COALESCE(sum(tokens_cache_write_1h), 0),
			COALESCE(sum(rated_cost_usd), 0),
			COALESCE(sum(provider_cost), 0),
			count(rated_cost_usd)::bigint,
			min(last_event_at),
			max(last_event_at)
		FROM v_burn_usage_rated
	`)
	var (
		sources, nonCursor int64
		in, out, cr, cw, cw5, cw1h int64
		rated, provider float64
		ratedRows int64
		minAt, maxAt sql.NullTime
	)
	if err := row.Scan(&sources, &nonCursor, &in, &out, &cr, &cw, &cw5, &cw1h, &rated, &provider, &ratedRows, &minAt, &maxAt); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"sources":              sources,
		"sources_non_cursor":   nonCursor,
		"tokens_input":         in,
		"tokens_output":        out,
		"tokens_cache_read":    cr,
		"tokens_cache_write":   cw,
		"tokens_cache_write_5m": cw5,
		"tokens_cache_write_1h": cw1h,
		"rated_cost_usd":       rated,
		"provider_cost_sum":    provider,
		"sources_with_rated_cost": ratedRows,
		"first_event_at":       nullTime(minAt),
		"last_event_at":        nullTime(maxAt),
	})
}

func (a *API) ByVendor(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT vendor,
			count(*)::bigint,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(tokens_cache_read), 0),
			COALESCE(sum(tokens_cache_write), 0),
			COALESCE(sum(rated_cost_usd), 0),
			count(rated_cost_usd)::bigint
		FROM v_burn_usage_rated
		GROUP BY vendor
		ORDER BY COALESCE(sum(rated_cost_usd), 0) DESC, count(*) DESC
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var vendor string
		var n, in, outTok, cr, cw, ratedN int64
		var rated float64
		if err := rows.Scan(&vendor, &n, &in, &outTok, &cr, &cw, &rated, &ratedN); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"vendor": vendor, "sources": n,
			"tokens_input": in, "tokens_output": outTok,
			"tokens_cache_read": cr, "tokens_cache_write": cw,
			"rated_cost_usd": rated, "sources_with_rated_cost": ratedN,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) ByModel(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 40)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT COALESCE(NULLIF(model, ''), '(unknown)') AS model,
			COALESCE(vendor, '') AS vendor,
			count(*)::bigint,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(rated_cost_usd), 0)
		FROM v_burn_usage_rated
		GROUP BY 1, 2
		ORDER BY COALESCE(sum(rated_cost_usd), 0) DESC, count(*) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var model, vendor string
		var n, in, outTok int64
		var rated float64
		if err := rows.Scan(&model, &vendor, &n, &in, &outTok, &rated); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"model": model, "vendor": vendor, "sources": n,
			"tokens_input": in, "tokens_output": outTok, "rated_cost_usd": rated,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) Daily(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 45)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT day::date::text,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(rated_cost_usd), 0),
			count(*)::bigint
		FROM (
			SELECT date_trunc('day', COALESCE(last_event_at, started_at, ingested_at)) AS day,
				tokens_input, tokens_output, rated_cost_usd
			FROM v_burn_usage_rated
			WHERE COALESCE(last_event_at, started_at, ingested_at) >= (CURRENT_TIMESTAMP - make_interval(days => $1))
		) t
		GROUP BY day
		ORDER BY day
	`, days)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var day string
		var in, outTok, n int64
		var rated float64
		if err := rows.Scan(&day, &in, &outTok, &rated, &n); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"day": day, "tokens_input": in, "tokens_output": outTok,
			"rated_cost_usd": rated, "sources": n,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) Snapshots(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	vendor := r.URL.Query().Get("vendor")

	q := `
		SELECT source_id, vendor, COALESCE(model, ''), source_path,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_cache_write_5m, tokens_cache_write_1h,
			rated_cost_usd, provider_cost, last_event_at, started_at
		FROM v_burn_usage_rated
	`
	args := []any{}
	if vendor != "" {
		q += ` WHERE vendor = $1`
		args = append(args, vendor)
		q += ` ORDER BY COALESCE(last_event_at, started_at, ingested_at) DESC NULLS LAST LIMIT $2 OFFSET $3`
		args = append(args, limit, offset)
	} else {
		q += ` ORDER BY COALESCE(last_event_at, started_at, ingested_at) DESC NULLS LAST LIMIT $1 OFFSET $2`
		args = append(args, limit, offset)
	}

	rows, err := a.DB.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id, vendor, model, path string
			in, outTok, cr, cw      sql.NullInt64
			cw5, cw1h               sql.NullInt64
			rated, pcost            sql.NullFloat64
			lastAt, startAt         sql.NullTime
		)
		if err := rows.Scan(&id, &vendor, &model, &path, &in, &outTok, &cr, &cw, &cw5, &cw1h, &rated, &pcost, &lastAt, &startAt); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"source_id": id, "vendor": vendor, "model": model, "source_path": path,
			"tokens_input": nullInt(in), "tokens_output": nullInt(outTok),
			"tokens_cache_read": nullInt(cr), "tokens_cache_write": nullInt(cw),
			"tokens_cache_write_5m": nullInt(cw5), "tokens_cache_write_1h": nullInt(cw1h),
			"rated_cost_usd": nullFloat(rated), "provider_cost": nullFloat(pcost),
			"last_event_at": nullTime(lastAt), "started_at": nullTime(startAt),
		})
	}
	writeJSON(w, 200, out)
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	if n > 500 {
		return 500
	}
	return n
}

func nullTime(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}
