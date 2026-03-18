package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/logger"
)

// RecoveryConfig holds options for the Recovery middleware.
type RecoveryConfig struct {
	// Log is the logger used to record panic details.
	// If nil, panics are silently recovered without logging.
	Log *logger.Logger

	// PrintStack controls whether the full goroutine stack trace is included
	// in the log entry (default: true). Disable in very high-traffic services
	// where panics—and their stack-trace allocations—are expected to be rare.
	PrintStack bool

	// Handler is an optional custom function called after a panic is recovered.
	// It receives the *Context and the recovered value. If nil, a standard
	// 500 Internal Server Error response is written.
	Handler func(ctx *handler.Context, recovered any)
}

// Recovery returns a middleware that catches panics anywhere in the handler
// chain, logs them with a stack trace, and returns a 500 Internal Server Error
// to the client. Without this middleware an unhandled panic would crash the
// entire process.
//
//	app.Use(middleware.Recovery(middleware.RecoveryConfig{
//	    Log:        log,
//	    PrintStack: true,
//	}))
func Recovery(cfg RecoveryConfig) handler.MiddlewareFunc {
	if !cfg.PrintStack {
		// Keep the default true unless the caller explicitly opts out.
		cfg.PrintStack = false
	}

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			defer func() {
				if r := recover(); r != nil {
					// Build a human-readable representation of the recovered value.
					var msg string
					switch v := r.(type) {
					case error:
						msg = v.Error()
					default:
						msg = fmt.Sprintf("%v", v)
					}

					// Log the panic with optional stack trace.
					if cfg.Log != nil {
						if cfg.PrintStack {
							stack := string(debug.Stack())
							cfg.Log.Error("panic recovered",
								"error", msg,
								"stack", stack,
								"method", ctx.Request.Method,
								"path", ctx.Request.URL.Path,
							)
						} else {
							cfg.Log.Error("panic recovered",
								"error", msg,
								"method", ctx.Request.Method,
								"path", ctx.Request.URL.Path,
							)
						}
					}

					// Write a response only if headers have not been committed yet.
					if !ctx.Written() {
						if cfg.Handler != nil {
							cfg.Handler(ctx, r)
						} else {
							ctx.InternalServerError("")
						}
					}

					// Abort the request by not calling next — execution resumes
					// here after the recover(), and the deferred function returns
					// normally.
				}
			}()

			next(ctx)
		}
	}
}

// DefaultRecovery creates a Recovery middleware with sensible defaults.
// Pass a logger to capture panic details; pass nil to recover silently.
func DefaultRecovery(log *logger.Logger) handler.MiddlewareFunc {
	return Recovery(RecoveryConfig{
		Log:        log,
		PrintStack: true,
		Handler: func(ctx *handler.Context, recovered any) {
			// Avoid leaking internal error details to the client.
			http.Error(
				ctx.ResponseWriter(),
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError,
			)
		},
	})
}
