package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
)

// cors is the pre-computed CORS state derived from config, stored once at
// server startup. Pre-computing avoids repeated string operations on every
// request.
type cors struct {
	cfg            config.CORSConfig
	allowedOrigins map[string]struct{} // exact origins for O(1) lookup
	allowAll       bool                // true when "*" is in AllowedOrigins
	allowedMethods string              // pre-joined header value
	allowedHeaders string              // pre-joined header value
	exposedHeaders string              // pre-joined header value
	maxAge         string              // pre-formatted header value
}

// CORS returns a middleware that enforces the Cross-Origin Resource Sharing
// policy defined in cfg.
//
// The middleware handles both simple requests and preflight (OPTIONS) requests.
// For preflight requests it writes the appropriate headers and responds with
// 204 No Content, aborting the chain. For simple requests it adds headers and
// passes control to the next handler.
//
// Security note: Never combine AllowedOrigins: ["*"] with AllowCredentials:
// true — browsers reject such responses per the Fetch specification.
//
//	app.Use(middleware.CORS(cfg.CORS))
func CORS(cfg config.CORSConfig) handler.MiddlewareFunc {
	c := buildCORS(cfg)

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			origin := ctx.Request.Header.Get("Origin")

			// Non-CORS request (no Origin header) — pass through unmodified.
			if origin == "" {
				next(ctx)
				return
			}

			// Validate the origin against the allow-list.
			allowed := c.allowAll || c.isOriginAllowed(origin)
			if !allowed {
				// Reject with 403; do not set any CORS headers.
				ctx.Forbidden("origin not allowed")
				return
			}

			rw := ctx.ResponseWriter()

			// ── Common headers (set on both preflight and actual responses) ──
			rw.Header().Set("Access-Control-Allow-Origin", originHeader(c, origin))
			if cfg.AllowCredentials {
				rw.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if c.exposedHeaders != "" {
				rw.Header().Set("Access-Control-Expose-Headers", c.exposedHeaders)
			}

			// ── Preflight request ─────────────────────────────────────────────
			if ctx.Request.Method == http.MethodOptions &&
				ctx.Request.Header.Get("Access-Control-Request-Method") != "" {

				rw.Header().Set("Access-Control-Allow-Methods", c.allowedMethods)
				rw.Header().Set("Access-Control-Allow-Headers", c.allowedHeaders)
				if c.maxAge != "" {
					rw.Header().Set("Access-Control-Max-Age", c.maxAge)
				}
				// Respond to the preflight without forwarding to the actual handler.
				rw.WriteHeader(http.StatusNoContent)
				return
			}

			// ── Simple / actual request ───────────────────────────────────────
			// Vary header prevents caches from serving one origin's response to
			// another origin.
			rw.Header().Add("Vary", "Origin")

			next(ctx)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildCORS pre-computes the cors state from config.
func buildCORS(cfg config.CORSConfig) *cors {
	c := &cors{cfg: cfg}

	// Build origin lookup set.
	c.allowedOrigins = make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			c.allowAll = true
		}
		c.allowedOrigins[strings.ToLower(o)] = struct{}{}
	}

	// Pre-join header lists.
	c.allowedMethods = strings.Join(cfg.AllowedMethods, ", ")
	c.allowedHeaders = strings.Join(cfg.AllowedHeaders, ", ")
	c.exposedHeaders = strings.Join(cfg.ExposedHeaders, ", ")
	if cfg.MaxAge > 0 {
		c.maxAge = strconv.Itoa(cfg.MaxAge)
	}

	return c
}

// isOriginAllowed performs a case-insensitive lookup against the pre-built
// origin set.
func (c *cors) isOriginAllowed(origin string) bool {
	_, ok := c.allowedOrigins[strings.ToLower(origin)]
	return ok
}

// originHeader returns the value to set for Access-Control-Allow-Origin.
// When wildcards are used, return "*"; otherwise echo the request's origin.
// Echoing the exact origin is required when credentials are allowed.
func originHeader(c *cors, requestOrigin string) string {
	if c.allowAll && !c.cfg.AllowCredentials {
		return "*"
	}
	return requestOrigin
}
