// Package metrics owns the platform's Prometheus registry: Go/process
// collectors, database pool stats, HTTP middleware and business counters.
//
// The registry is served on a dedicated local listener (METRICS_ADDR,
// default :9102) so scraping does not depend on catalog plugin bindings or
// the public API server.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is private so third-party libraries cannot add metrics the
// platform does not own.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_http_requests_total",
		Help: "HTTP requests served by the platform API.",
	}, []string{"route", "method", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "platform_http_request_duration_seconds",
		Help:    "HTTP request latency by route and method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})

	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "platform_http_in_flight_requests",
		Help: "HTTP requests currently being served.",
	})

	telemetrySamples = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_telemetry_samples_total",
		Help: "Telemetry samples stored, by account and product.",
	}, []string{"account", "product"})

	telemetryErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_telemetry_errors_total",
		Help: "Telemetry messages rejected or failed to store, by account and product.",
	}, []string{"account", "product"})

	rpcCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_rpc_calls_total",
		Help: "Device RPC calls completed, by account, product, method and status.",
	}, []string{"account", "product", "method", "status"})

	eventBusPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_event_bus_published_total",
		Help: "Rules-bus messages published, by account and kind.",
	}, []string{"account", "kind"})

	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "platform_build_info",
		Help: "Build information; always 1.",
	}, []string{"version", "commit"})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
		httpInFlight,
		telemetrySamples,
		telemetryErrors,
		rpcCalls,
		eventBusPublished,
		buildInfo,
	)
}

// Handler serves the registry in the Prometheus text exposition format.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

// SetBuildInfo labels platform_build_info with the injected build metadata.
func SetBuildInfo(version, commit string) {
	buildInfo.WithLabelValues(version, commit).Set(1)
}

// TelemetrySamples counts stored telemetry samples.
func TelemetrySamples(account, product string, n int) {
	if n <= 0 {
		return
	}
	telemetrySamples.WithLabelValues(account, product).Add(float64(n))
}

// TelemetryError counts telemetry messages rejected before storage.
func TelemetryError(account, product string) {
	telemetryErrors.WithLabelValues(account, product).Inc()
}

// RPCCall counts a completed RPC call with its terminal status.
func RPCCall(account, product, method, status string) {
	rpcCalls.WithLabelValues(account, product, method, status).Inc()
}

// EventBusPublished counts rules-bus publishes.
func EventBusPublished(account, kind string) {
	eventBusPublished.WithLabelValues(account, kind).Inc()
}
