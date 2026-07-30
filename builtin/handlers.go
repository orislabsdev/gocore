// Package builtin provides ready-made handlers for common operational endpoints
// that every backend service should expose.
//
// Included handlers:
//
//   - HealthCheck — a simple liveness probe that always returns 200 OK.
//   - ReadyCheck  — a readiness probe that delegates to user-supplied checks.
//   - NotFound    — a JSON 404 handler consistent with the standard envelope.
//
// Typical registration:
//
//	app.GET("/health",  builtin.HealthCheck()).Public()
//	app.GET("/ready",   builtin.ReadyCheck(db.Ping, cache.Ping)).Public()
package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/orislabsdev/gocore/handler"
)

// ─────────────────────────────────────────────────────────────────────────────
// Health check
// ─────────────────────────────────────────────────────────────────────────────

// healthResponse is the JSON payload returned by the health endpoint.
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// HealthCheck returns a handler that always responds 200 OK with
// {"status":"ok"}. This endpoint is suitable for liveness probes (e.g.,
// Kubernetes livenessProbe) which only need to confirm the process is alive.
//
// Mark this route as .Public() so it is reachable without authentication.
func HealthCheck() handler.HandlerFunc {
	return func(ctx *handler.Context) {
		ctx.JSON(http.StatusOK, healthResponse{Status: "ok"})
	}
}

// VersionedHealthCheck is like HealthCheck but also reports the application
// version string. The version is baked in at startup so there is no overhead.
func VersionedHealthCheck(version string) handler.HandlerFunc {
	return func(ctx *handler.Context) {
		ctx.JSON(http.StatusOK, healthResponse{Status: "ok", Version: version})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Readiness check
// ─────────────────────────────────────────────────────────────────────────────

// CheckFunc is a function that verifies the availability of a dependency.
// It should return nil when the dependency is healthy, or a descriptive error
// when it is not. The context should be respected so checks can be cancelled.
type CheckFunc func(ctx context.Context) error

// checkResult is the per-dependency result included in the ready response.
type checkResult struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// readyResponse is the JSON payload returned by the readiness endpoint.
type readyResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks,omitempty"`
}

// ReadyCheck returns a handler that runs all supplied named dependency checks
// concurrently and returns 200 OK only if every check passes. If any check
// fails the response is 503 Service Unavailable.
//
// Use this for Kubernetes readinessProbe / startupProbe endpoints.
//
//	app.GET("/ready", builtin.ReadyCheck(
//	    "database", db.PingContext,
//	    "cache",    cache.PingContext,
//	)).Public()
//
// The arguments alternate: name, CheckFunc, name, CheckFunc, …
func ReadyCheck(nameChecks ...any) handler.HandlerFunc {
	type namedCheck struct {
		name string
		fn   CheckFunc
	}

	// Parse the variadic name-check pairs at registration time.
	checks := make([]namedCheck, 0, len(nameChecks)/2)
	for i := 0; i+1 < len(nameChecks); i += 2 {
		name, ok1 := nameChecks[i].(string)
		fn, ok2 := nameChecks[i+1].(CheckFunc)
		if ok1 && ok2 {
			checks = append(checks, namedCheck{name, fn})
		}
	}

	return func(ctx *handler.Context) {
		// Run all checks with a 5-second deadline.
		deadline, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
		defer cancel()

		var (
			mu      sync.Mutex
			results = make(map[string]checkResult, len(checks))
			wg      sync.WaitGroup
			allOK   = true
		)

		for _, c := range checks {
			wg.Add(1)
			go func(nc namedCheck) {
				defer wg.Done()

				start := time.Now()
				err := nc.fn(deadline)
				elapsed := time.Since(start)

				var r checkResult
				if err != nil {
					r = checkResult{
						Status:  "fail",
						Error:   err.Error(),
						Latency: fmt.Sprintf("%dms", elapsed.Milliseconds()),
					}
					mu.Lock()
					allOK = false
					mu.Unlock()
				} else {
					r = checkResult{
						Status:  "pass",
						Latency: fmt.Sprintf("%dms", elapsed.Milliseconds()),
					}
				}

				mu.Lock()
				results[nc.name] = r
				mu.Unlock()
			}(c)
		}

		wg.Wait()

		status := "ok"
		code := http.StatusOK
		if !allOK {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}

		ctx.JSON(code, readyResponse{Status: status, Checks: results})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Metrics (basic runtime stats)
// ─────────────────────────────────────────────────────────────────────────────

// metricsResponse is the JSON payload returned by the metrics endpoint.
type metricsResponse struct {
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_alloc_mb"`
	HeapSysMB   float64 `json:"heap_sys_mb"`
	GCCycles    uint32  `json:"gc_cycles"`
	Uptime      string  `json:"uptime"`
}

// Metrics returns a handler that exposes basic Go runtime statistics.
// This is a lightweight alternative for services that do not use Prometheus.
//
// The endpoint should be protected — either mark it as .Private() (JWT
// required) or place it behind a separate internal listener.
func Metrics(startTime time.Time) handler.HandlerFunc {
	return func(ctx *handler.Context) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		ctx.JSON(http.StatusOK, metricsResponse{
			Goroutines:  runtime.NumGoroutine(),
			HeapAllocMB: float64(ms.HeapAlloc) / 1024 / 1024,
			HeapSysMB:   float64(ms.HeapSys) / 1024 / 1024,
			GCCycles:    ms.NumGC,
			Uptime:      time.Since(startTime).Truncate(time.Second).String(),
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// File serving (API-oriented)
// ─────────────────────────────────────────────────────────────────────────────

// fileConfig holds the configuration for file serving handlers.
type fileConfig struct {
	cacheMaxAge  int
	denyDotFiles bool
	download     bool
	notFound     handler.HandlerFunc
}

// FileOption configures a file serving handler using the functional options
// pattern. This keeps the common case simple while allowing customization:
//
//	builtin.ServeDir("./uploads")                                    // simple
//	builtin.ServeDir("./uploads", builtin.WithFileCache(3600))       // con caché
//	builtin.ServeDir("./docs", builtin.WithFileDownload(true))       // forzar descarga
type FileOption func(*fileConfig)

func defaultFileConfig() fileConfig {
	return fileConfig{
		denyDotFiles: true, // Security by default
	}
}

// WithFileCache sets the Cache-Control max-age directive in seconds.
//
// Use cases:
//   - 0 (default): no Cache-Control header, browser decides.
//   - 3600: uploads que cambian ocasionalmente.
//   - 31536000: SOLO si los filenames tienen hash (img-3a7b2c.png).
func WithFileCache(seconds int) FileOption {
	return func(c *fileConfig) { c.cacheMaxAge = seconds }
}

// WithFileDenyDot controls whether dotfiles are blocked.
// Defaults to true — files like .env, .gitignore, .htaccess are denied.
//
// Disable only if you explicitly need to serve hidden files:
//
//	builtin.ServeDir("./data", builtin.WithFileDenyDot(false))
func WithFileDenyDot(enabled bool) FileOption {
	return func(c *fileConfig) { c.denyDotFiles = enabled }
}

// WithFileDownload forces the browser to download the file instead of
// displaying it. This sets Content-Disposition: attachment; filename="..."
//
// Use this for documents, reports, and exports:
//
//	app.GET("/api/reports/*filepath", builtin.ServeDir("./reports", builtin.WithFileDownload(true)))
func WithFileDownload(enabled bool) FileOption {
	return func(c *fileConfig) { c.download = enabled }
}

// WithFileNotFound sets a custom handler for when a file is not found.
// Without this, the handler calls ctx.NotFound("") which returns a JSON 404
// consistent with the framework's envelope format.
func WithFileNotFound(h handler.HandlerFunc) FileOption {
	return func(c *fileConfig) { c.notFound = h }
}

// ServeDir returns a handler that serves files from a directory on disk.
//
// It expects to be registered with a wildcard route parameter named "filepath"
// that captures the path within the directory:
//
//	app.GET("/api/images/*filepath", builtin.ServeDir("./uploads"))
//
// For a more convenient API, use Core.Static():
//
//	app.Static("/api/images", "./uploads")
//
// How it works:
//  1. Extracts the "filepath" parameter from the router.
//  2. Cleans the path to prevent directory traversal.
//  3. Blocks dotfiles (.env, .git, etc.) by default.
//  4. Serves the file using the standard library (handles Content-Type,
//     ETag, Range requests, Last-Modified).
//  5. Returns a JSON 404 if the file doesn't exist.
func ServeDir(dir string, opts ...FileOption) handler.HandlerFunc {
	return ServeFS(os.DirFS(dir), opts...)
}

// ServeFS returns a handler that serves files from an fs.FS.
//
// Use this when you need to serve files from an embedded filesystem or a
// custom fs.FS implementation:
//
//	//go:embed assets
//	var assetFS embed.FS
//
//	app.GET("/api/assets/*filepath", builtin.ServeFS(assetFS))
func ServeFS(root fs.FS, opts ...FileOption) handler.HandlerFunc {
	cfg := defaultFileConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Validate the FS at registration time — fail fast.
	// A misconfigured directory should panic at startup, not at request time.
	if _, err := fs.Stat(root, "."); err != nil {
		panic("builtin: invalid file directory: " + err.Error())
	}

	// Create the standard file server once at registration time.
	// http.FileServer is safe for concurrent use after creation.
	fsrv := http.FileServer(http.FS(root))

	return func(ctx *handler.Context) {
		// ── Extract the file path from the wildcard parameter ──
		filePath := ctx.Param("filepath")
		if filePath == "" {
			filePath = "/"
		}

		// ── Clean and normalize the path ──
		// path.Clean removes ".", "..", and double slashes.
		// Prepending "/" ensures absolute path normalization.
		// fs.FS then validates that the resulting path doesn't escape root.
		filePath = path.Clean("/" + filePath)

		// Convert to FS-relative name (fs.FS uses relative paths).
		name := strings.TrimPrefix(filePath, "/")
		if name == "" {
			// Root path requested — no file to serve.
			fileNotFound(ctx, cfg)
			return
		}

		// ── Security: block dotfiles ──
		if cfg.denyDotFiles && hasDotFile(name) {
			fileNotFound(ctx, cfg)
			return
		}

		// ── Check if the file exists ──
		stat, err := fs.Stat(root, name)
		if err != nil {
			if os.IsNotExist(err) {
				fileNotFound(ctx, cfg)
				return
			}
			ctx.Fail(http.StatusInternalServerError, "INTERNAL_ERROR",
				"unable to access file", nil)
			return
		}

		// Directories are not served — this is an API, not a file browser.
		if stat.IsDir() {
			fileNotFound(ctx, cfg)
			return
		}

		// ── Set response headers ──
		if cfg.cacheMaxAge > 0 {
			ctx.ResponseWriter().Header().Set("Cache-Control",
				fmt.Sprintf("public, max-age=%d", cfg.cacheMaxAge))
		}

		if cfg.download {
			// Force the browser to download instead of displaying.
			// RFC 6266: Content-Disposition header.
			ctx.ResponseWriter().Header().Set("Content-Disposition",
				fmt.Sprintf(`attachment; filename="%s"`, stat.Name()))
		}

		// ── Serve the file ──
		// Clone the request to avoid mutating the original.
		// The standard file server reads the URL path to determine which
		// file to serve, so we set it to the file name.
		req := ctx.Request.Clone(ctx.Request.Context())
		req.URL.Path = "/" + name
		fsrv.ServeHTTP(ctx.ResponseWriter(), req)
	}
}

// fileNotFound handles the case when a requested file doesn't exist.
func fileNotFound(ctx *handler.Context, cfg fileConfig) {
	if cfg.notFound != nil {
		cfg.notFound(ctx)
		return
	}
	ctx.NotFound("")
}

// hasDotFile checks if the path contains any dotfile or dot directory.
//
//	hasDotFile(".env")          → true
//	hasDotFile("images/.thumb") → true
//	hasDotFile(".git/config")   → true
//	hasDotFile("images/logo")   → false
func hasDotFile(name string) bool {
	parts := strings.SplitSeq(name, "/")
	for part := range parts {
		if part != "" && strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}
