package builtin

import (
	"github.com/orislabsdev/gocore/handler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus returns a handler that exposes the Prometheus /metrics endpoint.
// It wraps the standard promhttp.Handler() so it integrates cleanly with gocore.
func Prometheus() handler.HandlerFunc {
	h := promhttp.Handler()
	return func(ctx *handler.Context) {
		h.ServeHTTP(ctx.ResponseWriter(), ctx.Request)
	}
}
