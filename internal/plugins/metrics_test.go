package plugins

import (
	"testing"
)

func TestMetricsStore_IncrCounter(t *testing.T) {
	s := &metricsStore{
		counters: make(map[string]*uint64),
		gauges:   make(map[string]*int64),
	}

	// Increment new counter
	s.IncrCounter("test_counter")
	counters, _ := s.Snapshot()
	if counters["test_counter"] != 1 {
		t.Errorf("expected 1, got %d", counters["test_counter"])
	}

	// Increment same counter
	s.IncrCounter("test_counter")
	s.IncrCounter("test_counter")
	counters, _ = s.Snapshot()
	if counters["test_counter"] != 3 {
		t.Errorf("expected 3, got %d", counters["test_counter"])
	}

	// Different counter
	s.IncrCounter("other_counter")
	counters, _ = s.Snapshot()
	if counters["other_counter"] != 1 {
		t.Errorf("expected 1, got %d", counters["other_counter"])
	}
}

func TestMetricsStore_SetGauge(t *testing.T) {
	s := &metricsStore{
		counters: make(map[string]*uint64),
		gauges:   make(map[string]*int64),
	}

	// Set gauge to 1
	s.SetGauge("online", 1)
	_, gauges := s.Snapshot()
	if gauges["online"] != 1 {
		t.Errorf("expected 1, got %d", gauges["online"])
	}

	// Set gauge to 0
	s.SetGauge("online", 0)
	_, gauges = s.Snapshot()
	if gauges["online"] != 0 {
		t.Errorf("expected 0, got %d", gauges["online"])
	}
}

func TestMetricsStore_SetGaugeValue(t *testing.T) {
	s := &metricsStore{
		counters: make(map[string]*uint64),
		gauges:   make(map[string]*int64),
	}

	// Set arbitrary value
	s.SetGaugeValue("temperature", 42)
	_, gauges := s.Snapshot()
	if gauges["temperature"] != 42 {
		t.Errorf("expected 42, got %d", gauges["temperature"])
	}

	// Update value
	s.SetGaugeValue("temperature", 25)
	_, gauges = s.Snapshot()
	if gauges["temperature"] != 25 {
		t.Errorf("expected 25, got %d", gauges["temperature"])
	}
}

func TestMetricsStore_Snapshot(t *testing.T) {
	s := &metricsStore{
		counters: make(map[string]*uint64),
		gauges:   make(map[string]*int64),
	}

	// Empty snapshot
	counters, gauges := s.Snapshot()
	if len(counters) != 0 || len(gauges) != 0 {
		t.Error("empty snapshot should have no entries")
	}

	// Add some metrics
	s.IncrCounter("c1")
	s.IncrCounter("c2")
	s.SetGauge("g1", 1)

	counters, gauges = s.Snapshot()
	if len(counters) != 2 {
		t.Errorf("expected 2 counters, got %d", len(counters))
	}
	if len(gauges) != 1 {
		t.Errorf("expected 1 gauge, got %d", len(gauges))
	}

	// Snapshot should be a copy (modifying original shouldn't affect snapshot)
	s.IncrCounter("c1")
	if counters["c1"] != 1 {
		t.Error("snapshot should be independent of original")
	}
}

func TestParseMetricKey(t *testing.T) {
	tests := []struct {
		input      string
		wantFamily string
		wantLabels string
	}{
		{"cogitor_devices_total{account=acme}", "cogitor_devices_total", "account=acme"},
		{"cogitor_telemetry_total{account=acme,product=sensor}", "cogitor_telemetry_total", "account=acme,product=sensor"},
		{"simple_metric", "simple_metric", ""},
		{"metric{}", "metric", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			family, labels := parseMetricKey(tt.input)
			if family != tt.wantFamily {
				t.Errorf("family = %q, want %q", family, tt.wantFamily)
			}
			if labels != tt.wantLabels {
				t.Errorf("labels = %q, want %q", labels, tt.wantLabels)
			}
		})
	}
}

func TestGroupMetrics(t *testing.T) {
	m := map[string]uint64{
		"cogitor_devices_total{account=acme}":   5,
		"cogitor_devices_total{account=beta}":   3,
		"cogitor_telemetry_total{account=acme}": 100,
	}

	families := groupMetrics(m)
	if len(families) != 2 {
		t.Errorf("expected 2 families, got %d", len(families))
	}
	if len(families["cogitor_devices_total"]) != 2 {
		t.Errorf("expected 2 entries for devices_total, got %d", len(families["cogitor_devices_total"]))
	}
	if len(families["cogitor_telemetry_total"]) != 1 {
		t.Errorf("expected 1 entry for telemetry_total, got %d", len(families["cogitor_telemetry_total"]))
	}
}

func TestGroupGaugeMetrics(t *testing.T) {
	m := map[string]int64{
		"cogitor_online_devices{account=acme}": 10,
		"cogitor_online_devices{account=beta}": 5,
	}

	families := groupGaugeMetrics(m)
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}
	if len(families["cogitor_online_devices"]) != 2 {
		t.Errorf("expected 2 entries, got %d", len(families["cogitor_online_devices"]))
	}
}

func TestRenderMetrics(t *testing.T) {
	// Reset global metrics for clean test
	Metrics = &metricsStore{
		counters: make(map[string]*uint64),
		gauges:   make(map[string]*int64),
	}

	// Empty metrics
	output := RenderMetrics()
	if output != "\n" {
		t.Errorf("empty metrics should produce just newline, got %q", output)
	}

	// Add some metrics
	Metrics.IncrCounter("test_counter{label=value}")
	Metrics.SetGauge("test_gauge{label=value}", 1)

	output = RenderMetrics()
	if output == "" {
		t.Error("output should not be empty")
	}
	// Should contain TYPE declarations
	if !containsStr(output, "# TYPE") {
		t.Error("output should contain TYPE declarations")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
