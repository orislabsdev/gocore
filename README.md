# gocore

> A highly secure, optimized, and configurable HTTP backend library for Go.

[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

---

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration Reference](#configuration-reference)
- [Routing](#routing)
  - [Static Routes](#static-routes)
  - [Path Parameters](#path-parameters)
  - [Wildcards](#wildcards)
  - [Route Groups](#route-groups)
  - [Public vs Private Routes](#public-vs-private-routes)
- [Middleware](#middleware)
  - [Built-in Middleware](#built-in-middleware)
  - [Writing Custom Middleware](#writing-custom-middleware)
  - [Middleware Order](#middleware-order)
- [Authentication](#authentication)
  - [Issuing Tokens](#issuing-tokens)
  - [Validating Tokens](#validating-tokens)
  - [Role-Based Access Control](#role-based-access-control)
- [Request Handling](#request-handling)
- [Response Helpers](#response-helpers)
- [Validation](#validation)
- [Built-in Handlers](#built-in-handlers)
- [TLS / HTTPS](#tls--https)
- [Graceful Shutdown](#graceful-shutdown)
- [Testing](#testing)
- [Project Structure](#project-structure)
- [Security Design](#security-design)

---

## Overview

**gocore** is a modular, zero-magic HTTP backend library designed to be installed
as a dependency and used as the foundation for new Go services. It gives you:

| Feature | Description |
|---|---|
| **Trie router** | O(depth) matching with static, `:param`, and `*wildcard` segments |
| **JWT auth** | HMAC-signed access + refresh tokens with multi-source extraction |
| **Middleware chain** | Ordered, composable `MiddlewareFunc` wrappers |
| **Security headers** | HSTS, CSP, X-Frame-Options, X-Content-Type-Options, etc. |
| **CORS** | Per-origin policy with preflight support |
| **Rate limiting** | Per-IP (or custom key) token-bucket limiter with auto-cleanup |
| **Structured logging** | JSON or text output, per-request access log |
| **Panic recovery** | Catches panics, logs stack traces, returns `500` |
| **Graceful shutdown** | Drains connections on `SIGINT`/`SIGTERM` |
| **Validation** | Zero-reflection request-payload validation helpers |
| **TLS / mTLS** | Configurable cipher suites, mTLS client-cert verification |

gocore wraps Go's standard `net/http` without replacing it. Every
`http.Handler`, `http.ResponseWriter`, and `*http.Request` is still accessible,
so third-party middleware and libraries work without adaptation.

---

## Architecture

```
gocore/
├── core.go            — root Core type; application entry point
├── config/            — all configuration structs (no external deps)
├── auth/              — JWT issuance, validation, claims
├── handler/           — Context, HandlerFunc, MiddlewareFunc, response helpers
├── middleware/        — Recovery, Logger, Security, CORS, RateLimit, Auth
├── router/            — trie-based HTTP router + route groups
├── server/            — net/http.Server wrapper with graceful shutdown
├── builtin/           — ready-made HealthCheck, ReadyCheck, Metrics handlers
├── validate/          — request-payload validation helpers
└── example/           — full working example application
```

Each package has a single well-defined responsibility. The dependency graph is
strictly acyclic:

```
config  ←  auth  ←  middleware  ←  router  ←  server  ←  core
  ↑                      ↑
handler ──────────────────
```

---

## Installation

```bash
go get github.com/orislabsdev/gocore@latest
```

**Requirements:** Go 1.22+

**Dependencies (only 2):**

| Package | Purpose |
|---|---|
| `github.com/golang-jwt/jwt/v5` | HMAC JWT signing and parsing |
| `golang.org/x/time` | Token-bucket rate limiter |

---

## Quick Start

```go
package main

import (
    "os"

    "github.com/orislabsdev/gocore"
    "github.com/orislabsdev/gocore/builtin"
    "github.com/orislabsdev/gocore/config"
    "github.com/orislabsdev/gocore/handler"
    "github.com/orislabsdev/gocore/middleware"
)

func main() {
    // 1. Configuration — start from safe defaults, override what you need.
    cfg := config.Default()
    cfg.Server.Port = 8080
    cfg.JWT.Secret  = os.Getenv("JWT_SECRET")

    // 2. Create the application.
    app := gocore.NewWithConfig(cfg)

    // 3. Register global middleware (RequestID → Recovery → Logger →
    //    Security headers → CORS → RateLimit).
    app.UseDefaults()

    // 4. Public routes — no JWT required.
    app.GET("/health", builtin.HealthCheck()).Public()

    // 5. Private route group — JWT required.
    api := app.Group("/api/v1")
    api.Use(middleware.Auth(middleware.AuthConfig{
        Manager: app.JWTManager(),
    }))
    api.GET("/me", func(ctx *handler.Context) {
        ctx.Success(map[string]any{
            "subject": ctx.Claims().Subject,
            "roles":   ctx.Claims().Roles,
        })
    })

    // 6. Start — blocks until SIGINT/SIGTERM.
    if err := app.Run(); err != nil {
        app.Logger().Fatal("server exited with error", "err", err)
    }
}
```

```bash
JWT_SECRET=my-32-byte-secret go run ./main.go
# {"time":"…","level":"INFO","msg":"server starting","addr":"0.0.0.0:8080","tls":false}

curl http://localhost:8080/health
# {"status":"ok"}

curl -X POST http://localhost:8080/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email":"admin@example.com","password":"secret123"}'
```

---

## Configuration Reference

All configuration lives in `config.Config`. Start from `config.Default()` and
override only the values you need — every field has a documented safe default.

```go
cfg := config.Default()

// ── Server ────────────────────────────────────────────────────────────────
cfg.Server.Host              = "0.0.0.0"       // bind address
cfg.Server.Port              = 8080             // TCP port
cfg.Server.ReadTimeout       = 30 * time.Second
cfg.Server.WriteTimeout      = 30 * time.Second
cfg.Server.IdleTimeout       = 120 * time.Second
cfg.Server.ReadHeaderTimeout = 10 * time.Second // Slowloris defence
cfg.Server.MaxHeaderBytes    = 1 << 20          // 1 MiB
cfg.Server.ShutdownTimeout   = 30 * time.Second

// ── TLS ───────────────────────────────────────────────────────────────────
cfg.TLS.Enabled    = true
cfg.TLS.CertFile   = "/etc/ssl/certs/server.crt"
cfg.TLS.KeyFile    = "/etc/ssl/private/server.key"
cfg.TLS.MinVersion = tls.VersionTLS12

// ── CORS ──────────────────────────────────────────────────────────────────
cfg.CORS.AllowedOrigins   = []string{"https://app.example.com"}
cfg.CORS.AllowCredentials = true
cfg.CORS.MaxAge           = 86400

// ── Rate Limiting ─────────────────────────────────────────────────────────
cfg.RateLimit.RequestsPerSecond = 100
cfg.RateLimit.Burst             = 20
cfg.RateLimit.KeyFunc           = func(r *http.Request) string {
    return r.Header.Get("X-API-Key") // key by API key instead of IP
}

// ── JWT ───────────────────────────────────────────────────────────────────
cfg.JWT.Secret          = os.Getenv("JWT_SECRET") // ≥32 bytes
cfg.JWT.Issuer          = "my-service"
cfg.JWT.Audience        = []string{"my-clients"}
cfg.JWT.Algorithm       = "HS256"                 // HS256 | HS384 | HS512
cfg.JWT.AccessTokenTTL  = 15 * time.Minute
cfg.JWT.RefreshTokenTTL = 7 * 24 * time.Hour
cfg.JWT.TokenLookup     = "header:Authorization,cookie:jwt"

// ── Logging ───────────────────────────────────────────────────────────────
cfg.Log.Level      = "info"    // debug | info | warn | error
cfg.Log.Format     = "json"    // json | text
cfg.Log.Output     = "stdout"  // stdout | stderr | /var/log/app.log
cfg.Log.RequestLog = true

// ── Security Headers ──────────────────────────────────────────────────────
cfg.Security.HSTSMaxAge            = 31_536_000 // 1 year
cfg.Security.HSTSIncludeSubdomains = true
cfg.Security.ContentSecurityPolicy = "default-src 'self'"
cfg.Security.XFrameOptions         = "DENY"
cfg.Security.XContentTypeOptions   = true
cfg.Security.ReferrerPolicy        = "strict-origin-when-cross-origin"
```

---

## Routing

### Static Routes

```go
app.GET("/users", listUsers)
app.POST("/users", createUser)
app.PUT("/users/:id", updateUser)
app.PATCH("/users/:id", patchUser)
app.DELETE("/users/:id", deleteUser)
app.OPTIONS("/users", optionsHandler)
app.HEAD("/users", headHandler)
app.Any("/catch-all", anyMethodHandler)
```

### Path Parameters

Segments prefixed with `:` capture a single URL segment:

```go
// Route: /articles/:year/:month/:slug
app.GET("/articles/:year/:month/:slug", func(ctx *handler.Context) {
    year  := ctx.Param("year")   // "2024"
    month := ctx.Param("month")  // "03"
    slug  := ctx.Param("slug")   // "my-article"
    ctx.Success(map[string]string{"year": year, "month": month, "slug": slug})
})
```

### Wildcards

Segments prefixed with `*` capture the rest of the path (including slashes):

```go
// Route: /files/*path
app.GET("/files/*path", func(ctx *handler.Context) {
    path := ctx.Param("path") // "docs/api/reference.pdf"
    ctx.String(200, "serve: %s", path)
})
```

**Match priority (high → low):** static > `:param` > `*wildcard`

```go
app.GET("/users/me",   getMeHandler)   // matches /users/me   exactly
app.GET("/users/:id",  getUserHandler) // matches /users/42   (param)
app.GET("/users/*rest", fallback)      // matches /users/a/b  (wildcard)
```

### Route Groups

Groups share a URL prefix and optionally a set of middleware:

```go
v1 := app.Group("/api/v1")
v1.Use(middleware.Auth(...))      // applies to all routes in v1

users := v1.Group("/users")       // prefix: /api/v1/users
users.GET("",        listUsers)   // GET  /api/v1/users
users.POST("",       createUser)  // POST /api/v1/users
users.GET("/:id",    getUser)     // GET  /api/v1/users/:id
users.DELETE("/:id", deleteUser)  // DELETE /api/v1/users/:id

// Sub-group with additional middleware
admin := v1.Group("/admin")
admin.Use(middleware.RequireRoles("admin"))
admin.GET("/stats", adminStats)   // GET /api/v1/admin/stats
```

### Public vs Private Routes

Every route is **private by default**. Mark a route as public using `.Public()`:

```go
// Public — no JWT verification
app.GET("/health",      healthHandler).Public()
app.POST("/auth/login", loginHandler).Public()

// Private (default) — JWT required when Auth middleware is in the chain
app.GET("/profile", profileHandler)

// Fluent chaining
app.POST("/items", createItem).
    Name("items.create").
    Use(rateLimiter).
    Private()
```

The `IsPublic` flag is stored on the route entry and exposed via
`router.MatchResult.IsPublic`. You can use this flag in custom middleware to
conditionally skip authentication:

```go
middleware.Auth(middleware.AuthConfig{
    SkipFunc: func(ctx *handler.Context) bool {
        // Read the flag stored during route matching
        if v, ok := ctx.Get("route.public"); ok {
            return v.(bool)
        }
        return false
    },
})
```

---

## Middleware

### Built-in Middleware

| Middleware | Constructor | Description |
|---|---|---|
| RequestID | `middleware.RequestID()` | Generates/propagates `X-Request-ID` |
| Recovery | `middleware.DefaultRecovery(log)` | Catches panics, returns `500` |
| Logger | `middleware.DefaultLogger(log, skipPaths...)` | Per-request access log |
| Security | `middleware.Security(cfg.Security)` | HTTP security headers |
| CORS | `middleware.CORS(cfg.CORS)` | Cross-Origin Resource Sharing |
| RateLimit | `middleware.RateLimit(cfg.RateLimit, done)` | Token-bucket per-client throttling |
| Auth | `middleware.Auth(cfg)` | JWT validation; stores claims in context |
| RequireRoles | `middleware.RequireRoles("admin", ...)` | Role-based access check |

### Writing Custom Middleware

```go
// MiddlewareFunc signature: func(next HandlerFunc) HandlerFunc
func Tracing(serviceName string) handler.MiddlewareFunc {
    return func(next handler.HandlerFunc) handler.HandlerFunc {
        return func(ctx *handler.Context) {
            traceID := ctx.Header("X-Trace-ID")
            if traceID == "" {
                traceID = generateTraceID()
            }

            // Store the trace ID for downstream handlers.
            ctx.Set("trace_id", traceID)
            ctx.SetHeader("X-Trace-ID", traceID)

            // Call the next handler (mandatory).
            next(ctx)

            // Post-processing (runs after the handler returns).
            status := ctx.Status()
            logTrace(traceID, status)
        }
    }
}

// Apply globally:
app.Use(Tracing("my-service"))

// Apply to a group:
api.Use(Tracing("api"))

// Apply to a single route:
app.GET("/special", handler).Use(Tracing("special"))
```

### Middleware Order

Middleware executes in registration order. The first `Use()` call is the
outermost wrapper (first on the way in, last on the way out):

```
Request → MW1 → MW2 → MW3 → Handler → MW3 → MW2 → MW1 → Response
```

The recommended global order for a typical JSON API:

```go
app.Use(
    middleware.RequestID(),        // 1. assign a request ID first
    middleware.DefaultRecovery(log), // 2. catch panics before logging
    middleware.DefaultLogger(log),   // 3. log every request
    middleware.Security(cfg.Security), // 4. security headers
    middleware.CORS(cfg.CORS),       // 5. CORS policy
    middleware.RateLimit(cfg.RateLimit, app.Done()), // 6. rate limit
)
// Auth is NOT global — apply it to specific groups only.
```

---

## Authentication

### Issuing Tokens

```go
mgr := app.JWTManager() // *auth.Manager

// Issue an access token (15 min default TTL)
accessToken, err := mgr.IssueAccessToken(
    "user-42",                        // subject (user ID)
    []string{"admin", "billing"},     // roles
    map[string]any{"email": "u@example.com"}, // extra claims
)

// Issue a refresh token (7 days default TTL)
refreshToken, err := mgr.IssueRefreshToken("user-42")
```

### Validating Tokens

```go
// In a handler — the Auth middleware has already done this for private routes.
// Use direct validation only for custom flows (e.g., WebSocket upgrades).
claims, err := mgr.ValidateAccessToken(rawToken)
if err != nil {
    switch err {
    case auth.ErrTokenExpired:
        ctx.Unauthorized("token expired")
    default:
        ctx.Unauthorized("invalid token")
    }
    return
}
fmt.Println(claims.Subject) // "user-42"
fmt.Println(claims.Roles)   // ["admin", "billing"]
```

### Role-Based Access Control

```go
// Option 1 — middleware (recommended for route-level control)
admin := api.Group("/admin")
admin.Use(middleware.RequireRoles("admin", "superuser"))

// Option 2 — inline check in a handler
func myHandler(ctx *handler.Context) {
    if !auth.HasRole(ctx.Claims(), "billing") {
        ctx.Forbidden("billing permission required")
        return
    }
    // ... authorized logic
}
```

---

## Request Handling

```go
func myHandler(ctx *handler.Context) {
    // ── URL parameters ──────────────────────────────────────────
    id := ctx.Param("id")               // from /users/:id

    // ── Query parameters ────────────────────────────────────────
    page    := ctx.Query("page", "1")   // default: "1"
    all     := ctx.QueryAll("tag")      // []string

    // ── Request headers ─────────────────────────────────────────
    accept  := ctx.Header("Accept")
    reqID   := ctx.Header("X-Request-ID")

    // ── Body parsing ────────────────────────────────────────────
    var payload MyStruct
    if err := ctx.BindJSON(&payload); err != nil {
        ctx.BadRequest("invalid JSON")
        return
    }

    // ── Client information ──────────────────────────────────────
    ip          := ctx.ClientIP()       // respects X-Real-IP / X-Forwarded-For
    contentType := ctx.ContentType()    // "application/json"

    // ── JWT claims (set by Auth middleware) ─────────────────────
    claims := ctx.Claims()              // *auth.Claims or nil
    userID := claims.Subject

    // ── Request-scoped key-value store ──────────────────────────
    ctx.Set("myKey", someValue)
    val, ok := ctx.Get("myKey")
    val2 := ctx.MustGet("requiredKey")  // panics if not set

    // ── Raw access ──────────────────────────────────────────────
    req := ctx.Request                  // *http.Request
    w   := ctx.ResponseWriter()         // http.ResponseWriter
}
```

---

## Response Helpers

```go
// ── Success responses ────────────────────────────────────────────────────
ctx.Success(data)                       // 200 {"success":true,"data":…}
ctx.Created(data)                       // 201 {"success":true,"data":…}
ctx.SuccessWithMeta(data, meta)         // 200 + pagination meta
ctx.NoContent()                         // 204 (no body)

// ── Error responses (standard JSON envelope) ─────────────────────────────
ctx.BadRequest("email is required")     // 400
ctx.Unauthorized("token expired")       // 401
ctx.Forbidden("admin only")             // 403
ctx.NotFound("user not found")          // 404
ctx.Conflict("email already taken")     // 409
ctx.UnprocessableEntity("failed", errs) // 422 with field-level details
ctx.TooManyRequests()                   // 429
ctx.InternalServerError("")             // 500

// ── Custom responses ──────────────────────────────────────────────────────
ctx.JSON(201, myStruct)                 // custom status + JSON body
ctx.String(200, "pong")                 // plain text
ctx.HTML(200, "<h1>Hello</h1>")         // HTML
ctx.Blob(200, "image/png", pngBytes)    // binary
ctx.Redirect(301, "https://new.url")    // redirect

// ── Response headers ──────────────────────────────────────────────────────
ctx.SetHeader("X-Custom", "value")
ctx.AddHeader("X-Multi", "extra")
```

**Standard envelope shape:**

```json
// Success
{"success": true, "data": {...}, "meta": {...}}

// Error
{"success": false, "error": {"code": "NOT_FOUND", "message": "user not found"}}

// Validation error
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "validation failed",
    "details": {
      "email":    ["is required", "must be a valid email address"],
      "password": ["must be at least 8 characters"]
    }
  }
}
```

---

## Validation

```go
func createUserHandler(ctx *handler.Context) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
        Age      int    `json:"age"`
        Role     string `json:"role"`
    }
    if err := ctx.BindJSON(&req); err != nil {
        ctx.BadRequest("invalid JSON")
        return
    }

    v := validate.New()
    v.Required("email",    req.Email)
    v.Email("email",       req.Email)
    v.Required("password", req.Password)
    v.MinLen("password",   req.Password, 8)
    v.MaxLen("password",   req.Password, 72) // bcrypt limit
    v.Range("age",         float64(req.Age), 18, 120)
    v.OneOf("role",        req.Role, "user", "editor", "admin")

    if v.HasErrors() {
        ctx.UnprocessableEntity("validation failed", v.Errors())
        return
    }
    // ... proceed with valid data
}
```

**Available rules:**

| Method | Description |
|---|---|
| `Required(field, value)` | Non-empty after trimming whitespace |
| `RequiredIf(field, value, condition)` | Required only when condition is true |
| `MinLen(field, value, n)` | UTF-8 character count ≥ n |
| `MaxLen(field, value, n)` | UTF-8 character count ≤ n |
| `LenBetween(field, value, min, max)` | Length within inclusive range |
| `Email(field, value)` | RFC 5322 email format |
| `URL(field, value)` | Valid absolute HTTP/HTTPS URL |
| `Range(field, value, min, max)` | Numeric value within inclusive range |
| `Min(field, value, min)` | Numeric value ≥ min |
| `Max(field, value, max)` | Numeric value ≤ max |
| `OneOf(field, value, opts...)` | Value matches one of the permitted strings |
| `NotEmpty(field, slice)` | Slice has at least one element |
| `Matches(field, value, regexp)` | Value matches a compiled regexp |
| `Custom(field, value, fn)` | User-supplied validation function |

---

## Built-in Handlers

```go
import "github.com/orislabsdev/gocore/builtin"

// Liveness probe — always 200 OK
app.GET("/health",  builtin.HealthCheck()).Public()
app.GET("/health",  builtin.VersionedHealthCheck("v1.2.3")).Public()

// Readiness probe — runs dependency checks concurrently
app.GET("/ready", builtin.ReadyCheck(
    "database", db.PingContext,   // func(context.Context) error
    "cache",    redis.PingContext,
)).Public()

// Basic runtime metrics (protect in production)
startedAt := time.Now()
app.GET("/metrics", builtin.Metrics(startedAt)) // Private by default
```

---

## TLS / HTTPS

```go
cfg := config.Default()
cfg.TLS.Enabled   = true
cfg.TLS.CertFile  = "/etc/ssl/certs/server.crt"
cfg.TLS.KeyFile   = "/etc/ssl/private/server.key"
cfg.TLS.MinVersion = tls.VersionTLS13 // enforce TLS 1.3 only

// Mutual TLS (mTLS) — require client certificates
cfg.TLS.ClientAuth = tls.RequireAndVerifyClientCert
cfg.TLS.ClientCAs  = "/etc/ssl/certs/client-ca.crt"

app := gocore.NewWithConfig(cfg)
```

When `TLS.Enabled` is true the server automatically shifts from port 8080 to
8443 if the port has not been explicitly overridden. In production, run the
TLS listener directly on 443 and set `cfg.Server.Port = 443`.

---

## Graceful Shutdown

The server registers a handler for `SIGINT` and `SIGTERM`. When either signal
arrives:

1. The HTTP listener stops accepting new connections.
2. All background goroutines (e.g., the rate-limiter cleanup loop) are stopped
   via the `done` channel.
3. Active connections are given up to `ShutdownTimeout` (default 30 s) to
   complete their current requests.
4. `Run()` returns `nil`.

```go
// The done channel is closed the moment shutdown begins.
// Pass it to any middleware or goroutine that should stop cleanly.
app.Use(middleware.RateLimit(cfg.RateLimit, app.Done()))
```

You can also trigger a programmatic shutdown by calling `app.Run()` in a
goroutine and signalling yourself:

```go
go app.Run()
// ... later:
syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
```

---

## Testing

gocore apps are straightforward to test with `net/http/httptest`:

```go
func TestMyRoute(t *testing.T) {
    cfg := config.Default()
    cfg.JWT.Secret = "test-secret-32-bytes-minimum!!"
    cfg.RateLimit.Enabled = false // avoid flakiness in unit tests
    cfg.Log.Level = "error"       // suppress access log noise

    app := gocore.NewWithConfig(cfg)
    app.UseDefaults()
    app.GET("/ping", func(ctx *handler.Context) {
        ctx.Success(map[string]string{"msg": "pong"})
    }).Public()

    req := httptest.NewRequest(http.MethodGet, "/ping", nil)
    rec := httptest.NewRecorder()
    app.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", rec.Code)
    }
}
```

Run all tests with the race detector:

```bash
make test
# or
go test -race -count=1 ./...
```

---

## Project Structure

```
gocore/
├── core.go                     # Core type — app entry point
├── integration_test.go         # Full-stack integration tests
├── go.mod
├── go.sum
├── Makefile
│
├── config/
│   └── config.go               # All configuration structs + Default()
│
├── auth/
│   ├── jwt.go                  # Manager, IssueAccessToken, ValidateToken
│   └── jwt_test.go
│
├── handler/
│   ├── context.go              # Context type, HandlerFunc, MiddlewareFunc
│   └── response.go             # JSON/text/blob response helpers
│
├── middleware/
│   ├── middleware.go           # Chain(), RequestID(), generateID()
│   ├── recovery.go             # Panic recovery
│   ├── logger.go               # Access log
│   ├── security.go             # HTTP security headers
│   ├── cors.go                 # CORS policy
│   ├── ratelimit.go            # Token-bucket rate limiter
│   └── auth.go                 # JWT extraction + validation
│
├── router/
│   ├── router.go               # Trie router, ServeHTTP, Match
│   ├── group.go                # Route groups
│   └── router_test.go
│
├── server/
│   └── server.go               # net/http.Server wrapper, TLS, shutdown
│
├── builtin/
│   └── handlers.go             # HealthCheck, ReadyCheck, Metrics
│
├── validate/
│   ├── validate.go             # Validator type + rule methods
│   └── validate_test.go
│
└── example/
    └── main.go                 # Full working example application
```

---

## Security Design

| Layer | Mechanism | Default |
|---|---|---|
| **Transport** | TLS 1.2+, strong cipher suites, HSTS | Configurable; TLS optional |
| **Authentication** | HMAC-signed JWT (HS256/384/512) | Required on private routes |
| **Authorization** | Role claims in JWT, `RequireRoles` middleware | Opt-in per route/group |
| **Rate limiting** | Token-bucket per client IP (or custom key) | 100 rps / burst 20 |
| **CORS** | Explicit allow-list; wildcard disabled with credentials | Configurable |
| **Headers** | HSTS, CSP, X-Frame-Options, X-Content-Type-Options | Enabled by default |
| **Panic recovery** | Deferred recovery in every request; no process crash | Always active |
| **Request timeouts** | ReadHeaderTimeout 10 s (Slowloris defence) | Conservative defaults |
| **Body size** | MaxHeaderBytes 1 MiB; body reads limited to 32 MiB | Configurable |
| **Algorithm confusion** | JWT `ValidMethods` whitelist; one allowed algorithm | Enforced |
| **Secret management** | Secrets never hard-coded; loaded from env / secret store | By convention |

### Secrets checklist

- `JWT.Secret`: load from `os.Getenv` or a secrets manager; use ≥ 32 random bytes.
- `TLS.KeyFile`: restrict file permissions (`chmod 600`).
- Never commit `.env` files with real secrets to source control.

---

## License

MIT — see [LICENSE](LICENSE).
