package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		r := New()
		if r == nil {
			t.Fatal("New() returned nil")
		}
	})
}

func TestRegistry_IsHealthy(t *testing.T) {
	t.Run("returns nil with no checks", func(t *testing.T) {
		r := New()
		if err := r.IsHealthy(context.Background()); err != nil {
			t.Errorf("IsHealthy() with no checks = %v, want nil", err)
		}
	})

	t.Run("returns nil when all checks pass", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{
			Name:  "always-ok",
			Check: func(ctx context.Context) error { return nil },
		})
		if err := r.IsHealthy(context.Background()); err != nil {
			t.Errorf("IsHealthy() = %v, want nil", err)
		}
	})

	t.Run("returns error when a check fails", func(t *testing.T) {
		r := New()
		wantErr := errors.New("something broke")
		r.RegisterHealth(Check{
			Name:  "failing",
			Check: func(ctx context.Context) error { return wantErr },
		})
		err := r.IsHealthy(context.Background())
		if err == nil {
			t.Fatal("IsHealthy() should return error when check fails")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("IsHealthy() error = %v, want to wrap %v", err, wantErr)
		}
	})

	t.Run("returns first error when multiple checks fail", func(t *testing.T) {
		r := New()
		err1 := errors.New("first error")
		err2 := errors.New("second error")
		r.RegisterHealth(Check{Name: "check-1", Check: func(ctx context.Context) error { return err1 }})
		r.RegisterHealth(Check{Name: "check-2", Check: func(ctx context.Context) error { return err2 }})
		got := r.IsHealthy(context.Background())
		if !errors.Is(got, err1) {
			t.Errorf("IsHealthy() = %v, want to wrap first error %v", got, err1)
		}
	})

	t.Run("runs all checks even after first failure", func(t *testing.T) {
		r := New()
		var ran []string
		r.RegisterHealth(Check{Name: "a", Check: func(ctx context.Context) error {
			ran = append(ran, "a")
			return errors.New("fail")
		}})
		r.RegisterHealth(Check{Name: "b", Check: func(ctx context.Context) error {
			ran = append(ran, "b")
			return nil
		}})
		_ = r.IsHealthy(context.Background())
		if len(ran) != 2 {
			t.Errorf("expected both checks to run, got %v", ran)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		r := New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r.RegisterHealth(Check{Name: "cancelled", Check: func(ctx context.Context) error {
			return ctx.Err()
		}})
		err := r.IsHealthy(ctx)
		if err == nil {
			t.Error("expected context cancelled error")
		}
	})
}

func TestRegistry_IsReady(t *testing.T) {
	t.Run("returns nil with no checks", func(t *testing.T) {
		r := New()
		if err := r.IsReady(context.Background()); err != nil {
			t.Errorf("IsReady() with no checks = %v, want nil", err)
		}
	})

	t.Run("returns nil when all checks pass", func(t *testing.T) {
		r := New()
		r.RegisterReadiness(Check{
			Name:  "db",
			Check: func(ctx context.Context) error { return nil },
		})
		if err := r.IsReady(context.Background()); err != nil {
			t.Errorf("IsReady() = %v, want nil", err)
		}
	})

	t.Run("returns error when a readiness check fails", func(t *testing.T) {
		r := New()
		wantErr := errors.New("db unavailable")
		r.RegisterReadiness(Check{
			Name:  "db",
			Check: func(ctx context.Context) error { return wantErr },
		})
		err := r.IsReady(context.Background())
		if !errors.Is(err, wantErr) {
			t.Errorf("IsReady() = %v, want to wrap %v", err, wantErr)
		}
	})
}

func TestRegistry_SeparateEvaluationPaths(t *testing.T) {
	t.Run("health and readiness checks are independent", func(t *testing.T) {
		r := New()
		healthErr := errors.New("health failing")
		r.RegisterHealth(Check{
			Name:  "health-check",
			Check: func(ctx context.Context) error { return healthErr },
		})
		// No readiness checks registered
		if err := r.IsReady(context.Background()); err != nil {
			t.Errorf("IsReady() = %v, want nil (no readiness checks)", err)
		}
		if err := r.IsHealthy(context.Background()); !errors.Is(err, healthErr) {
			t.Errorf("IsHealthy() = %v, want to wrap %v", err, healthErr)
		}
	})

	t.Run("readiness failure does not affect health", func(t *testing.T) {
		r := New()
		r.RegisterReadiness(Check{
			Name:  "db-ready",
			Check: func(ctx context.Context) error { return errors.New("not ready") },
		})
		r.RegisterHealth(Check{
			Name:  "health-ok",
			Check: func(ctx context.Context) error { return nil },
		})
		if err := r.IsHealthy(context.Background()); err != nil {
			t.Errorf("IsHealthy() = %v, want nil", err)
		}
		if err := r.IsReady(context.Background()); err == nil {
			t.Error("IsReady() should return error")
		}
	})

	t.Run("health failure does not affect readiness", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{
			Name:  "health-fail",
			Check: func(ctx context.Context) error { return errors.New("unhealthy") },
		})
		r.RegisterReadiness(Check{
			Name:  "ready-ok",
			Check: func(ctx context.Context) error { return nil },
		})
		if err := r.IsReady(context.Background()); err != nil {
			t.Errorf("IsReady() = %v, want nil", err)
		}
		if err := r.IsHealthy(context.Background()); err == nil {
			t.Error("IsHealthy() should return error")
		}
	})
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	t.Run("RegisterHealth and IsHealthy from multiple goroutines", func(t *testing.T) {
		r := New()
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.RegisterHealth(Check{
					Name:  "check",
					Check: func(ctx context.Context) error { return nil },
				})
			}()
		}
		wg.Wait()
		if err := r.IsHealthy(context.Background()); err != nil {
			t.Errorf("IsHealthy() = %v, want nil", err)
		}
	})

	t.Run("RegisterReadiness and IsReady from multiple goroutines", func(t *testing.T) {
		r := New()
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.RegisterReadiness(Check{
					Name:  "check",
					Check: func(ctx context.Context) error { return nil },
				})
			}()
		}
		wg.Wait()
		if err := r.IsReady(context.Background()); err != nil {
			t.Errorf("IsReady() = %v, want nil", err)
		}
	})
}

func TestRegistry_HealthzHandler(t *testing.T) {
	t.Run("returns 200 with no checks", func(t *testing.T) {
		r := New()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.HealthzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		assertOKResponse(t, w.Body.Bytes())
	})

	t.Run("returns 200 when all checks pass", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{Name: "ok", Check: func(ctx context.Context) error { return nil }})
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.HealthzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		assertOKResponse(t, w.Body.Bytes())
	})

	t.Run("returns 503 when a check fails", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{
			Name:  "db",
			Check: func(ctx context.Context) error { return errors.New("connection refused") },
		})
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.HealthzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if body["status"] != "unhealthy" {
			t.Errorf("status = %v, want %q", body["status"], "unhealthy")
		}
		checks, ok := body["checks"].([]any)
		if !ok || len(checks) != 1 {
			t.Fatalf("checks = %v, want 1 check", body["checks"])
		}
		check := checks[0].(map[string]any)
		if check["name"] != "db" {
			t.Errorf("check name = %v, want %q", check["name"], "db")
		}
		if check["status"] != "failing" {
			t.Errorf("check status = %v, want %q", check["status"], "failing")
		}
		if check["error"] != "connection refused" {
			t.Errorf("check error = %v, want %q", check["error"], "connection refused")
		}
	})

	t.Run("returns 503 with mixed passing and failing checks", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{
			Name:  "db",
			Check: func(ctx context.Context) error { return errors.New("db down") },
		})
		r.RegisterHealth(Check{
			Name:  "cache",
			Check: func(ctx context.Context) error { return errors.New("cache down") },
		})
		r.RegisterHealth(Check{
			Name:  "ok-check",
			Check: func(ctx context.Context) error { return nil },
		})
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.HealthzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		checks := body["checks"].([]any)
		if len(checks) != 3 {
			t.Fatalf("checks len = %d, want 3", len(checks))
		}
		// First check failing
		c0 := checks[0].(map[string]any)
		if c0["status"] != "failing" {
			t.Errorf("check[0] status = %v, want %q", c0["status"], "failing")
		}
		// Passing check
		c2 := checks[2].(map[string]any)
		if c2["status"] != "passing" {
			t.Errorf("check[2] status = %v, want %q", c2["status"], "passing")
		}
	})

	t.Run("returns application/json content type", func(t *testing.T) {
		r := New()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.HealthzHandler()(w, req)
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
	})
}

func TestRegistry_ReadyzHandler(t *testing.T) {
	t.Run("returns 200 with no checks", func(t *testing.T) {
		r := New()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ReadyzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		assertOKResponse(t, w.Body.Bytes())
	})

	t.Run("returns 200 when all readiness checks pass", func(t *testing.T) {
		r := New()
		r.RegisterReadiness(Check{Name: "db", Check: func(ctx context.Context) error { return nil }})
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ReadyzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		assertOKResponse(t, w.Body.Bytes())
	})

	t.Run("returns 503 when a readiness check fails", func(t *testing.T) {
		r := New()
		r.RegisterReadiness(Check{
			Name:  "migrations",
			Check: func(ctx context.Context) error { return errors.New("pending migrations") },
		})
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ReadyzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if body["status"] != "not_ready" {
			t.Errorf("status = %v, want %q", body["status"], "not_ready")
		}
	})

	t.Run("health failure does not affect readiness handler", func(t *testing.T) {
		r := New()
		r.RegisterHealth(Check{
			Name:  "health-fail",
			Check: func(ctx context.Context) error { return errors.New("unhealthy") },
		})
		// No readiness checks
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ReadyzHandler()(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d (health failure should not affect readiness)", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("returns application/json content type", func(t *testing.T) {
		r := New()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ReadyzHandler()(w, req)
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
	})
}

func assertOKResponse(t *testing.T, body []byte) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v, body: %s", err, string(body))
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want %q", result["status"], "ok")
	}
}
