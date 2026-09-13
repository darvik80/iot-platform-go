// Package plugins — prometheus-exporter catalog plugin.
//
// This plugin starts an HTTP server that exposes Prometheus text-format metrics.
// Counters are incremented in-process via the MetricsStore; gauges are kept in
// memory and periodically synced from the database.
//
// The HTTP server lifecycle is tied to binding activation: it starts when at
// least one binding is active and shuts down when all are disabled.
package plugins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const PluginPrometheusExporter = "prometheus-exporter"

func init() {
	Register(Descriptor{
		ID:          PluginPrometheusExporter,
		Name:        "Prometheus Exporter",
		Description: "Expose device/telemetry/RPC metrics in Prometheus text format for external scraping",
		DefaultConfig: json.RawMessage(`{
			"port": 9100,
			"bind": "0.0.0.0",
			"path": "/metrics",
			"bearer_token": "",
			"update_interval_ms": 5000
		}`),
		Effects: []string{"http_server"},
	}, func() Instance { return &PrometheusExporter{} })
}

// ── Global metrics store (shared across all plugin instances) ─────────────

// MetricsStore holds atomic counters and in-memory gauges that are incremented
// by service-layer hooks and served by the exporter HTTP handler.
var Metrics = &metricsStore{
	counters: make(map[string]*uint64),
	gauges:   make(map[string]*int64),
}

type metricsStore struct {
	mu       sync.Mutex
	counters map[string]*uint64 // key -> *counter
	gauges   map[string]*int64  // key -> *gauge (value * 1 or 0)
}

// IncrCounter atomically increments a named counter.
func (s *metricsStore) IncrCounter(name string) {
	s.mu.Lock()
	c, ok := s.counters[name]
	if !ok {
		var v uint64
		c = &v
		s.counters[name] = c
	}
	s.mu.Unlock()
	atomic.AddUint64(c, 1)
}

// SetGauge atomically sets a named gauge to 0 or 1.
func (s *metricsStore) SetGauge(name string, val int64) {
	s.mu.Lock()
	g, ok := s.gauges[name]
	if !ok {
		var v int64
		g = &v
		s.gauges[name] = g
	}
	s.mu.Unlock()
	atomic.StoreInt64(g, val)
}

// SetGaugeValue sets a gauge to an arbitrary integer value.
func (s *metricsStore) SetGaugeValue(name string, val int64) {
	s.mu.Lock()
	g, ok := s.gauges[name]
	if !ok {
		var v int64
		g = &v
		s.gauges[name] = g
	}
	s.mu.Unlock()
	atomic.StoreInt64(g, val)
}

// Snapshot returns a copy of all current metric values.
func (s *metricsStore) Snapshot() (counters map[string]uint64, gauges map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counters = make(map[string]uint64, len(s.counters))
	for k, v := range s.counters {
		counters[k] = atomic.LoadUint64(v)
	}
	gauges = make(map[string]int64, len(s.gauges))
	for k, v := range s.gauges {
		gauges[k] = atomic.LoadInt64(v)
	}
	return
}

// ── PrometheusExporter plugin instance ────────────────────────────────────

type exporterConfig struct {
	Port             int      `json:"port"`
	Bind             string   `json:"bind"`
	Path             string   `json:"path"`
	BearerToken      string   `json:"bearer_token"`
	Metrics          []string `json:"metrics"`
	UpdateIntervalMS int      `json:"update_interval_ms"`
}

type PrometheusExporter struct {
	mu     sync.Mutex
	cfg    exporterConfig
	server *http.Server
}

func (e *PrometheusExporter) ApplyConfig(raw json.RawMessage) error {
	cfg := exporterConfig{
		Port:             9100,
		Bind:             "0.0.0.0",
		Path:             "/metrics",
		UpdateIntervalMS: 5000,
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = 9100
	}
	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}
	if cfg.Path == "" {
		cfg.Path = "/metrics"
	}
	if cfg.UpdateIntervalMS < 1000 {
		cfg.UpdateIntervalMS = 5000
	}
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
	return nil
}

func (e *PrometheusExporter) IntervalMS() int { return 0 }
func (e *PrometheusExporter) Auto() bool      { return false }

func (e *PrometheusExporter) OnEvent(_ Runtime, ev Event) error {
	if ev.Kind != EventApply {
		return nil
	}
	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()

	// Start HTTP server if not already running
	if e.server != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		if cfg.BearerToken != "" {
			tok := r.Header.Get("Authorization")
			tok = strings.TrimPrefix(tok, "Bearer ")
			tok = strings.TrimSpace(tok)
			if tok != cfg.BearerToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(renderMetrics()))
	})

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	e.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Server error — logged by the hub
		}
	}()
	return nil
}

// Stop shuts down the HTTP server. Called by the hub when the binding is removed.
func (e *PrometheusExporter) Stop() {
	e.mu.Lock()
	srv := e.server
	e.server = nil
	e.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

// ── Prometheus text rendering ─────────────────────────────────────────────

// RenderMetrics returns all metrics in Prometheus text exposition format.
func RenderMetrics() string {
	return renderMetrics()
}

func renderMetrics() string {
	counters, gauges := Metrics.Snapshot()

	// Collect and sort keys for deterministic output
	var lines []string

	// Group counters by metric name family
	counterFamilies := groupMetrics(counters)
	for family, entries := range counterFamilies {
		lines = append(lines, fmt.Sprintf("# TYPE %s counter", family))
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf("%s{%s} %d", family, entry.labels, entry.value))
		}
	}

	gaugeFamilies := groupGaugeMetrics(gauges)
	for family, entries := range gaugeFamilies {
		lines = append(lines, fmt.Sprintf("# TYPE %s gauge", family))
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf("%s{%s} %d", family, entry.labels, entry.value))
		}
	}

	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

type metricEntry struct {
	labels string
	value  uint64
}

type gaugeEntry struct {
	labels string
	value  int64
}

func groupMetrics(m map[string]uint64) map[string][]metricEntry {
	families := make(map[string][]metricEntry)
	for key, val := range m {
		family, labels := parseMetricKey(key)
		families[family] = append(families[family], metricEntry{labels: labels, value: val})
	}
	return families
}

func groupGaugeMetrics(m map[string]int64) map[string][]gaugeEntry {
	families := make(map[string][]gaugeEntry)
	for key, val := range m {
		family, labels := parseMetricKey(key)
		families[family] = append(families[family], gaugeEntry{labels: labels, value: val})
	}
	return families
}

// parseMetricKey splits "cogitor_devices_total{account=acme,product=sensor}" into
// family="cogitor_devices_total" and labels="account=acme,product=sensor".
func parseMetricKey(key string) (family, labels string) {
	idx := strings.Index(key, "{")
	if idx < 0 {
		return key, ""
	}
	family = key[:idx]
	labels = strings.Trim(key[idx+1:], "}")
	return
}
