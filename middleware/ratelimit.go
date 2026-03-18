package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/handler"
)

// ─────────────────────────────────────────────────────────────────────────────
// Per-client limiter entry
// ─────────────────────────────────────────────────────────────────────────────

// clientLimiter pairs a token-bucket rate limiter with the last time the
// client made a request. The timestamp is used to evict stale entries from
// the store during cleanup sweeps.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ─────────────────────────────────────────────────────────────────────────────
// Limiter store
// ─────────────────────────────────────────────────────────────────────────────

// limiterStore holds a map of client keys to their per-client limiters.
// A background goroutine periodically evicts entries that have not been seen
// for longer than the configured ClientTTL.
type limiterStore struct {
	mu      sync.Mutex
	clients map[string]*clientLimiter
	cfg     config.RateLimitConfig
}

// newLimiterStore creates a store and starts its background cleanup goroutine.
// The goroutine exits when ctx (passed via done channel) is closed.
func newLimiterStore(cfg config.RateLimitConfig, done <-chan struct{}) *limiterStore {
	s := &limiterStore{
		clients: make(map[string]*clientLimiter),
		cfg:     cfg,
	}
	go s.cleanupLoop(done)
	return s
}

// get returns the rate limiter for the given key, creating one if necessary.
func (s *limiterStore) get(key string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.clients[key]
	if !ok {
		lim := rate.NewLimiter(rate.Limit(s.cfg.RequestsPerSecond), s.cfg.Burst)
		entry = &clientLimiter{limiter: lim}
		s.clients[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// cleanupLoop runs in a goroutine and periodically removes stale client
// entries to bound memory usage. The loop exits when done is closed.
func (s *limiterStore) cleanupLoop(done <-chan struct{}) {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.evictStale()
		case <-done:
			return
		}
	}
}

// evictStale removes entries that have not been seen for longer than ClientTTL.
func (s *limiterStore) evictStale() {
	threshold := time.Now().Add(-s.cfg.ClientTTL)

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, entry := range s.clients {
		if entry.lastSeen.Before(threshold) {
			delete(s.clients, key)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Middleware
// ─────────────────────────────────────────────────────────────────────────────

// RateLimit returns a middleware that enforces per-client request rate limits
// using a token-bucket algorithm.
//
// By default the client is identified by their IP address. Supply a custom
// KeyFunc in the config to key by authenticated user ID, API key, or any
// other dimension.
//
// When a client exceeds the configured rate, the middleware responds with
// HTTP 429 Too Many Requests and sets the Retry-After header. The request
// chain is aborted (the actual handler is not called).
//
// The done channel should be closed when the server shuts down to stop the
// background cleanup goroutine:
//
//	done := make(chan struct{})
//	app.Use(middleware.RateLimit(cfg.RateLimit, done))
//	// on shutdown:
//	close(done)
func RateLimit(cfg config.RateLimitConfig, done <-chan struct{}) handler.MiddlewareFunc {
	if done == nil {
		// Provide a never-closed channel so the store can still start safely.
		done = make(chan struct{})
	}

	store := newLimiterStore(cfg, done)

	// Default key function: use the client's remote IP.
	keyFunc := cfg.KeyFunc
	if keyFunc == nil {
		keyFunc = defaultRateLimitKey
	}

	return func(next handler.HandlerFunc) handler.HandlerFunc {
		return func(ctx *handler.Context) {
			key := keyFunc(ctx.Request)
			if key == "" {
				// Empty key means "skip rate limiting for this request."
				next(ctx)
				return
			}

			lim := store.get(key)
			if !lim.Allow() {
				// Inform the client how many seconds to wait before retrying.
				// We use a fixed 1-second estimate here; a precise calculation
				// would require exposing rate.Reservation, which adds complexity
				// without meaningful benefit for most use cases.
				ctx.SetHeader("Retry-After", "1")
				ctx.TooManyRequests()
				return
			}

			next(ctx)
		}
	}
}

// defaultRateLimitKey extracts the client IP from the request.
// It respects X-Real-IP and X-Forwarded-For headers when present.
func defaultRateLimitKey(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list; use the leftmost value.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to the raw RemoteAddr (host:port). Strip the port.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
