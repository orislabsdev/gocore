package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/router"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// textHandler returns a HandlerFunc that writes a plain-text status 200 with the 
// given body string. Used to confirm which route was matched.
func textHandler(body string) handler.HandlerFunc {
	return func(ctx *handler.Context) {
		ctx.String(http.StatusOK, body)
	}
}

// get performs a GET request against the router and returns the response.
func get(r *router.Router, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// do performs a request with the given method against the router.
func do(r *router.Router, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ─────────────────────────────────────────────────────────────────────────────
// Static routes
// ─────────────────────────────────────────────────────────────────────────────

func TestStaticRoutes(t *testing.T) {
	r := router.New()
	r.GET("/", textHandler("root"))
	r.GET("/health", textHandler("health"))
	r.GET("/api/v1/users", textHandler("users"))

	tests := []struct {
		path string
		want string
		code int
	}{
		{"/", "root", 200},
		{"/health", "health", 200},
		{"/api/v1/users", "users", 200},
		{"/not-found", "", 404},
	}

	for _, tc := range tests {
		rec := get(r, tc.path)
		if rec.Code != tc.code {
			t.Errorf("GET %s: status = %d, want %d", tc.path, rec.Code, tc.code)
		}
		if tc.want != "" && rec.Body.String() != tc.want {
			t.Errorf("GET %s: body = %q, want %q", tc.path, rec.Body.String(), tc.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Parametric routes
// ─────────────────────────────────────────────────────────────────────────────

func TestParamRoutes(t *testing.T) {
	r := router.New()
	r.GET("/users/:id", func(ctx *handler.Context) {
		ctx.String(http.StatusOK, ctx.Param("id"))
	})
	r.GET("/users/:id/posts/:postID", func(ctx *handler.Context) {
		ctx.String(http.StatusOK, ctx.Param("id")+":"+ctx.Param("postID"))
	})

	tests := []struct {
		path string
		want string
	}{
		{"/users/42", "42"},
		{"/users/abc-123", "abc-123"},
		{"/users/99/posts/7", "99:7"},
	}

	for _, tc := range tests {
		rec := get(r, tc.path)
		if rec.Code != 200 {
			t.Errorf("GET %s: status = %d", tc.path, rec.Code)
		}
		if rec.Body.String() != tc.want {
			t.Errorf("GET %s: body = %q, want %q", tc.path, rec.Body.String(), tc.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Wildcard routes
// ─────────────────────────────────────────────────────────────────────────────

func TestWildcardRoutes(t *testing.T) {
	r := router.New()
	r.GET("/files/*path", func(ctx *handler.Context) {
		ctx.String(http.StatusOK, ctx.Param("path"))
	})

	tests := []struct {
		path string
		want string
	}{
		{"/files/a.txt", "a.txt"},
		{"/files/dir/sub/file.go", "dir/sub/file.go"},
	}

	for _, tc := range tests {
		rec := get(r, tc.path)
		if rec.Code != 200 {
			t.Errorf("GET %s: status = %d", tc.path, rec.Code)
		}
		if rec.Body.String() != tc.want {
			t.Errorf("GET %s: body = %q, want %q", tc.path, rec.Body.String(), tc.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP methods
// ─────────────────────────────────────────────────────────────────────────────

func TestHTTPMethods(t *testing.T) {
	r := router.New()
	r.GET("/item", textHandler("get"))
	r.POST("/item", textHandler("post"))
	r.PUT("/item", textHandler("put"))
	r.PATCH("/item", textHandler("patch"))
	r.DELETE("/item", textHandler("delete"))

	methods := []struct {
		method string
		want   string
	}{
		{http.MethodGet, "get"},
		{http.MethodPost, "post"},
		{http.MethodPut, "put"},
		{http.MethodPatch, "patch"},
		{http.MethodDelete, "delete"},
	}

	for _, tc := range methods {
		rec := do(r, tc.method, "/item")
		if rec.Code != 200 {
			t.Errorf("%s /item: status = %d", tc.method, rec.Code)
		}
		if rec.Body.String() != tc.want {
			t.Errorf("%s /item: body = %q, want %q", tc.method, rec.Body.String(), tc.want)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	r := router.New()
	r.GET("/item", textHandler("ok"))

	rec := do(r, http.MethodPost, "/item")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Route groups
// ─────────────────────────────────────────────────────────────────────────────

func TestGroups(t *testing.T) {
	r := router.New()

	api := r.Group("/api/v1")
	api.GET("/users", textHandler("list"))
	api.POST("/users", textHandler("create"))

	admin := api.Group("/admin")
	admin.GET("/stats", textHandler("stats"))

	tests := []struct {
		method string
		path   string
		want   string
		code   int
	}{
		{http.MethodGet, "/api/v1/users", "list", 200},
		{http.MethodPost, "/api/v1/users", "create", 200},
		{http.MethodGet, "/api/v1/admin/stats", "stats", 200},
		{http.MethodGet, "/api/v1/missing", "", 404},
	}

	for _, tc := range tests {
		rec := do(r, tc.method, tc.path)
		if rec.Code != tc.code {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.code)
		}
		if tc.want != "" && rec.Body.String() != tc.want {
			t.Errorf("%s %s: body = %q, want %q", tc.method, tc.path, rec.Body.String(), tc.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Public / private flags
// ─────────────────────────────────────────────────────────────────────────────

func TestPublicRouteFlag(t *testing.T) {
	r := router.New()
	r.GET("/public", textHandler("pub")).Public()
	r.GET("/private", textHandler("priv")) // default is private

	// Confirm routes respond correctly (flag check is at the router.Match level).
	rec := get(r, "/public")
	if rec.Code != 200 {
		t.Errorf("public route: status = %d", rec.Code)
	}
	rec = get(r, "/private")
	if rec.Code != 200 {
		t.Errorf("private route: status = %d", rec.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Global middleware
// ─────────────────────────────────────────────────────────────────────────────

func TestGlobalMiddleware(t *testing.T) {
	r := router.New()

	// Middleware that adds a response header.
	addHeader := func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			ctx.SetHeader("X-Test", "mw-ran")
			next(ctx)
		}
	}
	r.Use(addHeader)
	r.GET("/", textHandler("ok"))

	rec := get(r, "/")
	if rec.Header().Get("X-Test") != "mw-ran" {
		t.Error("global middleware did not run")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Priority: static > param > wildcard
// ─────────────────────────────────────────────────────────────────────────────

func TestMatchPriority(t *testing.T) {
	r := router.New()
	r.GET("/users/me", textHandler("me"))      // static
	r.GET("/users/:id", textHandler("param"))  // param
	r.GET("/users/*rest", textHandler("wild")) // wildcard

	tests := []struct {
		path string
		want string
	}{
		{"/users/me", "me"}, // static wins over param
		{"/users/42", "param"},
		{"/users/some/extra", "wild"},
	}

	for _, tc := range tests {
		rec := get(r, tc.path)
		if rec.Body.String() != tc.want {
			t.Errorf("GET %s: body = %q, want %q", tc.path, rec.Body.String(), tc.want)
		}
	}
}
