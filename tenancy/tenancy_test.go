package tenancy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTenant(t *testing.T) {
	t.Run("holds ID and Slug", func(t *testing.T) {
		tenant := Tenant{ID: "00000000-0000-0000-0000-000000000001", Slug: "acme"}
		if tenant.ID != "00000000-0000-0000-0000-000000000001" {
			t.Errorf("ID = %q, want %q", tenant.ID, "00000000-0000-0000-0000-000000000001")
		}
		if tenant.Slug != "acme" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "acme")
		}
	})
}

func TestWithTenant(t *testing.T) {
	t.Run("stores tenant in context", func(t *testing.T) {
		tenant := Tenant{ID: "t1", Slug: "test"}
		ctx := WithTenant(context.Background(), tenant)

		got, ok := ctx.Value(tenantKey{}).(Tenant)
		if !ok {
			t.Fatal("expected tenant in context")
		}
		if got != tenant {
			t.Errorf("got %v, want %v", got, tenant)
		}
	})

	t.Run("does not modify the original context", func(t *testing.T) {
		orig := context.Background()
		_ = WithTenant(orig, Tenant{ID: "t1", Slug: "test"})

		_, ok := orig.Value(tenantKey{}).(Tenant)
		if ok {
			t.Error("original context should not have a tenant")
		}
	})

	t.Run("overwrites previous tenant", func(t *testing.T) {
		first := Tenant{ID: "t1", Slug: "first"}
		second := Tenant{ID: "t2", Slug: "second"}
		ctx := WithTenant(context.Background(), first)
		ctx = WithTenant(ctx, second)

		got, ok := FromContext(ctx)
		if !ok {
			t.Fatal("expected tenant in context")
		}
		if got.ID != "t2" {
			t.Errorf("got ID %q, want %q", got.ID, "t2")
		}
		if got.Slug != "second" {
			t.Errorf("got Slug %q, want %q", got.Slug, "second")
		}
	})
}

func TestFromContext(t *testing.T) {
	t.Run("returns tenant when present", func(t *testing.T) {
		tenant := Tenant{ID: "abc", Slug: "acme"}
		ctx := WithTenant(context.Background(), tenant)

		got, ok := FromContext(ctx)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if got != tenant {
			t.Errorf("got %v, want %v", got, tenant)
		}
	})

	t.Run("returns false when no tenant present", func(t *testing.T) {
		_, ok := FromContext(context.Background())
		if ok {
			t.Error("expected ok to be false with no tenant")
		}
	})

	t.Run("returns zero value tenant when absent", func(t *testing.T) {
		got, ok := FromContext(context.Background())
		if ok {
			t.Error("expected ok to be false")
		}
		if got != (Tenant{}) {
			t.Errorf("expected zero-value Tenant, got %v", got)
		}
	})
}

func TestRequire(t *testing.T) {
	t.Run("returns tenant when present", func(t *testing.T) {
		tenant := Tenant{ID: "t1", Slug: "test"}
		ctx := WithTenant(context.Background(), tenant)

		got, err := Require(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != tenant {
			t.Errorf("got %v, want %v", got, tenant)
		}
	})

	t.Run("returns error when no tenant present", func(t *testing.T) {
		_, err := Require(context.Background())
		if err == nil {
			t.Fatal("expected error when no tenant in context")
		}
	})

	t.Run("error message is clear and actionable", func(t *testing.T) {
		_, err := Require(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "tenant is required") {
			t.Errorf("error = %q, want to contain 'tenant is required'", msg)
		}
		if !strings.Contains(msg, "grove:") {
			t.Errorf("error = %q, want to contain 'grove:' prefix", msg)
		}
	})
}

func TestResolverInterface(t *testing.T) {
	t.Run("HeaderResolver implements Resolver", func(t *testing.T) {
		// This test exists to ensure HeaderResolver satisfies the Resolver
		// interface at compile time. If it does not, this line will fail to
		// compile.
		var _ Resolver = HeaderResolver{}
	})

	t.Run("*struct can implement Resolver for auth-based resolvers", func(t *testing.T) {
		// Verify the interface is compatible with pointer receivers so future
		// auth-claim-based resolvers can hold state (e.g. OIDC verifier).
		resolver := struct{ Resolver }{Resolver: HeaderResolver{}}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "t1")
		req.Header.Set("X-Tenant-Slug", "acme")

		tenant, ok, err := resolver.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected tenant to be found")
		}
		if tenant.ID != "t1" {
			t.Errorf("ID = %q, want %q", tenant.ID, "t1")
		}
		if tenant.Slug != "acme" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "acme")
		}
	})
}

func TestHeaderResolver(t *testing.T) {
	resolver := HeaderResolver{}

	t.Run("resolves tenant from both headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "00000000-0000-0000-0000-000000000001")
		req.Header.Set("X-Tenant-Slug", "acme")

		tenant, ok, err := resolver.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if tenant.ID != "00000000-0000-0000-0000-000000000001" {
			t.Errorf("ID = %q, want %q", tenant.ID, "00000000-0000-0000-0000-000000000001")
		}
		if tenant.Slug != "acme" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "acme")
		}
	})

	t.Run("returns false without error when no headers present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		tenant, ok, err := resolver.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if tenant != (Tenant{}) {
			t.Errorf("expected zero-value Tenant, got %v", tenant)
		}
	})

	t.Run("returns error when X-Tenant-ID present but X-Tenant-Slug missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "t1")

		_, ok, err := resolver.ResolveTenant(req)
		if err == nil {
			t.Fatal("expected error when ID header present without Slug")
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if !strings.Contains(err.Error(), "X-Tenant-Slug is missing") {
			t.Errorf("error = %q, want to contain 'X-Tenant-Slug is missing'", err.Error())
		}
	})

	t.Run("returns error when X-Tenant-Slug present but X-Tenant-ID missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-Slug", "acme")

		_, ok, err := resolver.ResolveTenant(req)
		if err == nil {
			t.Fatal("expected error when Slug header present without ID")
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if !strings.Contains(err.Error(), "X-Tenant-ID is missing") {
			t.Errorf("error = %q, want to contain 'X-Tenant-ID is missing'", err.Error())
		}
	})

	t.Run("returns error when X-Tenant-ID is whitespace only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "  ")
		req.Header.Set("X-Tenant-Slug", "acme")

		_, ok, err := resolver.ResolveTenant(req)
		if err == nil {
			t.Fatal("expected error for whitespace-only ID")
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if !strings.Contains(err.Error(), "X-Tenant-ID") {
			t.Errorf("error = %q, want to mention X-Tenant-ID", err.Error())
		}
	})

	t.Run("returns error when X-Tenant-Slug is whitespace only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "t1")
		req.Header.Set("X-Tenant-Slug", "\t\n")

		_, ok, err := resolver.ResolveTenant(req)
		if err == nil {
			t.Fatal("expected error for whitespace-only Slug")
		}
		if ok {
			t.Error("expected ok to be false")
		}
		if !strings.Contains(err.Error(), "X-Tenant-Slug") {
			t.Errorf("error = %q, want to mention X-Tenant-Slug", err.Error())
		}
	})

	t.Run("preserves exact header values including non-whitespace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "  t1  ")
		req.Header.Set("X-Tenant-Slug", "acme-corp")

		tenant, ok, err := resolver.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if tenant.ID != "  t1  " {
			t.Errorf("ID = %q, want %q (should preserve surrounding spaces)", tenant.ID, "  t1  ")
		}
		if tenant.Slug != "acme-corp" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "acme-corp")
		}
	})

	t.Run("works with POST requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/widgets", nil)
		req.Header.Set("X-Tenant-ID", "t99")
		req.Header.Set("X-Tenant-Slug", "post-tenant")

		tenant, ok, err := resolver.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if tenant.ID != "t99" {
			t.Errorf("ID = %q, want %q", tenant.ID, "t99")
		}
		if tenant.Slug != "post-tenant" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "post-tenant")
		}
	})

	t.Run("error messages have grove: prefix", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "t1")

		_, _, err := resolver.ResolveTenant(req)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "grove:") {
			t.Errorf("error = %q, want to contain 'grove:' prefix", err.Error())
		}
	})

	t.Run("zero value HeaderResolver works without initialization", func(t *testing.T) {
		var r HeaderResolver
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		_, ok, err := r.ResolveTenant(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected ok to be false with no headers")
		}
	})
}
