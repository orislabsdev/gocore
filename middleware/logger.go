package middleware

import (
	"net/http"
	"time"

	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/logger"
)

// LoggerConfig holds options for the Logger middleware.
type LoggerConfig struct {
	// Log is the logger that will receive access-log entries.
	// Required — if nil, the middleware is a no-op.
	Log *logger.Logger

	// SkipPaths is a set of URL paths that should not be logged.
	// Useful for high-frequency health-check or metrics endpoints
	// that would otherwise flood the log stream.
	SkipPaths map[string]struct{}

	// SlowThreshold is the minimum request duration that triggers a "slow
	// request" warning in addition to the normal access-log entry.
	// Zero (default) disables slow-request warnings.
	SlowThreshold time.Duration
}

// Logger returns an access-log middleware that emits one structured log entry
// per request. The entry includes the method, path, status code, latency, and
// response size. An X-Request-ID is included when present.
//
// Pair this with the RequestID middleware for end-to-end request tracing.
//
//	app.Use(middleware.Logger(middleware.LoggerConfig{
//	    Log:       log,
//	    SkipPaths: map[string]struct{}{"/health": {}},
//	}))
func Logger(cfg LoggerConfig) handler.MiddlewareFunc {
	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			// Skip logging for configured paths.
			if _, skip := cfg.SkipPaths[ctx.Request.URL.Path]; skip || cfg.Log == nil {
				next(ctx)
				return
			}

			start := time.Now()
			path := ctx.Request.URL.RequestURI() // includes query string

			// Call the next handler/middleware.
			next(ctx)

			// Compute latency after the handler has completed.
			latency := time.Since(start)
			status := ctx.Status()

			var size int64
			if rw, ok := ctx.ResponseWriter().(responseWriterSize); ok {
				size = rw.Size()
			}

			// Select the log level based on the HTTP status code.
			kv := []any{
				"method", ctx.Request.Method,
				"path", path,
				"status", status,
				"latency_ms", latency.Milliseconds(),
				"bytes", size,
				"ip", ctx.ClientIP(),
			}
			if ua := ctx.Request.UserAgent(); ua != "" {
				kv = append(kv, "user_agent", ua)
			}

			// Store a contextual logger in the request-scoped keys so that handlers
			// can access it via ctx.Logger(). Automatically enrich it with the
			// request ID if present.
			l := cfg.Log
			if reqID := ctx.Header("X-Request-ID"); reqID != "" {
				l = l.With("request_id", reqID)
			}
			ctx.Set("logger", l)

			switch {
			case status >= http.StatusInternalServerError:
				cfg.Log.Error("request", kv...)
			case status >= http.StatusBadRequest:
				cfg.Log.Warn("request", kv...)
			default:
				cfg.Log.Info("request", kv...)
			}

			// Emit an additional warning for slow requests.
			if cfg.SlowThreshold > 0 && latency > cfg.SlowThreshold {
				cfg.Log.Warn("slow request",
					"path", path,
					"latency_ms", latency.Milliseconds(),
					"threshold_ms", cfg.SlowThreshold.Milliseconds(),
				)
			}
		}
	}
}

// responseWriterSize is a private alias used by the Logger to read the byte
// count. The Context exposes the responseWriter via ResponseWriter(); we
// perform a type assertion here. If the assertion fails (unexpected wrapper)
// we simply report 0 bytes.
type responseWriterSize interface {
	http.ResponseWriter
	Size() int64
}

// DefaultLogger creates a Logger middleware with sensible defaults.
func DefaultLogger(log *logger.Logger, skipPaths ...string) handler.MiddlewareFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return Logger(LoggerConfig{
		Log:           log,
		SkipPaths:     skip,
		SlowThreshold: 2 * time.Second,
	})
}
