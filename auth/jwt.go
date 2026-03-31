// Package auth provides JSON Web Token (JWT) generation and validation
// utilities used by the gocore authentication middleware.
//
// Tokens are HMAC-signed (HS256/HS384/HS512). The package wraps
// github.com/golang-jwt/jwt/v5 and adds opinionated helpers for common
// patterns: issuing access/refresh token pairs, extracting claims, and
// validating standard registered fields.
//
// Usage:
//
//	mgr := auth.NewManager(cfg.JWT)
//
//	// Issue an access token
//	token, err := mgr.IssueAccessToken("user-123", []string{"admin", "editor"}, nil)
//
//	// Validate a token and extract claims
//	claims, err := mgr.ValidateToken(token)
//	fmt.Println(claims.Subject, claims.Roles)
package auth

import (
	"errors"
	"fmt"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/orislabsdev/gocore/config"
)

// ─────────────────────────────────────────────────────────────────────────────
// Errors
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrTokenExpired is returned when a token's expiry has passed.
	ErrTokenExpired = errors.New("token expired")

	// ErrTokenInvalid is returned for malformed or tampered tokens.
	ErrTokenInvalid = errors.New("token invalid")

	// ErrTokenMissingClaims is returned when required claims are absent.
	ErrTokenMissingClaims = errors.New("token missing required claims")

	// ErrEmptySecret is returned when the JWT secret has not been configured.
	ErrEmptySecret = errors.New("jwt secret must not be empty")
)

// ─────────────────────────────────────────────────────────────────────────────
// Claims
// ─────────────────────────────────────────────────────────────────────────────

// TokenType distinguishes access tokens from refresh tokens.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims extends jwt.RegisteredClaims with application-specific fields.
// All fields are safe to include in the token payload — never put secrets here.
type Claims struct {
	jwt.RegisteredClaims

	// Roles is the list of roles/permissions granted to the subject.
	// e.g. []string{"admin", "billing:read"}
	Roles []string `json:"roles,omitempty"`

	// TokenType distinguishes access from refresh tokens.
	TokenType TokenType `json:"token_type,omitempty"`

	// Extra is a freeform map for application-specific claims.
	// Keep values small; JWT payloads travel in every request header.
	Extra map[string]any `json:"extra,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Manager
// ─────────────────────────────────────────────────────────────────────────────

// Manager handles JWT creation and validation for a single configuration.
// Construct one at startup and share it across the application.
type Manager struct {
	cfg    config.JWTConfig
	method jwt.SigningMethod
	key    []byte // HMAC key derived from cfg.Secret
}

// NewManager creates a Manager from a JWTConfig.
// Returns an error if the secret is empty or the algorithm is unsupported.
func NewManager(cfg config.JWTConfig) (*Manager, error) {
	if cfg.Secret == "" {
		return nil, ErrEmptySecret
	}

	var method jwt.SigningMethod
	switch cfg.Algorithm {
	case "HS256", "": // default to HS256 if empty
		method = jwt.SigningMethodHS256
	case "HS384":
		method = jwt.SigningMethodHS384
	case "HS512":
		method = jwt.SigningMethodHS512
	default:
		return nil, fmt.Errorf("auth: unsupported signing algorithm %q", cfg.Algorithm)
	}

	return &Manager{
		cfg:    cfg,
		method: method,
		key:    []byte(cfg.Secret),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Token issuance
// ─────────────────────────────────────────────────────────────────────────────

// IssueAccessToken mints a short-lived access token for the given subject.
//
//   - subject  — stable, unique user identifier (e.g., UUID or user ID).
//   - roles    — optional list of role strings embedded in the token.
//   - extra    — optional map of additional claims. Pass nil if not needed.
func (m *Manager) IssueAccessToken(subject string, roles []string, extra map[string]any) (string, error) {
	return m.issue(subject, roles, extra, TokenTypeAccess, m.cfg.AccessTokenTTL)
}

// IssueRefreshToken mints a long-lived refresh token for the given subject.
// Refresh tokens should only be used to obtain new access tokens; they must
// not grant access to API resources directly.
func (m *Manager) IssueRefreshToken(subject string) (string, error) {
	return m.issue(subject, nil, nil, TokenTypeRefresh, m.cfg.RefreshTokenTTL)
}

// issue is the internal token factory.
func (m *Manager) issue(
	subject string,
	roles []string,
	extra map[string]any,
	ttype TokenType,
	ttl time.Duration,
) (string, error) {
	now := time.Now()

	registered := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	if m.cfg.Issuer != "" {
		registered.Issuer = m.cfg.Issuer
	}
	if len(m.cfg.Audience) > 0 {
		registered.Audience = m.cfg.Audience
	}

	claims := &Claims{
		RegisteredClaims: registered,
		Roles:            roles,
		TokenType:        ttype,
		Extra:            extra,
	}

	token := jwt.NewWithClaims(m.method, claims)
	signed, err := token.SignedString(m.key)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Token validation
// ─────────────────────────────────────────────────────────────────────────────

// ValidateToken parses and validates a signed token string.
// On success the decoded *Claims are returned. On failure, a typed error is
// returned so callers can decide how to respond (e.g., refresh vs re-login).
func (m *Manager) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		m.keyFunc,
		// Only accept the algorithm we configured — prevent algorithm confusion.
		jwt.WithValidMethods([]string{m.method.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(m.cfg.Leeway),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	// Enforce issuer and audience when configured.
	if m.cfg.Issuer != "" && claims.Issuer != m.cfg.Issuer {
		return nil, fmt.Errorf("%w: issuer mismatch", ErrTokenInvalid)
	}

	return claims, nil
}

// ValidateAccessToken is a stricter variant that also rejects refresh tokens
// so they cannot be used in place of access tokens.
func (m *Manager) ValidateAccessToken(tokenStr string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, fmt.Errorf("%w: expected access token, got %s", ErrTokenInvalid, claims.TokenType)
	}
	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns its claims.
// Call this before issuing a new access token in a refresh flow.
func (m *Manager) ValidateRefreshToken(tokenStr string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, fmt.Errorf("%w: expected refresh token, got %s", ErrTokenInvalid, claims.TokenType)
	}
	return claims, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// keyFunc is the jwt.Keyfunc that returns the HMAC secret after confirming the
// signing algorithm matches what we expect.
func (m *Manager) keyFunc(token *jwt.Token) (any, error) {
	if token.Method.Alg() != m.method.Alg() {
		return nil, fmt.Errorf("auth: unexpected signing algorithm %q", token.Header["alg"])
	}
	return m.key, nil
}

// HasRole reports whether at least one of the provided roles appears in the
// claims. Useful in route handlers and policy checks.
func HasRole(claims *Claims, roles ...string) bool {
	if claims == nil {
		return false
	}
	set := make(map[string]struct{}, len(claims.Roles))
	for _, r := range claims.Roles {
		set[r] = struct{}{}
	}
	for _, r := range roles {
		if _, ok := set[r]; ok {
			return true
		}
	}
	return false
}
