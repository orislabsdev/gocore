// Package server provides the HTTP/HTTPS server for the gocore library.
// It wraps Go's net/http.Server and adds:
//
//   - Graceful shutdown on SIGINT/SIGTERM.
//   - TLS support (HTTP/1.1 and HTTP/2 via net/http's built-in ALPN).
//   - Secure TLS defaults (min TLS 1.2, restricted cipher suites).
//   - Configurable timeouts to prevent resource exhaustion.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orislabsdev/gocore/config"
	"github.com/orislabsdev/gocore/logger"
)

// Server wraps net/http.Server with additional lifecycle management.
type Server struct {
	httpServer *http.Server
	cfg        config.Config
	log        *logger.Logger
}

// New creates a Server from the provided configuration and attaches handler
// as the HTTP request handler (typically a *router.Router).
func New(cfg config.Config, handler http.Handler, log *logger.Logger) *Server {
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	s := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}

	// Apply TLS configuration when enabled.
	if cfg.TLS.Enabled {
		s.TLSConfig = buildTLSConfig(cfg.TLS)
		// Default TLS port convention: if using default HTTP port, shift to HTTPS.
		if cfg.Server.Port == 8080 {
			s.Addr = fmt.Sprintf("%s:8443", cfg.Server.Host)
		}
	}

	return &Server{httpServer: s, cfg: cfg, log: log}
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifecycle
// ─────────────────────────────────────────────────────────────────────────────

// ListenAndServe starts the server and blocks until it is shut down.
//
// It installs a signal handler for SIGINT and SIGTERM. When either signal is
// received the server performs a graceful shutdown: it stops accepting new
// connections and waits for in-flight requests to complete (up to
// cfg.Server.ShutdownTimeout).
//
// Returns nil if the server was shut down gracefully, or an error for
// unexpected failures.
func (s *Server) ListenAndServe() error {
	// Channel that receives OS signals.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Channel to receive the result of the serve goroutine.
	serveErr := make(chan error, 1)

	go func() {
		s.log.Info("server starting", "addr", s.httpServer.Addr, "tls", s.cfg.TLS.Enabled)

		var err error
		if s.cfg.TLS.Enabled {
			err = s.httpServer.ListenAndServeTLS(s.cfg.TLS.CertFile, s.cfg.TLS.KeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		// ErrServerClosed is expected during a graceful shutdown — not an error.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		} else {
			close(serveErr)
		}
	}()

	// Block until a shutdown signal or an unexpected serve error.
	select {
	case err := <-serveErr:
		return err
	case sig := <-quit:
		s.log.Info("shutdown signal received", "signal", sig.String())
	}

	return s.Shutdown()
}

// Shutdown gracefully stops the server, waiting up to ShutdownTimeout for
// active connections to finish.
func (s *Server) Shutdown() error {
	timeout := s.cfg.Server.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.log.Info("shutting down server", "timeout", timeout)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.log.Error("shutdown error", "err", err)
		return fmt.Errorf("server shutdown: %w", err)
	}

	s.log.Info("server stopped cleanly")
	return nil
}

// Addr returns the network address the server is configured to listen on.
func (s *Server) Addr() string { return s.httpServer.Addr }

// ─────────────────────────────────────────────────────────────────────────────
// TLS helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildTLSConfig creates a secure *tls.Config from TLSConfig.
// If no cipher suites are specified a hardened default set is used.
func buildTLSConfig(cfg config.TLSConfig) *tls.Config {
	minVersion := cfg.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	ciphers := cfg.CipherSuites
	if len(ciphers) == 0 {
		// OWASP-recommended cipher suites for TLS 1.2.
		// TLS 1.3 suites are automatically used when both sides support it and
		// are not configurable via this field.
		ciphers = []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		}
	}

	tlsCfg := &tls.Config{
		MinVersion:               minVersion,
		CipherSuites:             ciphers,
		ClientAuth:               cfg.ClientAuth,
		PreferServerCipherSuites: true, // server dictates cipher choice
		CurvePreferences: []tls.CurveID{
			tls.X25519, // fastest; prefer over P-256
			tls.CurveP256,
		},
	}

	// Load CA bundle for mutual TLS when ClientCAs is specified.
	if cfg.ClientCAs != "" {
		pool, err := loadCertPool(cfg.ClientCAs)
		if err == nil {
			tlsCfg.ClientCAs = pool
		}
	}

	return tlsCfg
}

// loadCertPool reads a PEM-encoded CA bundle from disk and returns a
// *x509.CertPool.
func loadCertPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("server: read CA bundle %q: %w", path, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("server: failed to parse any certificates from %q", path)
	}

	return pool, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Listener helper (for tests)
// ─────────────────────────────────────────────────────────────────────────────

// TestListener creates a net.Listener on an OS-assigned port. Useful for
// integration tests that need to spin up a real server without port conflicts.
func TestListener() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}
