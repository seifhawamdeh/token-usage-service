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
	// DayOffset shifts UTC timestamps before bucketing them into calendar
	// days, so "day" boundaries match the deployment's local timezone
	// instead of always being UTC+3.
	DayOffset time.Duration
}

func (a *API) dayOffsetHours() int {
	return int(a.DayOffset.Hours())
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

func (a *API) Projects(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT project, count(*)::bigint
		FROM v_burn_usage_rated
		WHERE project IS NOT NULL
		GROUP BY project
		ORDER BY project
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var project string
		var sources int64
		if err := rows.Scan(&project, &sources); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"project": project, "sources": sources})
	}
	writeJSON(w, 200, out)
}

func (a *API) Remotes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT pr.remote_url, p.project, count(distinct bs.source_id)::bigint AS snapshots
		FROM path_remotes pr
		LEFT JOIN project_remotes p ON pr.remote_url = p.remote_url
		LEFT JOIN burn_snapshots bs ON bs.host_id = pr.host_id AND bs.source_path = pr.source_path
		WHERE pr.remote_url IS NOT NULL AND pr.remote_url <> ''
		GROUP BY pr.remote_url, p.project
		ORDER BY pr.remote_url
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var remoteUrl string
		var project sql.NullString
		var snapshots int64
		if err := rows.Scan(&remoteUrl, &project, &snapshots); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		var projStr string
		if project.Valid {
			projStr = project.String
		}
		out = append(out, map[string]any{"remote_url": remoteUrl, "project": projStr, "snapshots": snapshots})
	}
	writeJSON(w, 200, out)
}

func (a *API) MapRemote(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeErr(w, 405, "method not allowed")
		return
	}
	var req struct {
		RemoteUrl string `json:"remote_url"`
		Project   string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.RemoteUrl == "" {
		writeErr(w, 400, "remote_url is required")
		return
	}
	
	if req.Project == "" {
		_, err := a.DB.ExecContext(r.Context(), "DELETE FROM project_remotes WHERE remote_url = $1", req.RemoteUrl)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := a.DB.ExecContext(r.Context(), `
			INSERT INTO project_remotes (remote_url, project) VALUES ($1, $2)
			ON CONFLICT (remote_url) DO UPDATE SET project = EXCLUDED.project
		`, req.RemoteUrl, req.Project)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) ByProject(w http.ResponseWriter, r *http.Request) {
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT COALESCE(project, '(unmapped)') AS project,
			count(*)::bigint,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(tokens_cache_read), 0),
			COALESCE(sum(tokens_cache_write), 0),
			COALESCE(sum(rated_cost_usd), 0),
			count(rated_cost_usd)::bigint
		FROM v_burn_usage_rated
		WHERE ($1 = '' OR host_id = $1)
		  AND ($2 = '' OR COALESCE(project, '(unmapped)') = $2)
		GROUP BY 1
		ORDER BY COALESCE(sum(rated_cost_usd), 0) DESC, count(*) DESC
	`, machine, project)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var project string
		var n, in, outTok, cr, cw, ratedN int64
		var rated float64
		if err := rows.Scan(&project, &n, &in, &outTok, &cr, &cw, &rated, &ratedN); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"project": project, "sources": n,
			"tokens_input": in, "tokens_output": outTok,
			"tokens_cache_read": cr, "tokens_cache_write": cw,
			"rated_cost_usd": rated, "sources_with_rated_cost": ratedN,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) Summary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
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
		WHERE ($1 = '' OR host_id = $1)
		  AND ($2 = '' OR COALESCE(project, '(unmapped)') = $2)
	`, machine, project)
	var (
		sources, nonCursor         int64
		in, outTok, cr, cw, cw5, cw1h int64
		rated, provider            float64
		ratedRows                  int64
		minAt, maxAt               sql.NullTime
	)
	if err := row.Scan(&sources, &nonCursor, &in, &outTok, &cr, &cw, &cw5, &cw1h, &rated, &provider, &ratedRows, &minAt, &maxAt); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"sources":                 sources,
		"sources_non_cursor":      nonCursor,
		"tokens_input":            in,
		"tokens_output":           outTok,
		"tokens_cache_read":       cr,
		"tokens_cache_write":      cw,
		"tokens_cache_write_5m":   cw5,
		"tokens_cache_write_1h":   cw1h,
		"rated_cost_usd":          rated,
		"provider_cost_sum":       provider,
		"sources_with_rated_cost": ratedRows,
		"first_event_at":          nullTime(minAt),
		"last_event_at":           nullTime(maxAt),
	})
}

func (a *API) ByVendor(w http.ResponseWriter, r *http.Request) {
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
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
		WHERE ($1 = '' OR host_id = $1)
		  AND ($2 = '' OR COALESCE(project, '(unmapped)') = $2)
		GROUP BY vendor
		ORDER BY COALESCE(sum(rated_cost_usd), 0) DESC, count(*) DESC
	`, machine, project)
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
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT COALESCE(NULLIF(model, ''), '(unknown)') AS model,
			COALESCE(vendor, '') AS vendor,
			count(*)::bigint,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(rated_cost_usd), 0)
		FROM v_burn_usage_rated
		WHERE ($1 = '' OR host_id = $1)
		  AND ($2 = '' OR COALESCE(project, '(unmapped)') = $2)
		GROUP BY 1, 2
		ORDER BY COALESCE(sum(rated_cost_usd), 0) DESC, count(*) DESC
		LIMIT $3
	`, machine, project, limit)
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
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT to_char((COALESCE(last_event_at, started_at, ingested_at) AT TIME ZONE 'UTC') + make_interval(hours => $4), 'YYYY-MM-DD') AS day,
			COALESCE(sum(tokens_input), 0),
			COALESCE(sum(tokens_output), 0),
			COALESCE(sum(rated_cost_usd), 0),
			count(*)::bigint
		FROM (
			SELECT last_event_at, started_at, ingested_at,
				tokens_input, tokens_output, rated_cost_usd
			FROM v_burn_usage_rated
			WHERE ($1 = '' OR host_id = $1)
			  AND ($3 = '' OR COALESCE(project, '(unmapped)') = $3)
			  AND COALESCE(last_event_at, started_at, ingested_at) >= (CURRENT_TIMESTAMP - make_interval(days => $2))
		) t
		GROUP BY 1
		ORDER BY 1
	`, machine, days, project, a.dayOffsetHours())
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
	machine := r.URL.Query().Get("machine")
	project := r.URL.Query().Get("project")
	day := r.URL.Query().Get("day")

	q := `
		SELECT source_id, host_id, vendor, COALESCE(model, ''), source_path,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_cache_write_5m, tokens_cache_write_1h,
			rated_cost_usd, provider_cost, last_event_at, started_at, ingested_at, COALESCE(project, '(unmapped)')
		FROM v_burn_usage_rated
		WHERE ($1 = '' OR host_id = $1)
		  AND ($2 = '' OR vendor = $2)
		  AND ($5 = '' OR COALESCE(project, '(unmapped)') = $5)
		  AND ($6 = '' OR to_char((COALESCE(last_event_at, started_at, ingested_at) AT TIME ZONE 'UTC') + make_interval(hours => $7), 'YYYY-MM-DD') = $6)
		ORDER BY COALESCE(last_event_at, started_at, ingested_at) DESC NULLS LAST
		LIMIT $3 OFFSET $4
	`

	rows, err := a.DB.QueryContext(r.Context(), q, machine, vendor, limit, offset, project, day, a.dayOffsetHours())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id, machine, vendor, model, path, proj string
			in, outTok, cr, cw               sql.NullInt64
			cw5, cw1h                        sql.NullInt64
			rated, pcost                     sql.NullFloat64
			lastAt, startAt                  sql.NullTime
			ingestedAt                       time.Time
		)
		if err := rows.Scan(&id, &machine, &vendor, &model, &path, &in, &outTok, &cr, &cw, &cw5, &cw1h, &rated, &pcost, &lastAt, &startAt, &ingestedAt, &proj); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"source_id": id, "machine": machine, "vendor": vendor, "model": model, "source_path": path, "project": proj,
			"tokens_input": nullInt(in), "tokens_output": nullInt(outTok),
			"tokens_cache_read": nullInt(cr), "tokens_cache_write": nullInt(cw),
			"tokens_cache_write_5m": nullInt(cw5), "tokens_cache_write_1h": nullInt(cw1h),
			"rated_cost_usd": nullFloat(rated), "provider_cost": nullFloat(pcost),
			"last_event_at": nullTime(lastAt), "started_at": nullTime(startAt), "ingested_at": ingestedAt,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) Machines(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT host_id, count(*)::bigint, max(COALESCE(ingested_at, last_event_at, started_at))
		FROM v_burn_usage_rated
		GROUP BY host_id
		ORDER BY host_id
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var machine string
		var sources int64
		var lastSync sql.NullTime
		if err := rows.Scan(&machine, &sources, &lastSync); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"machine": machine, 
			"sources": sources,
			"last_sync_at": nullTime(lastSync),
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

func nullString(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
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

func (a *API) Cwds(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT bs.host_id, bs.cwd, p.project, count(distinct bs.source_id)::bigint AS snapshots
		FROM burn_snapshots bs
		LEFT JOIN project_cwds p ON p.host_id = bs.host_id AND p.cwd = bs.cwd
		WHERE bs.cwd IS NOT NULL AND bs.cwd <> ''
		GROUP BY bs.host_id, bs.cwd, p.project
		ORDER BY bs.cwd
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var hostID, cwd string
		var project sql.NullString
		var snapshots int64

		if err := rows.Scan(&hostID, &cwd, &project, &snapshots); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{
			"host_id":   hostID,
			"cwd":       cwd,
			"project":   nullString(project),
			"snapshots": snapshots,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) MapCwd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeErr(w, 405, "method not allowed")
		return
	}
	var req struct {
		HostID  string `json:"host_id"`
		Cwd     string `json:"cwd"`
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.Cwd == "" {
		writeErr(w, 400, "cwd is required")
		return
	}
	if req.HostID == "" {
		writeErr(w, 400, "host_id is required")
		return
	}

	if req.Project == "" {
		_, err := a.DB.ExecContext(r.Context(), "DELETE FROM project_cwds WHERE host_id = $1 AND cwd = $2", req.HostID, req.Cwd)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		_, err := a.DB.ExecContext(r.Context(), `
			INSERT INTO project_cwds (host_id, cwd, project) VALUES ($1, $2, $3)
			ON CONFLICT (host_id, cwd) DO UPDATE SET project = EXCLUDED.project
		`, req.HostID, req.Cwd, req.Project)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
