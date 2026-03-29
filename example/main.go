// Command example demonstrates how to use the gocore library as a backend
// foundation. It registers public and private routes, shows JWT issuance and
// validation, uses the validator, and wires up all default middleware.
//
// Run:
//
//	JWT_SECRET=supersecret go run ./example/main.go
//
// Test endpoints:
//
//	# Health check (public)
//	curl http://localhost:8080/health
//
//	# Login — returns an access token (public)
//	curl -s -X POST http://localhost:8080/auth/login \
//	     -H 'Content-Type: application/json' \
//	     -d '{"email":"admin@example.com","password":"secret123"}' | jq
//
//	# Protected profile (copy token from login)
//	curl http://localhost:8080/api/v1/me \
//	     -H 'Authorization: Bearer <token>'
//
//	# Admin-only route
//	curl http://localhost:8080/api/v1/admin/users \
//	     -H 'Authorization: Bearer <token>'
package main

import (
	"fmt"
	"github.com/orislabsdev/gocore"
	"github.com/orislabsdev/gocore/auth"
	"github.com/orislabsdev/gocore/builtin"
	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/middleware"
	"github.com/orislabsdev/gocore/validate"
	"os"
)

// ─────────────────────────────────────────────────────────────────────────────
// Entry point
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	// ── 1. Configuration ──────────────────────────────────────────────────────
	// Start from the production-safe defaults and override only what differs.
	cfg := config.Default()
	cfg.Server.Port = 8080

	// Always load secrets from the environment — never hard-code them.
	cfg.JWT.Secret = getEnv("JWT_SECRET", "change-me-in-production-use-32-plus-bytes")
	cfg.JWT.Issuer = "gocore-example"
	cfg.JWT.Audience = []string{"gocore-example-clients"}

	// Relax CORS for local development; restrict to real origins in production.
	cfg.CORS.AllowedOrigins = []string{"*"}

	// Optional: Use Redis for distributed rate limiting.
	// cfg.RateLimit.Provider = "redis"
	// cfg.RateLimit.Redis.Address = "localhost:6379"

	// ── 2. Create the application ─────────────────────────────────────────────
	app := gocore.NewWithConfig(cfg)

	// ── 3. Register global middleware ─────────────────────────────────────────
	// UseDefaults installs: RequestID, Recovery, Logger, Security, CORS,
	// RateLimit — in the correct order.
	app.UseDefaults()

	// Install Prometheus observability middleware, skipping noisy utility paths.
	app.Use(middleware.Prometheus("/health", "/metrics", "/ready"))

	// ── 4. Public routes — no JWT required ────────────────────────────────────

	app.GET("/health", builtin.HealthCheck()).Public().Name("health")
	app.GET("/ready", builtin.ReadyCheck()).Public().Name("ready")
	app.GET("/metrics", builtin.Prometheus()) // Private by default

	// Auth endpoints are public (you cannot require a token to obtain a token).
	app.POST("/auth/login", loginHandler(app.JWTManager())).Public().Name("auth.login")
	app.POST("/auth/refresh", refreshHandler(app.JWTManager())).Public().Name("auth.refresh")

	// ── 5. Private API group — JWT required ───────────────────────────────────
	api := app.Group("/api/v1")
	api.Use(middleware.Auth(middleware.AuthConfig{
		Manager:     app.JWTManager(),
		TokenLookup: "header:Authorization",
		AuthScheme:  "Bearer",
	}))

	// User profile — accessible to any authenticated user.
	api.GET("/me", getMeHandler())
	api.PUT("/me", updateMeHandler())

	// Example of an optional query parameter: POST /api/v1/follow?user=123
	api.POST("/follow", followUserHandler())

	// User CRUD — accessible to any authenticated user.
	users := api.Group("/users")
	users.GET("", listUsersHandler())
	users.POST("", createUserHandler())
	users.GET("/:id", getUserHandler())
	users.PUT("/:id", updateUserHandler())
	users.DELETE("/:id", deleteUserHandler())

	// Admin sub-group — additionally requires the "admin" role.
	admin := api.Group("/admin")
	admin.Use(middleware.RequireRoles("admin"))
	admin.GET("/users", adminListUsersHandler())
	admin.DELETE("/users/:id", adminDeleteUserHandler())

	// ── 6. Start the server ───────────────────────────────────────────────────
	if err := app.Run(); err != nil {
		app.Logger().Fatal("server error", "err", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Auth handlers
// ─────────────────────────────────────────────────────────────────────────────

// loginRequest is the expected JSON body for POST /auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the JSON body returned on successful login.
type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // seconds
}

// loginHandler returns a handler that validates credentials and issues a JWT
// pair. In production replace the stub credential check with your real auth
// logic (e.g., database lookup + bcrypt comparison).
func loginHandler(mgr *auth.Manager) handler.HandlerFunc {
	return func(ctx *handler.Context) {
		var req loginRequest
		if err := ctx.BindJSON(&req); err != nil {
			ctx.BadRequest("invalid JSON body")
			return
		}

		// ── Validate input ────────────────────────────────────────────────────
		v := validate.New()
		v.Required("email", req.Email)
		v.Email("email", req.Email)
		v.Required("password", req.Password)
		v.MinLen("password", req.Password, 6)

		if v.HasErrors() {
			ctx.UnprocessableEntity("validation failed", v.Errors())
			return
		}

		// ── Credential check (stub) ───────────────────────────────────────────
		// Replace this block with your real user store lookup.
		roles := []string{"user"}
		userID := "user-001"
		if req.Email == "admin@example.com" {
			roles = append(roles, "admin")
		}
		if req.Password != "secret123" {
			ctx.Fail(401, "INVALID_CREDENTIALS", "email or password is incorrect", nil)
			return
		}

		// ── Issue tokens ──────────────────────────────────────────────────────
		accessToken, err := mgr.IssueAccessToken(userID, roles, map[string]any{
			"email": req.Email,
		})
		if err != nil {
			ctx.InternalServerError("")
			return
		}

		refreshToken, err := mgr.IssueRefreshToken(userID)
		if err != nil {
			ctx.InternalServerError("")
			return
		}

		ctx.Created(loginResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresIn:    int(15 * 60), // 15 minutes in seconds
		})
	}
}

// refreshHandler issues a new access token in exchange for a valid refresh
// token.
func refreshHandler(mgr *auth.Manager) handler.HandlerFunc {
	return func(ctx *handler.Context) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.BadRequest("invalid JSON body")
			return
		}
		if body.RefreshToken == "" {
			ctx.BadRequest("refresh_token is required")
			return
		}

		// Validate the refresh token.
		claims, err := mgr.ValidateRefreshToken(body.RefreshToken)
		if err != nil {
			ctx.Unauthorized("invalid or expired refresh token")
			return
		}

		// Issue a new access token for the same subject.
		accessToken, err := mgr.IssueAccessToken(claims.Subject, claims.Roles, claims.Extra)
		if err != nil {
			ctx.InternalServerError("")
			return
		}

		ctx.Success(map[string]string{
			"access_token": accessToken,
			"token_type":   "Bearer",
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// User handlers
// ─────────────────────────────────────────────────────────────────────────────

func getMeHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		claims := ctx.Claims() // populated by the Auth middleware
		ctx.Success(map[string]any{
			"subject": claims.Subject,
			"roles":   claims.Roles,
			"extra":   claims.Extra,
		})
	}
}

func updateMeHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		var body map[string]any
		if err := ctx.BindJSON(&body); err != nil {
			ctx.BadRequest("invalid JSON body")
			return
		}
		// Business logic would live here (update user in DB, etc.)
		ctx.Success(map[string]string{"message": "profile updated"})
	}
}

func listUsersHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		page := ctx.Query("page", "1")
		perPage := ctx.Query("per_page", "20")
		ctx.SuccessWithMeta(
			[]map[string]string{
				{"id": "user-001", "email": "admin@example.com"},
				{"id": "user-002", "email": "alice@example.com"},
			},
			map[string]string{"page": page, "per_page": perPage},
		)
	}
}

func createUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.BadRequest("invalid JSON body")
			return
		}

		v := validate.New()
		v.Required("email", body.Email).Email("email", body.Email)
		v.Required("password", body.Password).MinLen("password", body.Password, 8)
		if v.HasErrors() {
			ctx.UnprocessableEntity("validation failed", v.Errors())
			return
		}

		ctx.Created(map[string]string{"id": "user-new", "email": body.Email})
	}
}

func getUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		id := ctx.Param("id") // extracted from /users/:id
		ctx.Success(map[string]string{"id": id, "email": fmt.Sprintf("user-%s@example.com", id)})
	}
}

func updateUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		id := ctx.Param("id")
		ctx.Success(map[string]string{"id": id, "message": "updated"})
	}
}

func deleteUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		ctx.NoContent()
	}
}

func followUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		// ctx.Query(name, defaultValue) handles optional query parameters.
		// If the parameter is missing from the URL, it returns the defaultValue.
		target := ctx.Query("user", "")

		if target == "" {
			ctx.BadRequest("missing 'user' query parameter")
			return
		}

		ctx.Logger().Info("user followed",
			"follower", ctx.Claims().Subject,
			"target", target,
		)

		ctx.Success(map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("you are now following %s", target),
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Admin handlers
// ─────────────────────────────────────────────────────────────────────────────

func adminListUsersHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		ctx.Success([]map[string]string{
			{"id": "user-001", "email": "admin@example.com", "role": "admin"},
			{"id": "user-002", "email": "alice@example.com", "role": "user"},
		})
	}
}

func adminDeleteUserHandler() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		id := ctx.Param("id")
		ctx.Logger().Info("admin deleted user", "admin", ctx.Claims().Subject, "target", id)
		ctx.NoContent()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Utility
// ─────────────────────────────────────────────────────────────────────────────

// getEnv returns the environment variable named key, or fallback if unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
