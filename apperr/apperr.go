// Package apperr provides consistent error types and JSON error responses for
// Grove framework-generated failures. All framework errors follow a standard
// JSON structure so that consumers receive predictable error responses regardless
// of which Grove capability triggers the error.
//
// Response format:
//
//	{"error":{"code":"...","message":"...","request_id":"..."}}
//
// Request ID is included when available in the request context. Production
// responses never include stack traces or internal implementation details.
package apperr

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kusold/grove/httpx"
)

// Error represents a framework-generated application error with a consistent
// structure for JSON error responses. It implements the error interface so it
// can be used directly in error-returning code paths.
//
// The Cause field is logged but never exposed in HTTP responses. This ensures
// internal details like stack traces, database errors, or resolver failures
// are never leaked to clients.
type Error struct {
	// Code is a machine-readable error identifier (e.g. "tenant_required").
	Code string
	// Message is a human-readable error description safe for client responses.
	Message string
	// StatusCode is the HTTP status code to use when writing this error.
	StatusCode int
	// Cause is an optional underlying error. It is logged but never included
	// in the HTTP response.
	Cause error
}

// Error implements the error interface. It returns a formatted string
// including the code and message for logging and debugging.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("grove: %s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("grove: %s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As
// chaining.
func (e *Error) Unwrap() error {
	return e.Cause
}

// ErrorResponse is the JSON structure for framework-generated errors.
// It is exported so that tests and consumers can decode error responses
// into a typed structure.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the fields of a single error response.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteError writes a consistent JSON error response to w. It sets
// Content-Type to application/json and includes the request ID from context
// when available. The error code and message are taken from the provided Error.
//
// This is the canonical way to write framework errors from middleware and
// handlers. Service domain errors can be converted to apperr.Error before
// calling WriteError, or services can use their own response format for
// domain-specific errors.
func WriteError(w http.ResponseWriter, r *http.Request, appErr *Error) {
	var requestID string
	if id, ok := httpx.RequestIDFromContext(r.Context()); ok {
		requestID = id
	}

	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:      appErr.Code,
			Message:   appErr.Message,
			RequestID: requestID,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Framework Error Constructors ---
// These constructors create Error values for common framework failure modes.
// Each returns an *Error with the appropriate code, message, and status code.
// Only the constructors needed by implemented capabilities are provided.
// Additional constructors (NotFound, Unauthenticated, PermissionDenied,
// Internal) should be added in the phases that introduce those capabilities.

// TenantRequired returns an Error for routes that require a tenant but
// received a request without one. It maps to HTTP 422 Unprocessable Entity.
func TenantRequired() *Error {
	return &Error{
		Code:       "tenant_required",
		Message:    "tenant is required",
		StatusCode: http.StatusUnprocessableEntity,
	}
}

// InvalidTenant returns an Error for requests where tenant resolution failed
// due to malformed or incomplete tenant information. It maps to HTTP 400 Bad
// Request.
func InvalidTenant() *Error {
	return &Error{
		Code:       "invalid_tenant",
		Message:    "invalid tenant",
		StatusCode: http.StatusBadRequest,
	}
}
