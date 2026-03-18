package gocore_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/orislabsdev/gocore"
	"github.com/orislabsdev/gocore/auth"
	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/middleware"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test setup
// ─────────────────────────────────────────────────────────────────────────────

const testSecret = "integration-test-secret-must-be-32-bytes!"

// newTestApp builds a minimal gocore app for integration testing.
func newTestApp(t *testing.T) *gocore.Core {
	t.Helper()
	cfg := config.Default()
	cfg.JWT.Secret = testSecret
	cfg.JWT.Issuer = "test"
	cfg.JWT.Audience = []string{"test"}
	cfg.RateLimit.Enabled = false // disable for tests to avoid flakiness
	cfg.Log.Output = "stdout"
	cfg.Log.Level = "error" // suppress noise in test output

	app := gocore.NewWithConfig(cfg)
	app.UseDefaults()
	return app
}

// testToken issues a signed access token for the given subject and roles.
func testToken(t *testing.T, subject string, roles []string) string {
	t.Helper()
	cfg := config.Default()
	cfg.JWT.Secret = testSecret
	cfg.JWT.Issuer = "test"
	cfg.JWT.Audience = []string{"test"}

	mgr, err := auth.NewManager(cfg.JWT)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	tok, err := mgr.IssueAccessToken(subject, roles, nil)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	return tok
}

// do executes a request against the app's handler and returns the recorder.
func doReq(app *gocore.Core, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestPublicRouteNoAuth(t *testing.T) {
	app := newTestApp(t)
	app.GET("/health", func(ctx *handler.Context) {
		ctx.Success(map[string]string{"status": "ok"})
	}).Public()

	rec := doReq(app, http.MethodGet, "/health", nil, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

func TestPrivateRouteNoToken(t *testing.T) {
	app := newTestApp(t)

	// Mount auth middleware on a group.
	api := app.Group("/api")
	api.Use(middleware.Auth(middleware.AuthConfig{Manager: app.JWTManager()}))
	api.GET("/secret", func(ctx *handler.Context) {
		ctx.Success("should not reach here")
	})

	rec := doReq(app, http.MethodGet, "/api/secret", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

func TestPrivateRouteWithValidToken(t *testing.T) {
	app := newTestApp(t)

	api := app.Group("/api")
	api.Use(middleware.Auth(middleware.AuthConfig{Manager: app.JWTManager()}))
	api.GET("/profile", func(ctx *handler.Context) {
		claims := ctx.Claims()
		ctx.Success(map[string]string{"subject": claims.Subject})
	})

	token := testToken(t, "user-42", []string{"user"})
	rec := doReq(app, http.MethodGet, "/api/profile", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	if data["subject"] != "user-42" {
		t.Errorf("unexpected subject: %v", data["subject"])
	}
}

func TestRoleEnforcement(t *testing.T) {
	app := newTestApp(t)

	api := app.Group("/api")
	api.Use(middleware.Auth(middleware.AuthConfig{Manager: app.JWTManager()}))
	api.GET("/admin", func(ctx *handler.Context) {
		ctx.Success("admin only")
	}).Use(middleware.RequireRoles("admin"))

	t.Run("user without admin role gets 403", func(t *testing.T) {
		token := testToken(t, "user-1", []string{"user"})
		rec := doReq(app, http.MethodGet, "/api/admin", nil, map[string]string{
			"Authorization": "Bearer " + token,
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("admin user gets 200", func(t *testing.T) {
		token := testToken(t, "admin-1", []string{"admin"})
		rec := doReq(app, http.MethodGet, "/api/admin", nil, map[string]string{
			"Authorization": "Bearer " + token,
		})
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})
}

func TestRequestIDPropagation(t *testing.T) {
	app := newTestApp(t)
	app.GET("/ping", func(ctx *handler.Context) {
		ctx.Success(nil)
	}).Public()

	// Supply our own request ID and verify it is echoed back.
	rec := doReq(app, http.MethodGet, "/ping", nil, map[string]string{
		"X-Request-ID": "test-id-abc",
	})
	if rec.Header().Get("X-Request-ID") != "test-id-abc" {
		t.Errorf("X-Request-ID not echoed; got %q", rec.Header().Get("X-Request-ID"))
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	app := newTestApp(t)
	app.GET("/ping", func(ctx *handler.Context) {
		ctx.Success(nil)
	}).Public()

	rec := doReq(app, http.MethodGet, "/ping", nil, nil)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-Xss-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, expected := range headers {
		got := rec.Header().Get(header)
		if got != expected {
			t.Errorf("%s: got %q, want %q", header, got, expected)
		}
	}
}

func TestNotFound(t *testing.T) {
	app := newTestApp(t)
	rec := doReq(app, http.MethodGet, "/definitely-does-not-exist", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	app := newTestApp(t)
	app.GET("/item", func(ctx *handler.Context) { ctx.Success(nil) }).Public()

	rec := doReq(app, http.MethodPost, "/item", nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestJSONResponseEnvelope(t *testing.T) {
	app := newTestApp(t)
	app.GET("/data", func(ctx *handler.Context) {
		ctx.Success(map[string]int{"count": 42})
	}).Public()

	rec := doReq(app, http.MethodGet, "/data", nil, nil)

	var resp handler.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Data == nil {
		t.Error("expected non-nil data")
	}
}
