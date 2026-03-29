package middleware

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/orislabsdev/gocore/config"
)

// clientLimiter pairs a token-bucket rate limiter with the last time the
// client made a request. The timestamp is used to evict stale entries from
// the store during cleanup sweeps.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// memoryLimiterBackend holds a map of client keys to their per-client limiters.
// A background goroutine periodically evicts entries that have not been seen
// for longer than the configured ClientTTL.
type memoryLimiterBackend struct {
	mu      sync.Mutex
	clients map[string]*clientLimiter
	cfg     config.RateLimitConfig
}

// newMemoryLimiterBackend creates a store and starts its background cleanup goroutine.
// The goroutine exits when ctx (passed via done channel) is closed.
func newMemoryLimiterBackend(cfg config.RateLimitConfig, done <-chan struct{}) *memoryLimiterBackend {
	s := &memoryLimiterBackend{
		clients: make(map[string]*clientLimiter),
		cfg:     cfg,
	}
	go s.cleanupLoop(done)
	return s
}

func (s *memoryLimiterBackend) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.clients[key]
	if !ok {
		lim := rate.NewLimiter(rate.Limit(s.cfg.RequestsPerSecond), s.cfg.Burst)
		entry = &clientLimiter{limiter: lim}
		s.clients[key] = entry
	}
	entry.lastSeen = time.Now()

	allowed := entry.limiter.Allow()
	var retryAfter time.Duration
	if !allowed {
		retryAfter = time.Second // Simple 1-second delay for memory backend
	}
	return allowed, retryAfter, nil
}

// cleanupLoop runs in a goroutine and periodically removes stale client
// entries to bound memory usage. The loop exits when done is closed.
func (s *memoryLimiterBackend) cleanupLoop(done <-chan struct{}) {
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
func (s *memoryLimiterBackend) evictStale() {
	threshold := time.Now().Add(-s.cfg.ClientTTL)

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, entry := range s.clients {
		if entry.lastSeen.Before(threshold) {
			delete(s.clients, key)
		}
	}
}
