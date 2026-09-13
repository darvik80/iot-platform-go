// Package plugins — metrics-dashboard catalog plugin.
//
// This plugin does NOT start an HTTP server or run per-device automation.
// It supplies a panel configuration (default dashboard template) that the
// device dashboard API reads to aggregate telemetry, RPC, events and
// online-timeline data for the frontend Metrics+ tab.
package plugins

import (
	"encoding/json"
)

const PluginMetricsDashboard = "metrics-dashboard"

func init() {
	Register(Descriptor{
		ID:          PluginMetricsDashboard,
		Name:        "Metrics Dashboard",
		Description: "Per-device dashboard with telemetry charts, gauges, RPC history and online timeline",
		DefaultConfig: json.RawMessage(`{
			"panels": [
				{"id":"p1","type":"telemetry_chart","title":"WiFi RSSI","keys":["wifi_rssi"],"window":"1h"},
				{"id":"p2","type":"telemetry_gauge","title":"Free Heap","key":"free_heap","window":"1h"},
				{"id":"p3","type":"online_timeline","title":"Online Timeline","window":"24h"}
			]
		}`),
	}, func() Instance { return &MetricsDashboard{} })
}

// ── Panel configuration types ─────────────────────────────────────────────

// PanelConfig describes one dashboard panel. The API reads these from the
// binding settings and uses them to query the database.
type PanelConfig struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Keys   []string `json:"keys,omitempty"`   // telemetry_chart
	Key    string   `json:"key,omitempty"`    // telemetry_gauge
	Window string   `json:"window,omitempty"` // 1h, 6h, 24h, 7d
	Limit  int      `json:"limit,omitempty"`  // rpc_table
	Severity string `json:"severity,omitempty"` // events_count
}

type dashboardConfig struct {
	Panels []PanelConfig `json:"panels"`
}

// MetricsDashboard is a passive plugin — it stores config but does not
// perform any per-device automation.
type MetricsDashboard struct {
	cfg dashboardConfig
}

func (d *MetricsDashboard) ApplyConfig(raw json.RawMessage) error {
	cfg := dashboardConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	d.cfg = cfg
	return nil
}

func (d *MetricsDashboard) IntervalMS() int { return 0 }
func (d *MetricsDashboard) Auto() bool      { return false }
func (d *MetricsDashboard) OnEvent(_ Runtime, _ Event) error {
	return nil // passive — data aggregation happens in the API layer
}

// ── Helpers used by the dashboard API ─────────────────────────────────────

// DefaultPanels returns the default panel configuration when no binding exists.
func DefaultDashboardPanels() []PanelConfig {
	desc, _, ok := Lookup(PluginMetricsDashboard)
	if !ok {
		return nil
	}
	cfg := dashboardConfig{}
	_ = json.Unmarshal(desc.DefaultConfig, &cfg)
	return cfg.Panels
}

// ParseWindowMS converts a window string like "1h", "24h", "7d" to milliseconds.
func ParseWindowMS(w string) int64 {
	if w == "" {
		return 3600_000 // default 1h
	}
	var mult int64 = 3600_000
	s := w
	if len(w) > 0 {
		switch w[len(w)-1] {
		case 'h':
			mult = 3600_000
			s = w[:len(w)-1]
		case 'd':
			mult = 86400_000
			s = w[:len(w)-1]
		case 'm':
			mult = 60_000
			s = w[:len(w)-1]
		}
	}
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	if n <= 0 {
		n = 1
	}
	return n * mult
}
