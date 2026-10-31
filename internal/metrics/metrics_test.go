package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMiddlewareRecordsBoundedRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/api/v1/things/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/things/42", nil))

	got := testutil.ToFloat64(httpRequests.WithLabelValues("/api/v1/things/{id}", "GET", "418"))
	if got != 1 {
		t.Fatalf("route counter = %v, want 1", got)
	}
}

func TestMiddlewareIgnoresUnmatchedRequests(t *testing.T) {
	before := testutil.CollectAndCount(httpRequests)
	r := chi.NewRouter()
	r.Use(Middleware)

	// chi routes 404s to NotFoundHandler without running router middleware,
	// so a scan of random paths cannot create new series.
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/no/such/route", nil))

	if after := testutil.CollectAndCount(httpRequests); after != before {
		t.Fatalf("unmatched request added series: %d -> %d", before, after)
	}
}

func TestBusinessCounters(t *testing.T) {
	TelemetrySamples("acme-test", "sensor", 3)
	TelemetrySamples("acme-test", "sensor", 2)
	if got := testutil.ToFloat64(telemetrySamples.WithLabelValues("acme-test", "sensor")); got != 5 {
		t.Fatalf("telemetry samples = %v, want 5", got)
	}

	TelemetryError("acme-test", "sensor")
	if got := testutil.ToFloat64(telemetryErrors.WithLabelValues("acme-test", "sensor")); got != 1 {
		t.Fatalf("telemetry errors = %v, want 1", got)
	}

	RPCCall("acme-test", "sensor", "reboot", "timeout")
	if got := testutil.ToFloat64(rpcCalls.WithLabelValues("acme-test", "sensor", "reboot", "timeout")); got != 1 {
		t.Fatalf("rpc calls = %v, want 1", got)
	}

	EventBusPublished("acme-test", "telemetry")
	if got := testutil.ToFloat64(eventBusPublished.WithLabelValues("acme-test", "telemetry")); got != 1 {
		t.Fatalf("event bus publishes = %v, want 1", got)
	}
}

func TestRegistryHasRuntimeCollectors(t *testing.T) {
	SetBuildInfo("test-version", "test-commit")
	if got := testutil.ToFloat64(buildInfo.WithLabelValues("test-version", "test-commit")); got != 1 {
		t.Fatalf("build info = %v, want 1", got)
	}

	n, err := testutil.GatherAndCount(Registry, "go_goroutines", "process_start_time_seconds")
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if n < 2 {
		t.Fatalf("runtime collectors = %d metrics, want at least 2", n)
	}
}
