package auth_test

import (
	"testing"
	"time"

	"github.com/orislabsdev/gocore/auth"
	"github.com/orislabsdev/gocore/config"
)

// testConfig returns a JWTConfig suitable for unit tests.
func testConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:          "test-secret-must-be-at-least-32-bytes!",
		Issuer:          "test-issuer",
		Audience:        []string{"test-audience"},
		Algorithm:       "HS256",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		TokenLookup:     "header:Authorization",
		AuthScheme:      "Bearer",
	}
}

func TestNewManager_EmptySecret(t *testing.T) {
	cfg := testConfig()
	cfg.Secret = ""
	_, err := auth.NewManager(cfg)
	if err == nil {
		t.Fatal("expected error for empty secret, got nil")
	}
	if err != auth.ErrEmptySecret {
		t.Fatalf("expected ErrEmptySecret, got %v", err)
	}
}

func TestIssueAndValidateAccessToken(t *testing.T) {
	mgr, err := auth.NewManager(testConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	token, err := mgr.IssueAccessToken("user-42", []string{"admin", "editor"}, map[string]any{
		"email": "test@example.com",
	})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := mgr.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.Subject != "user-42" {
		t.Errorf("subject: got %q, want %q", claims.Subject, "user-42")
	}
	if claims.TokenType != auth.TokenTypeAccess {
		t.Errorf("token type: got %q, want %q", claims.TokenType, auth.TokenTypeAccess)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "admin" {
		t.Errorf("roles: got %v, want [admin editor]", claims.Roles)
	}
}

func TestIssueAndValidateRefreshToken(t *testing.T) {
	mgr, err := auth.NewManager(testConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	token, err := mgr.IssueRefreshToken("user-99")
	if err != nil {
		t.Fatalf("IssueRefreshToken: %v", err)
	}

	claims, err := mgr.ValidateRefreshToken(token)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if claims.TokenType != auth.TokenTypeRefresh {
		t.Errorf("expected refresh token type, got %q", claims.TokenType)
	}
}

func TestValidateAccessToken_RejectsRefreshToken(t *testing.T) {
	mgr, err := auth.NewManager(testConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	refresh, _ := mgr.IssueRefreshToken("user-1")
	_, err = mgr.ValidateAccessToken(refresh)
	if err == nil {
		t.Fatal("expected error when using refresh token as access token")
	}
}

func TestValidateToken_Tampered(t *testing.T) {
	mgr, err := auth.NewManager(testConfig())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	token, _ := mgr.IssueAccessToken("user-1", nil, nil)
	// Corrupt the last byte of the signature.
	tampered := token[:len(token)-1] + "X"

	_, err = mgr.ValidateAccessToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestValidateToken_WrongAlgorithm(t *testing.T) {
	// Create a manager with one config; sign with another config's key.
	mgr1, _ := auth.NewManager(testConfig())

	cfg2 := testConfig()
	cfg2.Secret = "completely-different-secret-key-!!!!"
	mgr2, _ := auth.NewManager(cfg2)

	token, _ := mgr1.IssueAccessToken("user-1", nil, nil)
	_, err := mgr2.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong key")
	}
}

func TestHasRole(t *testing.T) {
	claims := &auth.Claims{Roles: []string{"admin", "editor"}}

	tests := []struct {
		roles []string
		want  bool
	}{
		{[]string{"admin"}, true},
		{[]string{"editor"}, true},
		{[]string{"admin", "viewer"}, true},
		{[]string{"viewer"}, false},
		{[]string{}, false},
	}

	for _, tc := range tests {
		got := auth.HasRole(claims, tc.roles...)
		if got != tc.want {
			t.Errorf("HasRole(%v) = %v, want %v", tc.roles, got, tc.want)
		}
	}
}

func TestHasRole_NilClaims(t *testing.T) {
	if auth.HasRole(nil, "admin") {
		t.Error("HasRole(nil) should return false")
	}
}
