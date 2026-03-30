// Package config provides all configuration structures for the gocore library.
//
// Configuration is intentionally structured as plain Go structs, making it
// trivial to populate from any source: code, YAML, JSON, environment variables,
// or a secrets manager. No reflection magic — what you set is what runs.
//
// Usage:
//
//	cfg := config.Default()          // start with safe production defaults
//	cfg.Server.Port = 9090           // override only what you need
//	cfg.JWT.Secret = os.Getenv("JWT_SECRET")
//	app := gocore.NewWithConfig(cfg)
package config

import (
	"crypto/tls"
	"net/http"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Root configuration
// ─────────────────────────────────────────────────────────────────────────────

// Config is the root configuration structure for the gocore server.
// Every subsystem has its own dedicated block so configuration stays
// organized and IDE auto-complete works cleanly.
type Config struct {
	// Server holds core HTTP listener settings.
	Server ServerConfig `json:"server" yaml:"server"`

	// TLS holds HTTPS / mutual-TLS settings.
	TLS TLSConfig `json:"tls" yaml:"tls"`

	// CORS holds Cross-Origin Resource Sharing policy.
	CORS CORSConfig `json:"cors" yaml:"cors"`

	// RateLimit holds per-client request throttling settings.
	RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`

	// JWT holds JSON Web Token signing and validation settings.
	JWT JWTConfig `json:"jwt" yaml:"jwt"`

	// Log holds structured logging settings.
	Log LogConfig `json:"log" yaml:"log"`

	// Security holds HTTP security-header settings.
	Security SecurityConfig `json:"security" yaml:"security"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Server
// ─────────────────────────────────────────────────────────────────────────────

// ServerConfig holds low-level HTTP server parameters.
// All timeouts default to conservative values that prevent resource exhaustion
// while remaining comfortable for normal browser and API clients.
type ServerConfig struct {
	// Host is the network address to bind to (default: "127.0.0.1").
	// Use "0.0.0.0" to accept connections from any network interface.
	Host string `json:"host" yaml:"host"`

	// Port is the TCP port to listen on (default: 8080).
	// If TLS is enabled and Port is 8080 the server will listen on 8443 instead
	// unless overridden.
	Port int `json:"port" yaml:"port"`

	// ReadTimeout is the max duration for reading the complete request,
	// including the body (default: 30s).
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout"`

	// WriteTimeout is the max duration before timing out writes of the
	// response (default: 30s).
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`

	// IdleTimeout is the max time to wait for the next keep-alive request
	// (default: 120s).
	IdleTimeout time.Duration `json:"idle_timeout" yaml:"idle_timeout"`

	// ReadHeaderTimeout is the amount of time allowed to read request headers
	// before the connection is aborted (default: 10s).
	// This mitigates Slowloris-style header exhaustion attacks.
	ReadHeaderTimeout time.Duration `json:"read_header_timeout" yaml:"read_header_timeout"`

	// MaxHeaderBytes is the maximum number of bytes the server will read when
	// parsing request headers (default: 1 MiB).
	MaxHeaderBytes int `json:"max_header_bytes" yaml:"max_header_bytes"`

	// ShutdownTimeout is how long to wait for active connections to drain
	// during a graceful shutdown before forcing closure (default: 30s).
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`

	// TrustedProxies is a list of CIDR ranges or IP addresses that are
	// allowed to set X-Forwarded-For / X-Real-IP headers.
	// Requests from untrusted sources have those headers stripped.
	TrustedProxies []string `json:"trusted_proxies" yaml:"trusted_proxies"`
}

// ─────────────────────────────────────────────────────────────────────────────
// TLS
// ─────────────────────────────────────────────────────────────────────────────

// TLSConfig holds TLS/HTTPS configuration.
// When Enabled is true the server will serve HTTPS only; all plain-HTTP
// requests should be redirected by a reverse proxy or a separate redirect
// handler.
type TLSConfig struct {
	// Enabled activates HTTPS. Requires CertFile and KeyFile.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CertFile is the path to the PEM-encoded TLS certificate (chain).
	CertFile string `json:"cert_file" yaml:"cert_file"`

	// KeyFile is the path to the PEM-encoded TLS private key.
	KeyFile string `json:"key_file" yaml:"key_file"`

	// MinVersion is the minimum TLS version the server will accept.
	// Defaults to tls.VersionTLS12. Consider tls.VersionTLS13 for new services.
	MinVersion uint16 `json:"min_version" yaml:"min_version"`

	// CipherSuites is an explicit list of cipher suite IDs. When empty the
	// Go TLS stack selects a secure default set automatically.
	CipherSuites []uint16 `json:"cipher_suites" yaml:"cipher_suites"`

	// ClientAuth controls whether and how client certificates are verified.
	// Use tls.RequireAndVerifyClientCert for mutual TLS (mTLS).
	ClientAuth tls.ClientAuthType `json:"client_auth" yaml:"client_auth"`

	// ClientCAs is the path to a PEM-encoded CA bundle used to verify
	// client certificates when ClientAuth is enabled.
	ClientCAs string `json:"client_cas" yaml:"client_cas"`
}

// ─────────────────────────────────────────────────────────────────────────────
// CORS
// ─────────────────────────────────────────────────────────────────────────────

// CORSConfig holds Cross-Origin Resource Sharing policy settings.
// CORS is enforced at the middleware level and evaluated per request.
type CORSConfig struct {
	// Enabled toggles the CORS middleware globally (default: true).
	Enabled bool `json:"enabled" yaml:"enabled"`

	// AllowedOrigins is the list of origins permitted to access the API.
	// Exact matches and "*" (wildcard) are supported. In production prefer
	// explicit origins over "*" to prevent credential leakage.
	AllowedOrigins []string `json:"allowed_origins" yaml:"allowed_origins"`

	// AllowedMethods lists the HTTP methods the browser is allowed to use.
	AllowedMethods []string `json:"allowed_methods" yaml:"allowed_methods"`

	// AllowedHeaders lists the request headers the browser is allowed to send.
	AllowedHeaders []string `json:"allowed_headers" yaml:"allowed_headers"`

	// ExposedHeaders lists response headers the browser JavaScript is
	// allowed to read.
	ExposedHeaders []string `json:"exposed_headers" yaml:"exposed_headers"`

	// AllowCredentials indicates that cookies and HTTP auth may be included
	// in cross-origin requests. Cannot be used with AllowedOrigins: ["*"].
	AllowCredentials bool `json:"allow_credentials" yaml:"allow_credentials"`

	// MaxAge is the number of seconds a preflight response may be cached
	// (default: 86400 = 24 h). Setting it too high can cause stale policy
	// during deployments.
	MaxAge int `json:"max_age" yaml:"max_age"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Rate Limiting
// ─────────────────────────────────────────────────────────────────────────────

// RateLimitConfig holds per-client request throttling parameters.
// The limiter uses a token-bucket algorithm: clients accumulate tokens at
// RequestsPerSecond and may burst up to Burst tokens at once.
type RateLimitConfig struct {
	// Enabled toggles rate limiting (default: true).
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Provider specifies the rate limiter backend.
	// Valid values: "memory", "redis" (default: "memory")
	Provider string `json:"provider" yaml:"provider"`

	// Redis holds Redis-specific configuration for distributed rate limiting.
	Redis RedisConfig `json:"redis" yaml:"redis"`

	// RequestsPerSecond is the sustained request rate allowed per key
	// (default: 100 rps). A value of 0 means unlimited.
	RequestsPerSecond float64 `json:"requests_per_second" yaml:"requests_per_second"`

	// Burst is the maximum number of requests that may be made instantaneously
	// above the sustained rate (default: 20).
	Burst int `json:"burst" yaml:"burst"`

	// CleanupInterval is how often the limiter sweeps expired client entries
	// (default: 5 min). Lower values use less memory; higher values reduce GC
	// pressure.
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// ClientTTL is how long a client's limiter state is retained after their
	// last seen request (default: 10 min).
	ClientTTL time.Duration `json:"client_ttl" yaml:"client_ttl"`

	// KeyFunc extracts the rate-limit key from the request.
	// Defaults to the client's remote IP. Return an empty string to skip
	// rate limiting for a specific request (e.g., health checks).
	//
	// This field is not serializable; set it programmatically.
	KeyFunc func(r *http.Request) string `json:"-" yaml:"-"`
}

// RedisConfig holds connection parameters for the Redis rate limit provider.
type RedisConfig struct {
	// Address is the Redis host:port (default: "localhost:6379").
	Address string `json:"address" yaml:"address"`

	// Password is the Redis authentication password (default: "").
	Password string `json:"password" yaml:"password"`

	// DB is the Redis database number (default: 0).
	DB int `json:"db" yaml:"db"`

	// DialTimeout is the connection timeout for Redis (default: 5s).
	DialTimeout time.Duration `json:"dial_timeout" yaml:"dial_timeout"`

	// ReadTimeout is the socket read timeout (default: 3s).
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout"`
}

// ─────────────────────────────────────────────────────────────────────────────
// JWT
// ─────────────────────────────────────────────────────────────────────────────

// JWTConfig holds JSON Web Token signing and parsing parameters.
//
// Secrets should always be loaded from an environment variable or secrets
// manager — never hard-coded in source files.
type JWTConfig struct {
	// Secret is the HMAC secret key used to sign and verify tokens.
	// Use at least 32 bytes of cryptographically random data.
	Secret string `json:"secret" yaml:"secret"`

	// Issuer is the "iss" claim value asserted when issuing tokens and
	// required when validating them.
	Issuer string `json:"issuer" yaml:"issuer"`

	// Audience is the "aud" claim value(s). All listed values must appear in
	// a valid token's audience.
	Audience []string `json:"audience" yaml:"audience"`

	// AccessTokenTTL is the lifetime of newly issued access tokens
	// (default: 15 min). Short lifetimes limit the blast radius of leaks.
	AccessTokenTTL time.Duration `json:"access_token_ttl" yaml:"access_token_ttl"`

	// RefreshTokenTTL is the lifetime of refresh tokens (default: 7 days).
	RefreshTokenTTL time.Duration `json:"refresh_token_ttl" yaml:"refresh_token_ttl"`

	// Algorithm is the HMAC signing algorithm (default: "HS256").
	// Accepted values: "HS256", "HS384", "HS512".
	Algorithm string `json:"algorithm" yaml:"algorithm"`

	// TokenLookup defines how the JWT is extracted from an incoming request.
	// Format: "source:name". Multiple lookups are tried in order (comma-sep).
	// Examples:
	//   "header:Authorization"   — Authorization: Bearer <token>
	//   "query:token"            — ?token=<token>
	//   "cookie:jwt"             — Cookie: jwt=<token>
	// Default: "header:Authorization".
	TokenLookup string `json:"token_lookup" yaml:"token_lookup"`

	// AuthScheme is the scheme prefix stripped from the Authorization header
	// before parsing (default: "Bearer").
	AuthScheme string `json:"auth_scheme" yaml:"auth_scheme"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Logging
// ─────────────────────────────────────────────────────────────────────────────

// LogConfig holds structured-logging settings.
type LogConfig struct {
	// Level is the minimum severity to emit (default: "info").
	// Valid values: "debug", "info", "warn", "error".
	Level string `json:"level" yaml:"level"`

	// Format selects the log encoding (default: "json").
	// Valid values: "json", "text".
	Format string `json:"format" yaml:"format"`

	// Output is the log sink (default: "stdout").
	// Valid values: "stdout", "stderr", or an absolute file path.
	Output string `json:"output" yaml:"output"`

	// RequestLog enables the per-request access log (default: true).
	RequestLog bool `json:"request_log" yaml:"request_log"`

	// SkipPaths is a list of URL paths whose requests are not access-logged.
	// Useful for high-frequency health-check endpoints.
	SkipPaths []string `json:"skip_paths" yaml:"skip_paths"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Security Headers
// ─────────────────────────────────────────────────────────────────────────────

// SecurityConfig holds HTTP security-header settings applied by the security
// middleware. All headers are standards-based defences against common browser
// attacks (XSS, clickjacking, MIME sniffing, etc.).
type SecurityConfig struct {
	// Enabled toggles the security-headers middleware (default: true).
	Enabled bool `json:"enabled" yaml:"enabled"`

	// HSTSMaxAge is the Strict-Transport-Security max-age in seconds.
	// Set to 0 to omit the HSTS header (not recommended for TLS services).
	// Default: 31 536 000 (1 year).
	HSTSMaxAge int `json:"hsts_max_age" yaml:"hsts_max_age"`

	// HSTSIncludeSubdomains adds the includeSubDomains directive to HSTS.
	HSTSIncludeSubdomains bool `json:"hsts_include_subdomains" yaml:"hsts_include_subdomains"`

	// HSTSPreload adds the preload directive so the domain may be submitted
	// to browsers' HSTS preload lists.
	HSTSPreload bool `json:"hsts_preload" yaml:"hsts_preload"`

	// ContentSecurityPolicy is the Content-Security-Policy header value.
	// An empty string omits the header. Craft a tight policy for your app.
	ContentSecurityPolicy string `json:"content_security_policy" yaml:"content_security_policy"`

	// XFrameOptions is the X-Frame-Options header value (default: "DENY").
	// Defends against clickjacking. Accepted: "DENY", "SAMEORIGIN".
	XFrameOptions string `json:"x_frame_options" yaml:"x_frame_options"`

	// XContentTypeOptions enables X-Content-Type-Options: nosniff (default: true).
	// Prevents browsers from MIME-sniffing responses away from the declared type.
	XContentTypeOptions bool `json:"x_content_type_options" yaml:"x_content_type_options"`

	// ReferrerPolicy is the Referrer-Policy header value.
	// Default: "strict-origin-when-cross-origin".
	ReferrerPolicy string `json:"referrer_policy" yaml:"referrer_policy"`

	// PermissionsPolicy is the Permissions-Policy header value.
	// An empty string omits the header.
	PermissionsPolicy string `json:"permissions_policy" yaml:"permissions_policy"`

	// XXSSProtection enables the legacy X-XSS-Protection: 1; mode=block header.
	// Modern browsers use CSP instead; this header is for older browser compat.
	XXSSProtection bool `json:"x_xss_protection" yaml:"x_xss_protection"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Default constructor
// ─────────────────────────────────────────────────────────────────────────────

// Default returns a *Config populated with conservative, production-safe
// defaults. Override only the fields that differ from these values.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:              "127.0.0.1",
			Port:              8080,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1 MiB
			ShutdownTimeout:   30 * time.Second,
		},
		TLS: TLSConfig{
			MinVersion: tls.VersionTLS12,
		},
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{}, // empty by default - force explicit config
			AllowedMethods: []string{
				http.MethodGet, http.MethodPost, http.MethodPut,
				http.MethodPatch, http.MethodDelete,
				http.MethodOptions, http.MethodHead,
			},
			AllowedHeaders: []string{
				"Accept", "Authorization", "Content-Type",
				"X-Request-ID", "X-Correlation-ID",
			},
			MaxAge: 86400,
		},
		RateLimit: RateLimitConfig{
			Enabled:           true,
			Provider:          "memory",
			RequestsPerSecond: 100,
			Burst:             20,
			CleanupInterval:   5 * time.Minute,
			ClientTTL:         10 * time.Minute,
			Redis: RedisConfig{
				Address:     "localhost:6379",
				DialTimeout: 5 * time.Second,
				ReadTimeout: 3 * time.Second,
			},
		},
		JWT: JWTConfig{
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Algorithm:       "HS256",
			TokenLookup:     "header:Authorization",
			AuthScheme:      "Bearer",
		},
		Log: LogConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			RequestLog: true,
		},
		Security: SecurityConfig{
			Enabled:               true,
			HSTSMaxAge:            31_536_000,
			HSTSIncludeSubdomains: true,
			XFrameOptions:         "DENY",
			XContentTypeOptions:   true,
			ReferrerPolicy:        "strict-origin-when-cross-origin",
			XXSSProtection:        true,
		},
	}
}
