// Package gocore is a highly secure, optimized, and configurable HTTP backend
// library for Go applications.
//
// # Overview
//
// gocore is designed to be installed as a dependency in a project and used as
// a full-featured HTTP backend foundation. It handles:
//
//   - Routing       — trie-based URL matching with path parameters and wildcards.
//   - Middleware    — composable, ordered middleware chains.
//   - Security      — JWT authentication, security headers, CORS, rate limiting.
//   - Observability — structured access logging, panic recovery.
//   - Lifecycle     — graceful shutdown on SIGINT / SIGTERM.
//
// All subsystems are individually configurable via the config package and can
// be replaced or extended independently.
//
// # Quick start
//
//	package main
//
//	import (
//	    "os"
//	    "github.com/orislabsdev/gocore"
//	    "github.com/orislabsdev/gocore/config"
//	    "github.com/orislabsdev/gocore/handler"
//	    "github.com/orislabsdev/gocore/middleware"
//	)
//
//	func main() {
//	    cfg := config.Default()
//	    cfg.Server.Port = 8080
//	    cfg.JWT.Secret = os.Getenv("JWT_SECRET")
//
//	    app := gocore.NewWithConfig(cfg)
//
//	    // Global middleware (applied to every request)
//	    app.Use(
//	        middleware.RequestID(),
//	        middleware.DefaultRecovery(app.Logger()),
//	        middleware.DefaultLogger(app.Logger()),
//	        middleware.Security(cfg.Security),
//	        middleware.CORS(cfg.CORS),
//	        middleware.RateLimit(cfg.RateLimit, app.Done()),
//	    )
//
//	    // Public route — no JWT required
//	    app.GET("/health", healthHandler).Public()
//
//	    // Private route group — JWT required
//	    api := app.Group("/api/v1")
//	    api.Use(middleware.Auth(middleware.AuthConfig{Manager: app.JWTManager()}))
//	    api.GET("/profile", getProfile)
//
//	    app.Run()
//	}
package gocore

import (
	"net/http"

	"github.com/orislabsdev/gocore/auth"
	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/logger"
	"github.com/orislabsdev/gocore/middleware"
	"github.com/orislabsdev/gocore/router"
	"github.com/orislabsdev/gocore/server"
)

// ─────────────────────────────────────────────────────────────────────────────
// Core
// ─────────────────────────────────────────────────────────────────────────────

// Core is the main application object. It wires together configuration,
// routing, middleware, authentication, and the HTTP server.
//
// Create one Core per application (it is not designed to be shared across
// multiple HTTP listeners).
type Core struct {
	cfg        *config.Config
	router     *router.Router
	log        *logger.Logger
	jwtManager *auth.Manager
	done       chan struct{} // closed on shutdown; used to stop background goroutines
}

// New creates a Core with production-safe defaults.
// Call NewWithConfig to override specific settings.
func New() *Core {
	return NewWithConfig(config.Default())
}

// NewWithConfig creates a Core with the provided configuration.
// Panics if the JWT secret is configured but invalid.
func NewWithConfig(cfg *config.Config) *Core {
	log := logger.NewFromStrings(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output)

	c := &Core{
		cfg:    cfg,
		router: router.New(),
		log:    log,
		done:   make(chan struct{}),
	}

	// Initialise the JWT manager when a secret has been provided.
	// Leaving the secret empty is valid for services that don't use JWT.
	if cfg.JWT.Secret != "" {
		mgr, err := auth.NewManager(cfg.JWT)
		if err != nil {
			log.Fatal("failed to initialise JWT manager", "err", err)
		}
		c.jwtManager = mgr
	}

	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// Accessors
// ─────────────────────────────────────────────────────────────────────────────

// Logger returns the application logger. Use it in your own handlers and
// middleware for consistent structured output.
func (c *Core) Logger() *logger.Logger { return c.log }

// Config returns the root configuration. Useful when building custom
// middleware that reads from the same configuration source.
func (c *Core) Config() *config.Config { return c.cfg }

// JWTManager returns the auth.Manager for issuing and validating tokens.
// Returns nil if no JWT secret was configured.
func (c *Core) JWTManager() *auth.Manager { return c.jwtManager }

// Done returns a channel that is closed when the server begins shutting down.
// Pass this to middleware that run background goroutines (e.g., RateLimit).
func (c *Core) Done() <-chan struct{} { return c.done }

// ─────────────────────────────────────────────────────────────────────────────
// Middleware registration
// ─────────────────────────────────────────────────────────────────────────────

// Use appends one or more global middleware to the application.
// Global middleware runs on every request before route-specific middleware.
// Must be called before Run.
func (c *Core) Use(mw ...handler.MiddlewareFunc) {
	c.router.Use(mw...)
}

// ─────────────────────────────────────────────────────────────────────────────
// Route registration (convenience wrappers over the embedded router)
// ─────────────────────────────────────────────────────────────────────────────

// GET registers a GET handler. Returns a *Route for fluent chaining (.Public(),
// .Name(), .Use()).
func (c *Core) GET(path string, h handler.HandlerFunc) *router.Route {
	return c.router.GET(path, h)
}

// POST registers a POST handler.
func (c *Core) POST(path string, h handler.HandlerFunc) *router.Route {
	return c.router.POST(path, h)
}

// PUT registers a PUT handler.
func (c *Core) PUT(path string, h handler.HandlerFunc) *router.Route {
	return c.router.PUT(path, h)
}

// PATCH registers a PATCH handler.
func (c *Core) PATCH(path string, h handler.HandlerFunc) *router.Route {
	return c.router.PATCH(path, h)
}

// DELETE registers a DELETE handler.
func (c *Core) DELETE(path string, h handler.HandlerFunc) *router.Route {
	return c.router.DELETE(path, h)
}

// OPTIONS registers an OPTIONS handler.
func (c *Core) OPTIONS(path string, h handler.HandlerFunc) *router.Route {
	return c.router.OPTIONS(path, h)
}

// HEAD registers a HEAD handler.
func (c *Core) HEAD(path string, h handler.HandlerFunc) *router.Route {
	return c.router.HEAD(path, h)
}

// Group creates a route group with a shared path prefix.
// Use groups to scope middleware (e.g., JWT auth) to a sub-tree of routes.
//
//	api := app.Group("/api/v1")
//	api.Use(middleware.Auth(...))
//	api.GET("/users", listUsers)
func (c *Core) Group(prefix string) *router.Group {
	return c.router.Group(prefix)
}

// Router returns the underlying *router.Router for advanced use cases.
func (c *Core) Router() *router.Router { return c.router }

// ─────────────────────────────────────────────────────────────────────────────
// Default middleware helpers
// ─────────────────────────────────────────────────────────────────────────────

// UseDefaults installs the recommended set of global middleware in the
// standard order:
//
//  1. RequestID   — ensure every request has a unique ID.
//  2. Recovery    — catch panics and return 500.
//  3. Logger      — emit a structured access-log entry.
//  4. Security    — set HTTP security headers.
//  5. CORS        — enforce the CORS policy.
//  6. RateLimit   — enforce per-client rate limits.
//
// Authentication (JWT) is intentionally omitted from the default set because
// it should be applied to specific route groups, not globally. Register it on
// a group via group.Use(middleware.Auth(...)).
//
// Call UseDefaults before registering routes and before calling Run.
func (c *Core) UseDefaults() {
	c.Use(
		middleware.RequestID(),
		middleware.DefaultRecovery(c.log),
		middleware.DefaultLogger(c.log, "/health", "/metrics"),
	)
	if c.cfg.Security.Enabled {
		c.Use(middleware.Security(c.cfg.Security))
	}
	if c.cfg.CORS.Enabled {
		c.Use(middleware.CORS(c.cfg.CORS))
	}
	if c.cfg.RateLimit.Enabled {
		c.Use(middleware.RateLimit(c.cfg.RateLimit, c.done))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Server lifecycle
// ─────────────────────────────────────────────────────────────────────────────

// Run starts the HTTP server and blocks until the server receives a shutdown
// signal (SIGINT/SIGTERM) or encounters an unrecoverable error.
//
// Run handles the full lifecycle:
//  1. Starts the HTTP (or HTTPS) listener.
//  2. Waits for an OS signal.
//  3. Closes the done channel (stopping background goroutines like RateLimit).
//  4. Drains active connections (up to ShutdownTimeout).
//  5. Returns nil on clean exit, or the error that caused the failure.
func (c *Core) Run() error {
	srv := server.New(*c.cfg, c.router, c.log)

	c.log.Info("gocore initialised",
		"addr", srv.Addr(),
		"tls", c.cfg.TLS.Enabled,
	)

	err := srv.ListenAndServe()

	// Signal background goroutines to stop.
	close(c.done)

	return err
}

// Handler returns the Core's router as an http.Handler. Use this when
// embedding gocore into an existing net/http setup instead of calling Run.
//
//	http.ListenAndServe(":8080", app.Handler())
func (c *Core) Handler() http.Handler { return c.router }
