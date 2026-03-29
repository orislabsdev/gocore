package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
)

// ─────────────────────────────────────────────────────────────────────────────
// Limiter Backend Interface
// ─────────────────────────────────────────────────────────────────────────────

// limiterBackend is the interface for rate limiting storage (memory or remote).
type limiterBackend interface {
	// Allow checks if the given key is allowed to make a request.
	// It returns a boolean indicating if it's allowed, and the duration
	// the client should wait if it's not allowed. The error is non-nil
	// only if the backend failed to evaluate the request.
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}

// ─────────────────────────────────────────────────────────────────────────────
// Middleware
// ─────────────────────────────────────────────────────────────────────────────

// RateLimit returns a middleware that enforces per-client request rate limits
// using a token-bucket algorithm.
//
// By default the client is identified by their IP address. Supply a custom
// KeyFunc in the config to key by authenticated user ID, API key, or any
// other dimension.
//
// The limiter supports pluggable backends:
//   - "memory": fast, lock-based in-memory map (default)
//   - "redis": distributed rate-limiting via atomic Lua scripts
//
// When a client exceeds the configured rate, the middleware responds with
// HTTP 429 Too Many Requests and sets the Retry-After header. The request
// chain is aborted (the actual handler is not called).
//
// The done channel should be closed when the server shuts down to stop the
// background cleanup goroutines for the memory backend:
//
//	done := make(chan struct{})
//	app.Use(middleware.RateLimit(cfg.RateLimit, done))
//	// on shutdown:
//	close(done)
func RateLimit(cfg config.RateLimitConfig, done <-chan struct{}) handler.MiddlewareFunc {
	if done == nil {
		// Provide a never-closed channel so the store can still start safely.
		done = make(chan struct{})
	}

	var backend limiterBackend
	if cfg.Provider == "redis" {
		backend = newRedisLimiterBackend(cfg)
	} else {
		// Fallback to "memory"
		backend = newMemoryLimiterBackend(cfg, done)
	}

	// Default key function: use the client's remote IP.
	keyFunc := cfg.KeyFunc
	if keyFunc == nil {
		keyFunc = defaultRateLimitKey
	}

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			key := keyFunc(ctx.Request)
			if key == "" {
				// Empty key means "skip rate limiting for this request."
				next(ctx)
				return
			}

			allowed, retryAfter, err := backend.Allow(ctx.Request.Context(), key)
			if err != nil {
				// Log the error but fail open so legitimate traffic isn't blocked by
				// a rate limiter backend outage.
				ctx.Logger().Error("rate limiter backend failed", "error", err, "key", key)
				next(ctx)
				return
			}

			if !allowed {
				// Inform the client how many seconds to wait before retrying.
				retrySec := int(retryAfter.Seconds())
				if retrySec < 1 {
					retrySec = 1
				}
				ctx.SetHeader("Retry-After", fmt.Sprintf("%d", retrySec))
				ctx.TooManyRequests()
				return
			}

			next(ctx)
		}
	}
}

// defaultRateLimitKey extracts the client IP from the request.
// It respects X-Real-IP and X-Forwarded-For headers when present.
func defaultRateLimitKey(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list; use the leftmost value.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to the raw RemoteAddr (host:port). Strip the port.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
