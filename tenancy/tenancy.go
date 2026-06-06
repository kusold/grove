// Package tenancy provides first-class tenant context for Grove services.
// Tenant values are stored in request context and propagated through the
// request lifecycle. All tenant state lives in context—there is no global
// tenant state.
package tenancy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kusold/grove/httpx"
)

// Tenant represents the resolved identity of a tenant. ID will likely become
// UUID-oriented once Postgres is introduced, but is kept as string initially
// to avoid forcing a UUID dependency before it is needed.
type Tenant struct {
	ID   string
	Slug string
}

type tenantKey struct{}

// WithTenant returns a new context with the given tenant attached.
func WithTenant(ctx context.Context, tenant Tenant) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}

// FromContext retrieves the tenant from the given context. It returns
// (Tenant, true) when a tenant is present, and (Tenant{}, false) when
// no tenant has been set.
func FromContext(ctx context.Context) (Tenant, bool) {
	tenant, ok := ctx.Value(tenantKey{}).(Tenant)
	return tenant, ok
}

// Require retrieves the tenant from the given context and returns an error
// with a clear message when no tenant is present. This is the safe accessor
// for handlers and code paths that require a tenant to proceed.
func Require(ctx context.Context) (Tenant, error) {
	tenant, ok := FromContext(ctx)
	if !ok {
		return Tenant{}, fmt.Errorf("grove: tenant is required but was not found in context")
	}
	return tenant, nil
}

// Resolver extracts a Tenant from an HTTP request. Implementations determine
// how tenant identity is derived—e.g. from headers, cookies, auth claims, or
// subdomain.
//
// The return values follow a three-state convention:
//   - (Tenant, true, nil): a tenant was found.
//   - (Tenant{}, false, nil): no tenant was found (not an error condition).
//   - (_, _, error): the resolver failed (e.g. malformed input).
//
// This interface is designed to remain compatible with future auth-claim-based
// tenancy resolution. Auth-based resolvers will extract the tenant from verified
// token claims rather than headers.
type Resolver interface {
	ResolveTenant(r *http.Request) (Tenant, bool, error)
}

// HeaderResolver extracts tenant identity from HTTP headers. It is intended
// for local development and demo use. In production, tenant identity should
// come from verified auth claims rather than client-supplied headers.
//
// Headers used:
//
//	X-Tenant-ID:   the tenant's unique identifier
//	X-Tenant-Slug: the tenant's human-readable slug
//
// Both headers must be present for a tenant to be resolved. Missing headers
// result in (Tenant{}, false, nil), not an error. Headers containing only
// whitespace are treated as missing.
type HeaderResolver struct{}

// ResolveTenant extracts a Tenant from X-Tenant-ID and X-Tenant-Slug headers.
// It returns (Tenant, true, nil) when both headers are present and non-empty.
// It returns (Tenant{}, false, nil) when both headers are absent or contain
// only whitespace. It returns (_, _, error) when exactly one header has a
// non-whitespace value.
func (HeaderResolver) ResolveTenant(r *http.Request) (Tenant, bool, error) {
	id := r.Header.Get("X-Tenant-ID")
	slug := r.Header.Get("X-Tenant-Slug")

	idPresent := strings.TrimSpace(id) != ""
	slugPresent := strings.TrimSpace(slug) != ""

	// Neither header present: no tenant, not an error.
	if !idPresent && !slugPresent {
		return Tenant{}, false, nil
	}

	// One header present but not the other: partially specified.
	if idPresent && !slugPresent {
		return Tenant{}, false, fmt.Errorf("grove: X-Tenant-ID header present but X-Tenant-Slug is missing")
	}
	if !idPresent && slugPresent {
		return Tenant{}, false, fmt.Errorf("grove: X-Tenant-Slug header present but X-Tenant-ID is missing")
	}

	// Both present. Reject whitespace-only values.
	if strings.TrimSpace(id) == "" {
		return Tenant{}, false, fmt.Errorf("grove: X-Tenant-ID header is present but contains only whitespace")
	}
	if strings.TrimSpace(slug) == "" {
		return Tenant{}, false, fmt.Errorf("grove: X-Tenant-Slug header is present but contains only whitespace")
	}

	return Tenant{ID: id, Slug: slug}, true, nil
}

// Middleware returns an HTTP middleware that resolves a tenant from the request
// using the provided Resolver. If a tenant is found, it is attached to the
// request context. If no tenant is found, the request passes through without
// modification. If the resolver returns an error, the middleware responds with
// HTTP 400 and a JSON error body.
//
// This middleware does not reject requests when no tenant is present. Use
// RequireMiddleware on route groups that must have a tenant.
//
// Typical composition: Middleware first (globally), then RequireMiddleware on
// specific route groups.
func Middleware(resolver Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, found, err := resolver.ResolveTenant(r)
			if err != nil {
				writeTenantError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			if found {
				r = r.WithContext(WithTenant(r.Context(), tenant))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireMiddleware returns an HTTP middleware that rejects requests when no
// tenant exists in the request context. It should be used after Middleware on
// route groups that require a tenant.
//
// When no tenant is present, it responds with HTTP 422 and a consistent JSON
// error body:
//
//	{"error":{"code":"tenant_required","message":"tenant is required"}}
//
// This middleware fails closed: if there is no tenant in context, the request
// is always rejected regardless of other conditions.
func RequireMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, ok := FromContext(r.Context())
			if !ok {
				writeTenantError(w, r, http.StatusUnprocessableEntity,
					"grove: tenant is required but was not found in context")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tenantErrorResponse is the JSON structure for framework-generated tenant
// errors. The structure matches the cross-cutting error handling convention
// described in the implementation plan:
//
//	{"error":{"code":"...","message":"...","request_id":"..."}}
//
// RequestID is included when a request ID is available in context. The
// request ID middleware may not exist yet, so the field is omitted when empty.
type tenantErrorResponse struct {
	Error tenantErrorDetail `json:"error"`
}

type tenantErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// writeTenantError writes a JSON error response for tenant-related failures.
// It sets Content-Type to application/json and includes a request ID in the
// response if one is present in the context.
func writeTenantError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	var requestID string
	if id, ok := httpx.RequestIDFromContext(r.Context()); ok {
		requestID = id
	}

	resp := tenantErrorResponse{
		Error: tenantErrorDetail{
			Code:      "tenant_required",
			Message:   message,
			RequestID: requestID,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}
