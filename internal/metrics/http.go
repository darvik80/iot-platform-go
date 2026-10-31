package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Middleware records request count, latency and in-flight requests. It must be
// registered outside chi's Recoverer so panics are recorded as 500 responses.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)

		// The route pattern (e.g. /api/v1/devices/{id}) keeps cardinality bounded.
		// Unmatched requests bypass router middleware entirely; the fallback only
		// covers handlers mounted outside the router tree.
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()

		// An upgraded WebSocket stays open for the whole session — its lifetime
		// says nothing about request latency.
		if status != http.StatusSwitchingProtocols {
			httpDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		}
	})
}
