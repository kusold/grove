// Package tenancy provides first-class tenant context for Grove services.
// Tenant values are stored in request context and propagated through the
// request lifecycle. All tenant state lives in context—there is no global
// tenant state.
package tenancy

import (
	"context"
	"fmt"
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
