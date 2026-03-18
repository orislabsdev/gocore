// Package validate provides lightweight, zero-dependency request validation
// utilities for the gocore library.
//
// The package deliberately avoids reflection-based struct tags so that
// validation rules stay next to the code that uses them and are immediately
// readable without special tooling.
//
// Usage:
//
//	type CreateUserReq struct {
//	    Email    string `json:"email"`
//	    Password string `json:"password"`
//	    Age      int    `json:"age"`
//	}
//
//	var req CreateUserReq
//	if err := ctx.BindJSON(&req); err != nil {
//	    ctx.BadRequest("invalid JSON")
//	    return
//	}
//
//	v := validate.New()
//	v.Required("email",    req.Email)
//	v.Email("email",       req.Email)
//	v.MinLen("password",   req.Password, 8)
//	v.Range("age",         float64(req.Age), 18, 120)
//
//	if v.HasErrors() {
//	    ctx.UnprocessableEntity("validation failed", v.Errors())
//	    return
//	}
package validate

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ─────────────────────────────────────────────────────────────────────────────
// Validator
// ─────────────────────────────────────────────────────────────────────────────

// Validator collects field-level validation errors. Create one per request
// with New(), call rule methods, then check HasErrors().
type Validator struct {
	errors map[string][]string
}

// New creates an empty Validator ready to collect errors.
func New() *Validator {
	return &Validator{errors: make(map[string][]string)}
}

// ─────────────────────────────────────────────────────────────────────────────
// Error collection
// ─────────────────────────────────────────────────────────────────────────────

// addError records an error for the given field.
func (v *Validator) addError(field, message string) {
	v.errors[field] = append(v.errors[field], message)
}

// HasErrors returns true if any validation rule has failed.
func (v *Validator) HasErrors() bool { return len(v.errors) > 0 }

// Errors returns the collected errors as a map of field → []string.
// This type is directly serialisable as the "details" field of an
// UnprocessableEntity response.
func (v *Validator) Errors() map[string][]string { return v.errors }

// First returns the first error message for the given field, or an empty
// string if no error was recorded.
func (v *Validator) First(field string) string {
	if msgs, ok := v.errors[field]; ok && len(msgs) > 0 {
		return msgs[0]
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in rules
// ─────────────────────────────────────────────────────────────────────────────

// Required fails if the value is empty after trimming whitespace.
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.addError(field, "is required")
	}
	return v
}

// RequiredIf calls Required only when the condition is true.
func (v *Validator) RequiredIf(field, value string, condition bool) *Validator {
	if condition {
		return v.Required(field, value)
	}
	return v
}

// MinLen fails if the UTF-8 character count of value is below min.
func (v *Validator) MinLen(field, value string, min int) *Validator {
	if utf8.RuneCountInString(value) < min {
		v.addError(field, fmt.Sprintf("must be at least %d characters", min))
	}
	return v
}

// MaxLen fails if the UTF-8 character count of value exceeds max.
func (v *Validator) MaxLen(field, value string, max int) *Validator {
	if utf8.RuneCountInString(value) > max {
		v.addError(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return v
}

// LenBetween fails if the length is outside [min, max] (inclusive).
func (v *Validator) LenBetween(field, value string, min, max int) *Validator {
	n := utf8.RuneCountInString(value)
	if n < min || n > max {
		v.addError(field, fmt.Sprintf("must be between %d and %d characters", min, max))
	}
	return v
}

// Email fails if value is not a syntactically valid email address.
// Uses net/mail.ParseAddress for RFC 5322 compliance.
func (v *Validator) Email(field, value string) *Validator {
	if value == "" {
		return v // Required() should catch empty separately
	}
	if _, err := mail.ParseAddress(value); err != nil {
		v.addError(field, "must be a valid email address")
	}
	return v
}

// URL fails if value is not a valid absolute HTTP/HTTPS URL.
func (v *Validator) URL(field, value string) *Validator {
	if value == "" {
		return v
	}
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		v.addError(field, "must be a valid URL (http:// or https://)")
		return v
	}
	if !urlRegexp.MatchString(value) {
		v.addError(field, "must be a valid URL")
	}
	return v
}

// Range fails if value is outside the inclusive range [min, max].
// Works for any numeric type via float64.
func (v *Validator) Range(field string, value, min, max float64) *Validator {
	if value < min || value > max {
		v.addError(field, fmt.Sprintf("must be between %v and %v", min, max))
	}
	return v
}

// Min fails if value < min.
func (v *Validator) Min(field string, value, min float64) *Validator {
	if value < min {
		v.addError(field, fmt.Sprintf("must be at least %v", min))
	}
	return v
}

// Max fails if value > max.
func (v *Validator) Max(field string, value, max float64) *Validator {
	if value > max {
		v.addError(field, fmt.Sprintf("must be at most %v", max))
	}
	return v
}

// OneOf fails if value is not one of the permitted string values.
func (v *Validator) OneOf(field, value string, permitted ...string) *Validator {
	for _, p := range permitted {
		if value == p {
			return v
		}
	}
	v.addError(field, fmt.Sprintf("must be one of: %s", strings.Join(permitted, ", ")))
	return v
}

// NotEmpty fails if the slice has zero elements.
func (v *Validator) NotEmpty(field string, slice []string) *Validator {
	if len(slice) == 0 {
		v.addError(field, "must not be empty")
	}
	return v
}

// Matches fails if value does not match the regular expression re.
// The pattern is compiled once by the caller and passed in.
func (v *Validator) Matches(field, value string, re *regexp.Regexp) *Validator {
	if !re.MatchString(value) {
		v.addError(field, "has an invalid format")
	}
	return v
}

// Custom executes fn(value) and, if it returns a non-empty string, records
// that string as the error message. Use this for domain-specific rules.
//
//	v.Custom("username", req.Username, func(s string) string {
//	    if isReserved(s) {
//	        return "username is reserved"
//	    }
//	    return ""
//	})
func (v *Validator) Custom(field, value string, fn func(string) string) *Validator {
	if msg := fn(value); msg != "" {
		v.addError(field, msg)
	}
	return v
}

// ─────────────────────────────────────────────────────────────────────────────
// Pre-compiled patterns
// ─────────────────────────────────────────────────────────────────────────────

// urlRegexp is a basic URL validator. net/url.Parse is too permissive for
// user input; this pattern requires scheme + host at minimum.
var urlRegexp = regexp.MustCompile(
	`^(https?://)[^\s/$.?#].[^\s]*$`,
)
