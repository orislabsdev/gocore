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
	"net/http"
	"runtime"
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
