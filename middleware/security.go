package middleware

import (
	"fmt"
	"strings"

	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
)

// Security returns a middleware that sets HTTP security response headers based
// on the provided SecurityConfig.
//
// The headers written depend on the configuration but typically include:
//
//   - Strict-Transport-Security (HSTS)
//   - Content-Security-Policy
//   - X-Frame-Options
//   - X-Content-Type-Options
//   - Referrer-Policy
//   - Permissions-Policy
//   - X-XSS-Protection (legacy)
//
// All headers are set before calling the next handler, so downstream
// middleware or handlers can still override individual values if necessary.
//
//	app.Use(middleware.Security(cfg.Security))
func Security(cfg config.SecurityConfig) handler.MiddlewareFunc {
	// Pre-compute the HSTS header value once at startup rather than on every
	// request.
	hsts := buildHSTS(cfg)

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			h := ctx.ResponseWriter().Header()

			// ── Strict-Transport-Security ────────────────────────────────────
			// HSTS tells browsers to only connect over HTTPS for the specified
			// duration. Only set this header when actually serving HTTPS;
			// setting it on HTTP responses has no effect and can be confusing.
			if hsts != "" {
				h.Set("Strict-Transport-Security", hsts)
			}

			// ── Content-Security-Policy ──────────────────────────────────────
			// CSP is the primary defence against XSS. Provide a policy tailored
			// to your application's actual needs.
			if cfg.ContentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
			}

			// ── X-Frame-Options ──────────────────────────────────────────────
			// Prevents the page from being loaded in an <iframe> or <frame>,
			// defending against clickjacking attacks.
			if cfg.XFrameOptions != "" {
				h.Set("X-Frame-Options", cfg.XFrameOptions)
			}

			// ── X-Content-Type-Options ───────────────────────────────────────
			// Prevents browsers from MIME-sniffing the response away from the
			// declared Content-Type.
			if cfg.XContentTypeOptions {
				h.Set("X-Content-Type-Options", "nosniff")
			}

			// ── Referrer-Policy ──────────────────────────────────────────────
			// Controls how much referrer information is included with requests.
			if cfg.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", cfg.ReferrerPolicy)
			}

			// ── Permissions-Policy ───────────────────────────────────────────
			// Restricts access to browser features (camera, microphone, etc.)
			// from this origin.
			if cfg.PermissionsPolicy != "" {
				h.Set("Permissions-Policy", cfg.PermissionsPolicy)
			}

			// ── X-XSS-Protection ─────────────────────────────────────────────
			// Legacy header for older IE/Chrome versions. Modern browsers use
			// CSP. Keep enabled for broad compatibility.
			if cfg.XXSSProtection {
				h.Set("X-XSS-Protection", "1; mode=block")
			}

			// ── Remove server fingerprinting headers ─────────────────────────
			// These headers are set by Go's net/http package and reveal
			// implementation details that attackers can use for reconnaissance.
			h.Del("X-Powered-By")
			h.Set("Server", "") // suppress version disclosure

			next(ctx)
		}
	}
}

// buildHSTS constructs the Strict-Transport-Security header value from config.
// Returns an empty string if HSTSMaxAge is 0 (i.e., HSTS is disabled).
func buildHSTS(cfg config.SecurityConfig) string {
	if cfg.HSTSMaxAge <= 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge))
	if cfg.HSTSIncludeSubdomains {
		sb.WriteString("; includeSubDomains")
	}
	if cfg.HSTSPreload {
		sb.WriteString("; preload")
	}
	return sb.String()
}

// DefaultSecurity returns a Security middleware with opinionated defaults
// appropriate for most JSON API services.
//
// Notable choices:
//   - HSTS is enabled (1 year + subdomains).
//   - CSP defaults to "default-src 'none'" — the most restrictive policy.
//     Override with a real policy for HTML-serving services.
//   - X-Frame-Options: DENY.
func DefaultSecurity() handler.MiddlewareFunc {
	return Security(config.SecurityConfig{
		Enabled:               true,
		HSTSMaxAge:            31_536_000,
		HSTSIncludeSubdomains: true,
		ContentSecurityPolicy: "default-src 'none'",
		XFrameOptions:         "DENY",
		XContentTypeOptions:   true,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		XXSSProtection:        true,
	})
}
