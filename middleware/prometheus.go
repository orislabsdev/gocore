package middleware

import (
	"strconv"
	"time"

	"github.com/orislabsdev/gocore/handler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gocore_http_requests_total",
			Help: "Total number of HTTP requests processed by gocore.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gocore_http_request_duration_seconds",
			Help:    "Histogram of HTTP request latencies in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// Prometheus returns an observability middleware that records request counts
// and latencies into standard Prometheus metrics.
//
// To avoid high cardinality, it records the matched parameterised route pattern
// (e.g. "/users/:id") rather than the exact request path ("/users/123").
// Provide paths to skipMetrics (e.g. "/health", "/metrics") to prevent logging
// highly active observability endpoints.
func Prometheus(skipPaths ...string) handler.MiddlewareFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			path := ctx.Request.URL.Path
			if _, ignored := skip[path]; ignored {
				next(ctx)
				return
			}

			start := time.Now()

			next(ctx)

			duration := time.Since(start)
			statusStr := strconv.Itoa(ctx.Status())

			// Extract matched pattern to avoid high cardinality map explosion.
			// Fall back to the raw path only if it wasn't matched (e.g., 404s).
			pattern := path
			if p, ok := ctx.Get("route_pattern"); ok {
				if s, ok := p.(string); ok && s != "" {
					pattern = s
				}
			}

			httpRequestsTotal.WithLabelValues(ctx.Request.Method, pattern, statusStr).Inc()
			httpRequestDuration.WithLabelValues(ctx.Request.Method, pattern).Observe(duration.Seconds())
		}
	}
}
