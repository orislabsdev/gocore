package middleware

import (
	"strings"

	"github.com/orislabsdev/gocore/auth"
	"github.com/orislabsdev/gocore/handler"
)

// AuthConfig holds options for the JWT authentication middleware.
type AuthConfig struct {
	// Manager is the auth.Manager used to validate tokens.
	// Required.
	Manager *auth.Manager

	// TokenLookup defines how the JWT is located in the request.
	// Format: "source:key[,source:key,...]"
	// Supported sources:
	//   header  — HTTP request header (e.g., "header:Authorization")
	//   query   — URL query parameter   (e.g., "query:token")
	//   cookie  — HTTP cookie            (e.g., "cookie:jwt")
	// Default: "header:Authorization"
	TokenLookup string

	// AuthScheme is the prefix stripped from the Authorization header value
	// before parsing (default: "Bearer").
	AuthScheme string

	// ErrorHandler is called when token extraction or validation fails.
	// If nil the default handler writes a 401 Unauthorized response.
	ErrorHandler func(ctx *handler.Context, err error)

	// SkipFunc, when set, is called before each request. If it returns true
	// the authentication check is skipped entirely for that request.
	// Use this to whitelist specific paths or client IPs at middleware level.
	SkipFunc func(ctx *handler.Context) bool
}

// Auth returns a middleware that validates a JWT from the configured source and
// stores the resulting claims in the request context.
//
// On success the claims are accessible via ctx.Claims() in downstream
// middleware and handlers. On failure the chain is aborted with a 401 or 403.
//
// Note: Only routes marked as private (the default) execute this middleware.
// Routes explicitly marked as .Public() bypass it entirely via the router.
//
//	api := app.Group("/api/v1")
//	api.Use(middleware.Auth(middleware.AuthConfig{
//	    Manager:     jwtManager,
//	    TokenLookup: "header:Authorization",
//	    AuthScheme:  "Bearer",
//	}))
func Auth(cfg AuthConfig) handler.MiddlewareFunc {
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}
	if cfg.AuthScheme == "" {
		cfg.AuthScheme = "Bearer"
	}

	extractors := buildExtractors(cfg.TokenLookup, cfg.AuthScheme)

	errHandler := cfg.ErrorHandler
	if errHandler == nil {
		errHandler = defaultAuthError
	}

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			// Allow the caller to skip auth for specific requests.
			if cfg.SkipFunc != nil && cfg.SkipFunc(ctx) {
				next(ctx)
				return
			}

			// Try each configured source in order; use the first non-empty token.
			var rawToken string
			for _, extract := range extractors {
				if t := extract(ctx); t != "" {
					rawToken = t
					break
				}
			}

			if rawToken == "" {
				errHandler(ctx, auth.ErrTokenInvalid)
				return
			}

			// Validate the token and extract claims.
			claims, err := cfg.Manager.ValidateAccessToken(rawToken)
			if err != nil {
				errHandler(ctx, err)
				return
			}

			// Attach claims to the request context for downstream use.
			ctx.Request = handler.SetClaims(ctx.Request, claims)

			next(ctx)
		}
	}
}

// RequireRoles returns a middleware that checks whether the authenticated
// principal holds at least one of the specified roles. It must be placed after
// an Auth middleware in the chain.
//
//	adminOnly := middleware.RequireRoles("admin", "superuser")
//	api.GET("/admin/users", listUsers, adminOnly)
func RequireRoles(roles ...string) handler.MiddlewareFunc {
	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			claims := ctx.Claims()
			if claims == nil {
				ctx.Unauthorized("")
				return
			}
			if !auth.HasRole(claims, roles...) {
				ctx.Forbidden("insufficient permissions")
				return
			}
			next(ctx)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Token extractors
// ─────────────────────────────────────────────────────────────────────────────

// tokenExtractor extracts a raw JWT string from a request context.
type tokenExtractor func(ctx *handler.Context) string

// buildExtractors parses the TokenLookup string and returns a slice of
// extractor functions, one per source:key pair.
func buildExtractors(lookup, scheme string) []tokenExtractor {
	var fns []tokenExtractor
	parts := strings.Split(lookup, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, ":")
		if idx < 0 {
			continue
		}
		source, key := part[:idx], part[idx+1:]
		switch source {
		case "header":
			fns = append(fns, headerExtractor(key, scheme))
		case "query":
			fns = append(fns, queryExtractor(key))
		case "cookie":
			fns = append(fns, cookieExtractor(key))
		}
	}
	return fns
}

// headerExtractor extracts a token from a request header, stripping the
// auth scheme prefix (e.g., "Bearer ").
func headerExtractor(header, scheme string) tokenExtractor {
	prefix := scheme + " "
	return func(ctx *handler.Context) string {
		value := ctx.Request.Header.Get(header)
		if strings.HasPrefix(value, prefix) {
			return value[len(prefix):]
		}
		// Also accept tokens without a scheme prefix.
		return value
	}
}

// queryExtractor extracts a token from a URL query parameter.
func queryExtractor(param string) tokenExtractor {
	return func(ctx *handler.Context) string {
		return ctx.Request.URL.Query().Get(param)
	}
}

// cookieExtractor extracts a token from a named cookie.
func cookieExtractor(name string) tokenExtractor {
	return func(ctx *handler.Context) string {
		cookie, err := ctx.Request.Cookie(name)
		if err != nil {
			return ""
		}
		return cookie.Value
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Default error handler
// ─────────────────────────────────────────────────────────────────────────────

// defaultAuthError writes a 401 for missing/invalid tokens and 403 for
// expired tokens (the client should refresh, not re-authenticate from scratch).
func defaultAuthError(ctx *handler.Context, err error) {
	switch err {
	case auth.ErrTokenExpired:
		// 401 so clients know they should attempt a token refresh.
		ctx.Unauthorized("token expired")
	default:
		ctx.Unauthorized("invalid or missing token")
	}
}
