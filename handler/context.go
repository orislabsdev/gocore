// Package handler provides the core types that route handlers interact with.
//
// The two primary exports are:
//
//   - HandlerFunc  — the function signature for all route handlers.
//   - Context      — an enriched request context that wraps http.ResponseWriter
//     and *http.Request and provides helpers for reading inputs and
//     writing outputs.
//
// Unlike frameworks that hide net/http, gocore exposes the raw Writer and
// Request fields so that existing net/http middleware and libraries continue
// to work without adaptation.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/orislabsdev/gocore/auth"
)

// ─────────────────────────────────────────────────────────────────────────────
// Function types
// ─────────────────────────────────────────────────────────────────────────────

// HandlerFunc is the function signature for all route handlers.
// Handlers receive a *Context that exposes helpers for every common operation.
//
//nolint:revive // Stutter is intentional for consistency with net/http
type HandlerFunc func(ctx *Context)

// MiddlewareFunc wraps a HandlerFunc to intercept the request/response cycle.
// Middleware must call next(ctx) to pass control to the next handler or
// middleware in the chain.
//
//	func MyMiddleware(next handler.HandlerFunc) handler.HandlerFunc {
//	    return func(ctx *handler.Context) {
//	        // before handler
//	        next(ctx)
//	        // after handler
//	    }
//	}
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// ─────────────────────────────────────────────────────────────────────────────
// URL Parameters
// ─────────────────────────────────────────────────────────────────────────────

// paramsKey is the unexported context key used to store URL parameters.
type paramsKey struct{}

// Params holds URL path parameters extracted during route matching.
// e.g. for route /users/:id — Params{"id": "42"}.
type Params map[string]string

// SetParams stores URL parameters in a request's context and returns the
// updated *http.Request. Called by the router after a successful match.
func SetParams(r *http.Request, p Params) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), paramsKey{}, p))
}

// GetParams retrieves URL parameters from the request context.
// Returns an empty (non-nil) Params if none are stored.
func GetParams(r *http.Request) Params {
	if p, ok := r.Context().Value(paramsKey{}).(Params); ok {
		return p
	}
	return Params{}
}

// ─────────────────────────────────────────────────────────────────────────────
// Claims context key
// ─────────────────────────────────────────────────────────────────────────────

// claimsKey is the unexported context key used to store JWT claims.
type claimsKey struct{}

// SetClaims stores JWT claims in the request context. Called by the auth
// middleware after successful token validation.
func SetClaims(r *http.Request, c *auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey{}, c))
}

// GetClaims retrieves JWT claims from the request context.
// Returns nil if no claims have been stored (unauthenticated request).
func GetClaims(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey{}).(*auth.Claims)
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// ResponseWriter wrapper
// ─────────────────────────────────────────────────────────────────────────────

// responseWriter wraps http.ResponseWriter to track whether the status code
// has been written and what value was used. This prevents double-write bugs.
type responseWriter struct {
	http.ResponseWriter
	status    int
	size      int64
	committed bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader captures the status code and marks the response as committed.
func (rw *responseWriter) WriteHeader(code int) {
	if rw.committed {
		return // ignore double-write
	}
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.committed = true
}

// Write forwards the body bytes and accumulates the total byte count.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.committed {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += int64(n)
	return n, err
}

// Status returns the HTTP status code that was written.
func (rw *responseWriter) Status() int { return rw.status }

// Size returns the number of body bytes written.
func (rw *responseWriter) Size() int64 { return rw.size }

// ─────────────────────────────────────────────────────────────────────────────
// Context
// ─────────────────────────────────────────────────────────────────────────────

// Context is the central type passed to every route handler and middleware.
// It wraps the http.ResponseWriter and *http.Request and adds convenience
// methods for the most common operations.
//
// A Context is recycled from a sync.Pool to minimise allocation pressure. Do
// not hold a reference to a Context after the handler returns.
type Context struct {
	// writer is the wrapped response writer. Access it via Writer() or the
	// typed response helpers (JSON, String, etc.).
	writer *responseWriter

	// Request is the incoming HTTP request. Access headers, body, and query
	// parameters directly through this field.
	Request *http.Request

	// keys is a request-scoped key-value store. Use Set/Get to pass data
	// between middleware and handlers (e.g., the authenticated user object).
	keys map[string]any
	mu   sync.RWMutex
}

// Logger defines the interface for structured logging within a handler.
// It is compatible with log/slog and the gocore/logger package.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// noopLogger is a silent fallback when no logger has been stored in context.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// contextPool recycles Context objects to reduce GC pressure.
var contextPool = sync.Pool{
	New: func() any { return &Context{} },
}

// NewContext allocates (or retrieves from the pool) a Context and initialises
// it for the current request. Called by the router on every request.
func NewContext(w http.ResponseWriter, r *http.Request) *Context {
	ctx := contextPool.Get().(*Context)
	ctx.writer = newResponseWriter(w)
	ctx.Request = r
	ctx.keys = nil // lazily initialised on first Set()
	return ctx
}

// Release returns the Context to the pool. Called by the router after the
// handler chain completes.
func Release(ctx *Context) {
	ctx.writer = nil
	ctx.Request = nil
	ctx.keys = nil
	contextPool.Put(ctx)
}

// ─── Writer access ───────────────────────────────────────────────────────────

// ResponseWriter returns the underlying http.ResponseWriter so that callers
// can use it with third-party middleware that requires the raw interface.
func (c *Context) ResponseWriter() http.ResponseWriter { return c.writer }

// Status returns the HTTP status code written so far (default 200).
func (c *Context) Status() int { return c.writer.status }

// Written reports whether the response headers have been committed.
func (c *Context) Written() bool { return c.writer.committed }

// ─── Request input helpers ───────────────────────────────────────────────────

// Param returns the URL path parameter with the given name.
// Returns an empty string if the parameter was not captured.
//
//	// Route: /users/:id
//	id := ctx.Param("id")
func (c *Context) Param(name string) string {
	return GetParams(c.Request)[name]
}

// Query returns the first value of the named query string parameter.
// Returns the provided default value if the parameter is absent.
//
//	page := ctx.Query("page", "1")
func (c *Context) Query(name, defaultValue string) string {
	v := c.Request.URL.Query().Get(name)
	if v == "" {
		return defaultValue
	}
	return v
}

// QueryAll returns all values for the named query string parameter.
func (c *Context) QueryAll(name string) []string {
	return c.Request.URL.Query()[name]
}

// Header returns the value of the named request header.
func (c *Context) Header(name string) string {
	return c.Request.Header.Get(name)
}

// BindJSON reads the request body, decodes it as JSON into dst, and closes the
// body. Returns an error if the body is not valid JSON or cannot be decoded
// into dst.
//
// The request Content-Type does not need to be application/json; this method
// always attempts JSON decoding regardless.
func (c *Context) BindJSON(dst any) error {
	defer c.Request.Body.Close()
	if err := json.NewDecoder(c.Request.Body).Decode(dst); err != nil {
		return fmt.Errorf("bind json: %w", err)
	}
	return nil
}

// BodyBytes reads and returns the raw request body. The body can only be read
// once; subsequent calls return an empty byte slice. Prefer BindJSON for JSON
// payloads.
func (c *Context) BodyBytes() ([]byte, error) {
	defer c.Request.Body.Close()
	b, err := io.ReadAll(io.LimitReader(c.Request.Body, 32<<20)) // 32 MiB max
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return b, nil
}

// ContentType returns the media type of the request body, stripping parameters
// (e.g., "application/json" from "application/json; charset=utf-8").
func (c *Context) ContentType() string {
	ct := c.Request.Header.Get("Content-Type")
	if ct == "" {
		return ""
	}
	mt, _, _ := mime.ParseMediaType(ct)
	return mt
}

// ClientIP returns the most-accurate client IP address available.
//
// It checks X-Forwarded-For before falling back to RemoteAddr.
// Crucially, X-Forwarded-For is ONLY trusted if the direct connection
// (RemoteAddr) originates from a known Cloudflare IP address. This prevents
// IP spoofing from non-Cloudflare sources.
func (c *Context) ClientIP() string {
	remoteIP, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		remoteIP = c.Request.RemoteAddr
	}

	// Only trust the forwarding header if the requester is Cloudflare.
	if isCloudflareIP(remoteIP) {
		if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// The leftmost IP is the original client IP.
			return strings.TrimSpace(parts[0])
		}
	}

	return remoteIP
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloudflare IP ranges (Static list as of 2024)
// ─────────────────────────────────────────────────────────────────────────────

var (
	cfIPv4 = []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	}
	cfIPv6 = []string{
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
		"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
	}
	cfNets []net.IPNet
	once   sync.Once
)

// isCloudflareIP checks whether the given IP string belongs to Cloudflare.
func isCloudflareIP(ipStr string) bool {
	once.Do(func() {
		for _, s := range append(cfIPv4, cfIPv6...) {
			_, n, err := net.ParseCIDR(s)
			if err == nil {
				cfNets = append(cfNets, *n)
			}
		}
	})

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range cfNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Claims returns the JWT claims stored in the request context by the auth
// middleware. Returns nil for unauthenticated requests.
func (c *Context) Claims() *auth.Claims {
	return GetClaims(c.Request)
}

// Logger returns the logger stored in the request-scoped keys.
//
// If no logger was set (e.g., the Logger middleware is disabled), it returns
// a no-op logger to prevent nil pointer panics in handlers.
func (c *Context) Logger() Logger {
	if v, ok := c.Get("logger"); ok {
		if l, ok := v.(Logger); ok {
			return l
		}
	}
	return noopLogger{}
}

// ─── Request-scoped key-value store ─────────────────────────────────────────

// Set stores a value under the given key. Values survive for the duration of
// the request and are accessible to all subsequent middleware and handlers.
// Thread-safe.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.keys == nil {
		c.keys = make(map[string]any, 4)
	}
	c.keys[key] = value
	c.mu.Unlock()
}

// Get retrieves a value stored by Set. Returns the value and true if found.
// Thread-safe.
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	v, ok := c.keys[key]
	c.mu.RUnlock()
	return v, ok
}

// MustGet retrieves a value by key and panics if it is not found.
// Use this only when the value is guaranteed to exist (e.g., set by a
// required middleware that runs before this handler).
func (c *Context) MustGet(key string) any {
	v, ok := c.Get(key)
	if !ok {
		panic("handler: key not found in context: " + key)
	}
	return v
}
