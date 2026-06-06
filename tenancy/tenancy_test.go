package tenancy

import (
	"context"
	"encoding/json"
	"errors"
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

	t.Run("returns false without error when both headers are whitespace only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "  ")
		req.Header.Set("X-Tenant-Slug", "\t\n")

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

// --- Middleware Tests ---

// testHandler reads tenant from context and writes a simple JSON response.
// If no tenant is present, it writes "no tenant".
func testHandler(w http.ResponseWriter, r *http.Request) {
	tenant, ok := FromContext(r.Context())
	resp := map[string]any{
		"has_tenant": ok,
	}
	if ok {
		resp["tenant_id"] = tenant.ID
		resp["tenant_slug"] = tenant.Slug
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// In tests, we don't care about write errors from the test handler.
		return
	}
}

func TestMiddleware(t *testing.T) {
	resolver := HeaderResolver{}

	t.Run("adds tenant to context when resolver finds one", func(t *testing.T) {
		handler := Middleware(resolver)(http.HandlerFunc(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Tenant-ID", "t1")
		req.Header.Set("X-Tenant-Slug", "acme")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["has_tenant"] != true {
			t.Errorf("has_tenant = %v, want true", body["has_tenant"])
		}
		if body["tenant_id"] != "t1" {
			t.Errorf("tenant_id = %v, want %q", body["tenant_id"], "t1")
		}
		if body["tenant_slug"] != "acme" {
			t.Errorf("tenant_slug = %v, want %q", body["tenant_slug"], "acme")
		}
	})

	t.Run("passes through without error when no tenant present", func(t *testing.T) {
		handler := Middleware(resolver)(http.HandlerFunc(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["has_tenant"] != false {
			t.Errorf("has_tenant = %v, want false", body["has_tenant"])
		}
	})

	t.Run("returns 400 with JSON error when resolver errors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Tenant-ID", "t1")
		// X-Tenant-Slug missing triggers a resolver error

		handler := Middleware(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called when resolver errors")
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body tenantErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.Code != "tenant_required" {
			t.Errorf("code = %q, want %q", body.Error.Code, "tenant_required")
		}
		if !strings.Contains(body.Error.Message, "X-Tenant-Slug is missing") {
			t.Errorf("message = %q, want to contain 'X-Tenant-Slug is missing'", body.Error.Message)
		}
	})

	t.Run("returns 400 with JSON error for custom resolver errors", func(t *testing.T) {
		errResolver := &errorResolver{err: errors.New("custom resolver failure")}
		handler := Middleware(errResolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called when resolver errors")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var body tenantErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.Code != "tenant_required" {
			t.Errorf("code = %q, want %q", body.Error.Code, "tenant_required")
		}
		if !strings.Contains(body.Error.Message, "custom resolver failure") {
			t.Errorf("message = %q, want to contain 'custom resolver failure'", body.Error.Message)
		}
	})

	t.Run("includes request_id in error when present in context", func(t *testing.T) {
		errResolver := &errorResolver{err: errors.New("fail")}
		handler := Middleware(errResolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), requestIDKey{}, "req-123")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var body tenantErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.RequestID != "req-123" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "req-123")
		}
	})

	t.Run("omits request_id when not in context", func(t *testing.T) {
		errResolver := &errorResolver{err: errors.New("fail")}
		handler := Middleware(errResolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}

		errObj, ok := body["error"].(map[string]any)
		if !ok {
			t.Fatal("expected 'error' object in response")
		}
		if _, exists := errObj["request_id"]; exists {
			t.Error("request_id should be omitted when not present")
		}
	})

	t.Run("composes with RequireMiddleware", func(t *testing.T) {
		// Build the middleware stack: Middleware then RequireMiddleware
		inner := http.HandlerFunc(testHandler)
		required := RequireMiddleware()(inner)
		full := Middleware(resolver)(required)

		t.Run("allows request when tenant is present", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-Tenant-ID", "t1")
			req.Header.Set("X-Tenant-Slug", "acme")
			rec := httptest.NewRecorder()

			full.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["has_tenant"] != true {
				t.Errorf("has_tenant = %v, want true", body["has_tenant"])
			}
		})

		t.Run("rejects request when no tenant present", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			full.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}

			var body tenantErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Error.Code != "tenant_required" {
				t.Errorf("code = %q, want %q", body.Error.Code, "tenant_required")
			}
		})
	})
}

func TestRequireMiddleware(t *testing.T) {
	t.Run("rejects request with 422 when no tenant in context", func(t *testing.T) {
		handler := RequireMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called without tenant")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body tenantErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.Code != "tenant_required" {
			t.Errorf("code = %q, want %q", body.Error.Code, "tenant_required")
		}
		if !strings.Contains(body.Error.Message, "tenant is required") {
			t.Errorf("message = %q, want to contain 'tenant is required'", body.Error.Message)
		}
	})

	t.Run("allows request when tenant is in context", func(t *testing.T) {
		var capturedTenantID string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, ok := FromContext(r.Context())
			if !ok {
				t.Error("expected tenant in context")
				return
			}
			capturedTenantID = tenant.ID
			w.WriteHeader(http.StatusOK)
		})

		handler := RequireMiddleware()(next)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), tenantKey{}, Tenant{ID: "t1", Slug: "acme"})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if capturedTenantID != "t1" {
			t.Errorf("capturedTenantID = %q, want %q", capturedTenantID, "t1")
		}
	})

	t.Run("fails closed with empty tenant ID", func(t *testing.T) {
		// Even if someone stores a zero-value Tenant in context, it still counts.
		// This is a design decision: presence in context is what matters.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called")
		})

		handler := RequireMiddleware()(next)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (should fail closed with no tenant)",
				rec.Code, http.StatusUnprocessableEntity)
		}
	})

	t.Run("includes request_id in error when present in context", func(t *testing.T) {
		handler := RequireMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), requestIDKey{}, "req-abc-456")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var body tenantErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.RequestID != "req-abc-456" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "req-abc-456")
		}
	})

	t.Run("error response body matches spec", func(t *testing.T) {
		handler := RequireMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var raw map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		errObj, ok := raw["error"].(map[string]any)
		if !ok {
			t.Fatal("expected 'error' object in response")
		}

		if errObj["code"] != "tenant_required" {
			t.Errorf("code = %v, want %q", errObj["code"], "tenant_required")
		}

		msg, _ := errObj["message"].(string)
		if !strings.Contains(msg, "tenant is required") {
			t.Errorf("message = %q, want to contain 'tenant is required'", msg)
		}
	})
}

// --- Test Helpers ---

// errorResolver is a test Resolver that always returns an error.
type errorResolver struct {
	err error
}

func (e *errorResolver) ResolveTenant(_ *http.Request) (Tenant, bool, error) {
	return Tenant{}, false, e.err
}
