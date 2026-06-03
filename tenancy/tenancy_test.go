package tenancy

import (
	"context"
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
