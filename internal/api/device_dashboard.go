package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/plugins"
)

// deviceDashboard returns aggregated panel data for the Metrics+ tab.
// GET /devices/{id}/dashboard
func (s *Server) deviceDashboard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}

	// Resolve panel config: try device binding settings, fall back to default
	panels := s.resolvePanels(d.ID)

	now := db.NowMS()
	result := make([]panelResult, 0, len(panels))
	for _, p := range panels {
		pr := panelResult{
			ID:    p.ID,
			Type:  p.Type,
			Title: p.Title,
		}
		windowMS := plugins.ParseWindowMS(p.Window)
		from := now - windowMS

		switch p.Type {
		case "telemetry_chart":
			pr.Data = s.telemetryChartData(d.ID, p.Keys, from, now)
		case "telemetry_gauge":
			keys := p.Keys
			if len(keys) == 0 && p.Key != "" {
				keys = []string{p.Key}
			}
			pr.Data = s.telemetryGaugeData(d.ID, keys, from, now)
		case "rpc_table":
			limit := p.Limit
			if limit <= 0 {
				limit = 20
			}
			pr.Data = s.rpcTableData(d.ID, limit)
		case "online_timeline":
			pr.Data = s.onlineTimelineData(d.ID, from, now)
		case "events_count":
			pr.Data = s.eventsCountData(d.ID, p.Severity, from, now)
		default:
			pr.Data = map[string]any{"error": "unknown panel type"}
		}
		result = append(result, pr)
	}

	writeJSON(w, http.StatusOK, map[string]any{"panels": result})
}

// ── Panel resolution ──────────────────────────────────────────────────────

func (s *Server) resolvePanels(devicePK int64) []plugins.PanelConfig {
	// Try to read binding settings for metrics-dashboard plugin
	var settingsJSON sql.NullString
	err := s.Plat.DB.QueryRow(
		`SELECT settings_json FROM device_plugin_bindings WHERE device_pk=? AND plugin_id=? LIMIT 1`,
		devicePK, plugins.PluginMetricsDashboard).Scan(&settingsJSON)
	if err == nil && settingsJSON.Valid && settingsJSON.String != "" && settingsJSON.String != "null" {
		var cfg struct {
			Panels []plugins.PanelConfig `json:"panels"`
		}
		if json.Unmarshal([]byte(settingsJSON.String), &cfg) == nil && len(cfg.Panels) > 0 {
			return cfg.Panels
		}
	}
	return plugins.DefaultDashboardPanels()
}

// ── Panel data aggregators ────────────────────────────────────────────────

type panelResult struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Data  any    `json:"data"`
}

type telemetryPoint struct {
	Key    string      `json:"key"`
	Points [][]any     `json:"points"` // [[ts, value], ...]
}

func (s *Server) telemetryChartData(devicePK int64, keys []string, from, to int64) any {
	if len(keys) == 0 {
		return []telemetryPoint{}
	}
	samples, err := s.Plat.QueryTelemetry(devicePK, keys, from, to, 10000)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	// Group by key
	grouped := map[string][][]any{}
	keyOrder := map[string]int{}
	idx := 0
	for _, s := range samples {
		if _, ok := keyOrder[s.Key]; !ok {
			keyOrder[s.Key] = idx
			idx++
		}
		var val any
		if s.ValueNum != nil {
			val = *s.ValueNum
		} else if s.ValueBool != nil {
			if *s.ValueBool {
				val = 1
			} else {
				val = 0
			}
		} else if s.ValueText != nil {
			val = *s.ValueText
		}
		grouped[s.Key] = append(grouped[s.Key], []any{s.TS, val})
	}
	result := make([]telemetryPoint, 0, len(grouped))
	for _, k := range keys {
		if pts, ok := grouped[k]; ok {
			result = append(result, telemetryPoint{Key: k, Points: pts})
		}
	}
	return result
}

type gaugeData struct {
	Key     string  `json:"key"`
	Current float64 `json:"current"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Avg     float64 `json:"avg"`
}

func (s *Server) telemetryGaugeData(devicePK int64, keys []string, from, to int64) any {
	if len(keys) == 0 {
		return []gaugeData{}
	}
	samples, err := s.Plat.QueryTelemetry(devicePK, keys, from, to, 10000)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	type acc struct {
		sum   float64
		count int
		min   float64
		max   float64
		last  float64
		lastTS int64
	}
	grouped := map[string]*acc{}
	for _, s := range samples {
		if s.ValueNum == nil {
			continue
		}
		v := *s.ValueNum
		a, ok := grouped[s.Key]
		if !ok {
			a = &acc{min: v, max: v}
			grouped[s.Key] = a
		}
		a.sum += v
		a.count++
		if v < a.min {
			a.min = v
		}
		if v > a.max {
			a.max = v
		}
		if s.TS >= a.lastTS {
			a.last = v
			a.lastTS = s.TS
		}
	}
	result := make([]gaugeData, 0, len(grouped))
	for _, k := range keys {
		if a, ok := grouped[k]; ok && a.count > 0 {
			result = append(result, gaugeData{
				Key:     k,
				Current: a.last,
				Min:     a.min,
				Max:     a.max,
				Avg:     a.sum / float64(a.count),
			})
		}
	}
	return result
}

func (s *Server) rpcTableData(devicePK int64, limit int) any {
	list, err := s.Plat.ListRPC(devicePK, limit)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return list
}

type timelineSegment struct {
	Start  int64 `json:"start"`
	End    int64 `json:"end"`
	Online bool  `json:"online"`
}

func (s *Server) onlineTimelineData(devicePK int64, from, to int64) any {
	// Query status events to build online/offline timeline
	rows, err := s.Plat.DB.Query(
		`SELECT type, ts, data_json FROM events WHERE device_pk=? AND ts>=? AND ts<=? AND type='status' ORDER BY ts ASC`,
		devicePK, from, to)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer rows.Close()

	var segments []timelineSegment
	var lastOnline bool
	var lastTS int64 = from

	for rows.Next() {
		var typ string
		var ts int64
		var detail sql.NullString
		if err := rows.Scan(&typ, &ts, &detail); err != nil {
			continue
		}
		// Determine online status from event data_json
		online := false
		if detail.Valid {
			var d map[string]any
			if json.Unmarshal([]byte(detail.String), &d) == nil {
				if v, ok := d["online"].(bool); ok {
					online = v
				}
			}
		}
		if ts > lastTS {
			segments = append(segments, timelineSegment{Start: lastTS, End: ts, Online: lastOnline})
		}
		lastOnline = online
		lastTS = ts
	}
	// Final segment to now
	if to > lastTS {
		segments = append(segments, timelineSegment{Start: lastTS, End: to, Online: lastOnline})
	}
	if segments == nil {
		segments = []timelineSegment{}
	}
	return segments
}

type eventsCountData struct {
	Critical int `json:"critical"`
	Error    int `json:"error"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

func (s *Server) eventsCountData(devicePK int64, severity string, from, to int64) any {
	query := `SELECT severity, COUNT(*) FROM events WHERE device_pk=? AND ts>=? AND ts<=?`
	args := []any{devicePK, from, to}
	if severity != "" {
		query += ` AND severity=?`
		args = append(args, severity)
	}
	query += ` GROUP BY severity`

	rows, err := s.Plat.DB.Query(query, args...)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer rows.Close()

	result := eventsCountData{}
	for rows.Next() {
		var sev string
		var count int
		if err := rows.Scan(&sev, &count); err != nil {
			continue
		}
		switch sev {
		case "critical":
			result.Critical = count
		case "error":
			result.Error = count
		case "warning":
			result.Warning = count
		case "info":
			result.Info = count
		}
	}
	return result
}
