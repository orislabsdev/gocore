package validate_test

import (
	"regexp"
	"testing"

	"github.com/orislabsdev/gocore/validate"
)

func TestRequired(t *testing.T) {
	t.Run("passes for non-empty value", func(t *testing.T) {
		v := validate.New()
		v.Required("name", "Alice")
		if v.HasErrors() {
			t.Errorf("unexpected errors: %v", v.Errors())
		}
	})

	t.Run("fails for empty string", func(t *testing.T) {
		v := validate.New()
		v.Required("name", "")
		if !v.HasErrors() {
			t.Error("expected error for empty field")
		}
	})

	t.Run("fails for whitespace-only value", func(t *testing.T) {
		v := validate.New()
		v.Required("name", "   ")
		if !v.HasErrors() {
			t.Error("expected error for whitespace-only value")
		}
	})
}

func TestEmail(t *testing.T) {
	valid := []string{"user@example.com", "user+tag@sub.example.org"}
	invalid := []string{"notanemail", "@nodomain.com", "missing@", "spaces in@email.com"}

	for _, e := range valid {
		v := validate.New()
		v.Email("email", e)
		if v.HasErrors() {
			t.Errorf("Email(%q) should pass but got errors: %v", e, v.Errors())
		}
	}
	for _, e := range invalid {
		v := validate.New()
		v.Email("email", e)
		if !v.HasErrors() {
			t.Errorf("Email(%q) should fail but passed", e)
		}
	}
}

func TestMinLen(t *testing.T) {
	v := validate.New()
	v.MinLen("password", "short", 8)
	if !v.HasErrors() {
		t.Error("expected error for short password")
	}

	v2 := validate.New()
	v2.MinLen("password", "longenough", 8)
	if v2.HasErrors() {
		t.Errorf("unexpected error: %v", v2.Errors())
	}
}

func TestMaxLen(t *testing.T) {
	v := validate.New()
	v.MaxLen("bio", "this is way too long for a bio field value", 10)
	if !v.HasErrors() {
		t.Error("expected error for too-long value")
	}
}

func TestRange(t *testing.T) {
	v := validate.New()
	v.Range("age", 15, 18, 120)
	if !v.HasErrors() {
		t.Error("expected error for age below minimum")
	}

	v2 := validate.New()
	v2.Range("age", 25, 18, 120)
	if v2.HasErrors() {
		t.Errorf("unexpected error: %v", v2.Errors())
	}
}

func TestOneOf(t *testing.T) {
	v := validate.New()
	v.OneOf("status", "archived", "active", "inactive")
	if !v.HasErrors() {
		t.Error("expected error for value not in set")
	}

	v2 := validate.New()
	v2.OneOf("status", "active", "active", "inactive")
	if v2.HasErrors() {
		t.Errorf("unexpected error: %v", v2.Errors())
	}
}

func TestMatches(t *testing.T) {
	alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]+$`)

	v := validate.New()
	v.Matches("username", "alice_123", alphanumeric)
	if !v.HasErrors() {
		t.Error("expected error for username with underscore")
	}

	v2 := validate.New()
	v2.Matches("username", "alice123", alphanumeric)
	if v2.HasErrors() {
		t.Errorf("unexpected error: %v", v2.Errors())
	}
}

func TestCustom(t *testing.T) {
	reserved := func(s string) string {
		if s == "admin" {
			return "username is reserved"
		}
		return ""
	}

	v := validate.New()
	v.Custom("username", "admin", reserved)
	if !v.HasErrors() {
		t.Error("expected error for reserved username")
	}
	if v.First("username") != "username is reserved" {
		t.Errorf("unexpected message: %q", v.First("username"))
	}

	v2 := validate.New()
	v2.Custom("username", "alice", reserved)
	if v2.HasErrors() {
		t.Errorf("unexpected error: %v", v2.Errors())
	}
}

func TestMultipleErrors(t *testing.T) {
	v := validate.New()
	v.Required("email", "")
	v.Email("email", "")
	v.MinLen("password", "ab", 8)

	errs := v.Errors()
	if len(errs["email"]) < 1 {
		t.Error("expected at least one email error")
	}
	if len(errs["password"]) < 1 {
		t.Error("expected at least one password error")
	}
}

func TestChaining(t *testing.T) {
	// All rules are chainable; ensure no panic.
	v := validate.New()
	v.Required("email", "test@example.com").
		Email("email", "test@example.com").
		MinLen("email", "test@example.com", 3)

	if v.HasErrors() {
		t.Errorf("unexpected errors: %v", v.Errors())
	}
}
