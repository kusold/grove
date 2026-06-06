// Package tenancy provides first-class tenant context for Grove services.
// Tenant values are stored in request context and propagated through the
// request lifecycle. All tenant state lives in context—there is no global
// tenant state.
package tenancy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
