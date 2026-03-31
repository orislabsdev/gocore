package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ─────────────────────────────────────────────────────────────────────────────
// Standard API envelope
// ─────────────────────────────────────────────────────────────────────────────

// Response is the standard JSON envelope returned by all API endpoints.
// Using a consistent shape simplifies client-side parsing and logging.
//
//	{
//	  "success": true,
//	  "data": { ... }
//	}
//
//	{
//	  "success": false,
//	  "error": { "code": "NOT_FOUND", "message": "resource not found" }
//	}
type Response struct {
	// Success indicates whether the request was handled without error.
	Success bool `json:"success"`

	// Data holds the response payload for successful requests.
	// Omitted when nil (error responses).
	Data any `json:"data,omitempty"`

	// Error holds error details for failed requests.
	// Omitted when nil (success responses).
	Error *ErrorDetail `json:"error,omitempty"`

	// Meta holds optional pagination or request metadata.
	// Omitted when nil.
	Meta any `json:"meta,omitempty"`
}

// ErrorDetail carries structured error information within a Response.
type ErrorDetail struct {
	// Code is a machine-readable error identifier (e.g., "VALIDATION_ERROR").
	Code string `json:"code"`

	// Message is a human-readable description of the error.
	Message string `json:"message"`

	// Details holds field-level validation errors or additional context.
	// Omitted when nil.
	Details any `json:"details,omitempty"`
}

// PageMeta holds standard pagination metadata for list endpoints.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON responses
// ─────────────────────────────────────────────────────────────────────────────

// JSON writes a JSON-encoded body with the given HTTP status code.
// It sets Content-Type to "application/json; charset=utf-8".
// Encoding errors are handled gracefully by writing a 500 response.
func (c *Context) JSON(statusCode int, v any) {
	c.writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.writer.WriteHeader(statusCode)

	enc := json.NewEncoder(c.writer)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		// Encoding failed after headers were committed — best-effort recovery.
		// Log entry is omitted here; callers should use the Recovery middleware.
		_ = err
	}
}

// Success writes a 200 OK response using the standard envelope.
//
//	ctx.Success(map[string]string{"status": "created"})
func (c *Context) Success(data any) {
	c.JSON(http.StatusOK, Response{Success: true, Data: data})
}

// Created writes a 201 Created response using the standard envelope.
func (c *Context) Created(data any) {
	c.JSON(http.StatusCreated, Response{Success: true, Data: data})
}

// SuccessWithMeta writes a 200 OK response that includes pagination or other
// metadata alongside the data payload.
func (c *Context) SuccessWithMeta(data, meta any) {
	c.JSON(http.StatusOK, Response{Success: true, Data: data, Meta: meta})
}

// NoContent writes a 204 No Content response. The body is intentionally empty.
func (c *Context) NoContent() {
	c.writer.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────────────────────────────────────────────────────────
// Error responses
// ─────────────────────────────────────────────────────────────────────────────

// Fail writes an error response using the standard envelope.
//
//	ctx.Fail(http.StatusBadRequest, "VALIDATION_ERROR", "email is required", nil)
func (c *Context) Fail(statusCode int, code, message string, details any) {
	c.JSON(statusCode, Response{
		Success: false,
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// BadRequest writes a 400 response.
func (c *Context) BadRequest(message string) {
	c.Fail(http.StatusBadRequest, "BAD_REQUEST", message, nil)
}

// Unauthorized writes a 401 response. Use this when authentication is
// required but the request provides no valid credentials.
func (c *Context) Unauthorized(message string) {
	if message == "" {
		message = "authentication required"
	}
	c.Fail(http.StatusUnauthorized, "UNAUTHORIZED", message, nil)
}

// Forbidden writes a 403 response. Use this when the caller is authenticated
// but lacks permission to access the resource.
func (c *Context) Forbidden(message string) {
	if message == "" {
		message = "forbidden"
	}
	c.Fail(http.StatusForbidden, "FORBIDDEN", message, nil)
}

// NotFound writes a 404 response.
func (c *Context) NotFound(message string) {
	if message == "" {
		message = "resource not found"
	}
	c.Fail(http.StatusNotFound, "NOT_FOUND", message, nil)
}

// Conflict writes a 409 response. Use for duplicate resource creation attempts.
func (c *Context) Conflict(message string) {
	c.Fail(http.StatusConflict, "CONFLICT", message, nil)
}

// UnprocessableEntity writes a 422 response with optional field-level details.
//
//	ctx.UnprocessableEntity("validation failed", map[string]string{
//	    "email":    "must be a valid email address",
//	    "username": "already taken",
//	})
func (c *Context) UnprocessableEntity(message string, details any) {
	c.Fail(http.StatusUnprocessableEntity, "VALIDATION_ERROR", message, details)
}

// TooManyRequests writes a 429 response. Used by the rate-limit middleware.
func (c *Context) TooManyRequests() {
	c.Fail(http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please slow down", nil)
}

// InternalServerError writes a 500 response. Avoid exposing internal error
// details to clients in production.
func (c *Context) InternalServerError(message string) {
	if message == "" {
		message = "an internal error occurred"
	}
	c.Fail(http.StatusInternalServerError, "INTERNAL_ERROR", message, nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// Text and raw responses
// ─────────────────────────────────────────────────────────────────────────────

// String writes a plain-text response with the given status code.
func (c *Context) String(statusCode int, text string) {
	c.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.writer.WriteHeader(statusCode)
	fmt.Fprint(c.writer, text)
}

// Stringf writes a formatted plain-text response with the given status code.
func (c *Context) Stringf(statusCode int, format string, a ...any) {
	c.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.writer.WriteHeader(statusCode)
	fmt.Fprintf(c.writer, format, a...)
}


// HTML writes an HTML response with the given status code.
func (c *Context) HTML(statusCode int, body string) {
	c.writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.writer.WriteHeader(statusCode)
	fmt.Fprint(c.writer, body)
}

// Blob writes raw bytes with the specified Content-Type.
// Useful for serving binary data (images, PDFs, etc.).
func (c *Context) Blob(statusCode int, contentType string, data []byte) {
	c.writer.Header().Set("Content-Type", contentType)
	c.writer.WriteHeader(statusCode)
	_, _ = c.writer.Write(data)
}

// ─────────────────────────────────────────────────────────────────────────────
// Redirects
// ─────────────────────────────────────────────────────────────────────────────

// Redirect sends an HTTP redirect to the given URL.
// Use 301 (permanent) or 302/307/308 (temporary) as appropriate.
func (c *Context) Redirect(statusCode int, url string) {
	http.Redirect(c.writer, c.Request, url, statusCode)
}

// ─────────────────────────────────────────────────────────────────────────────
// Response header helpers
// ─────────────────────────────────────────────────────────────────────────────

// SetHeader sets a response header value. Must be called before any body
// bytes are written.
func (c *Context) SetHeader(key, value string) {
	c.writer.Header().Set(key, value)
}

// AddHeader adds a response header value without replacing existing values.
func (c *Context) AddHeader(key, value string) {
	c.writer.Header().Add(key, value)
}
