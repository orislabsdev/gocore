// Package middleware provides production-ready HTTP middleware for the gocore
// library. Every middleware follows the same signature:
//
//	func SomeName(options...) handler.MiddlewareFunc
//
// This makes them trivially composable — apply them globally via app.Use() or
// to a specific route group via group.Use().
//
// Included middleware:
//
//   - Recovery   — catches panics and converts them into 500 responses.
//   - Logger     — emits a structured access-log entry per request.
//   - Security   — sets HTTP security headers (HSTS, CSP, X-Frame-Options…).
//   - CORS       — enforces a Cross-Origin Resource Sharing policy.
//   - RateLimit  — per-client token-bucket rate limiting.
//   - Auth       — validates a JWT and populates the request context with claims.
//   - RequestID  — generates or propagates a unique request identifier.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"

	"github.com/orislabsdev/gocore/handler"
)

// ─────────────────────────────────────────────────────────────────────────────
// Chain
// ─────────────────────────────────────────────────────────────────────────────

// Chain applies a slice of MiddlewareFuncs around a HandlerFunc.
// Middleware is applied from outermost to innermost — the first entry in
// the slice runs first on the way in and last on the way out.
//
//	final := middleware.Chain(innerHandler, mw1, mw2, mw3)
//	// request:  mw1 → mw2 → mw3 → innerHandler
//	// response: innerHandler → mw3 → mw2 → mw1
func Chain(h handler.HandlerFunc, mw ...handler.MiddlewareFunc) handler.HandlerFunc {
	// Wrap in reverse order so that mw[0] is the outermost wrapper.
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// ─────────────────────────────────────────────────────────────────────────────
// Request ID
// ─────────────────────────────────────────────────────────────────────────────

// RequestID is a lightweight middleware that ensures every request carries a
// unique identifier. If the incoming request already carries an X-Request-ID
// header it is preserved; otherwise a new cryptographically random ID is
// generated.
//
// The ID is propagated to handlers via ctx.Header("X-Request-ID") and echoed
// back to the client in the response.
func RequestID() handler.MiddlewareFunc {
	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			id := ctx.Header("X-Request-ID")
			if id == "" {
				id = generateID()
			}
			// Set on both response (visible to client) and request (visible to
			// downstream middleware and handlers).
			ctx.SetHeader("X-Request-ID", id)
			ctx.Request.Header.Set("X-Request-ID", id)
			next(ctx)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ID generation
// ─────────────────────────────────────────────────────────────────────────────

// requestCounter is used as a fallback counter when crypto/rand is unavailable.
var requestCounter uint64

// generateID returns a 16-character hex string derived from 8 random bytes.
// Falls back to a counter-based ID if the OS entropy source is temporarily
// exhausted (rare in practice).
func generateID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&requestCounter, 1)
		return fmt.Sprintf("%016x", n)
	}
	return hex.EncodeToString(buf)
}
