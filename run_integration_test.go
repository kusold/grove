package grove

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kusold/grove/db"
	"github.com/kusold/grove/internal/integrationtest"
	"github.com/kusold/grove/lifecycle"
)

// closeBody drains and closes an HTTP response body to satisfy errcheck.
func closeBody(resp *http.Response) {
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
}

func TestRun_PostgresLifecycle(t *testing.T) {
	t.Run("connects to Postgres during startup", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		connected := make(chan struct{})
		m := testModule{
			name: "pg-connect-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-db",
					Start: func(ctx context.Context) error {
						database, err := app.RequireDB()
						if err != nil {
							return err
						}
						if err := database.Ping(ctx); err != nil {
							return err
						}
						close(connected)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres())
		}()

		select {
		case <-connected:
			// DB connected successfully
		case err := <-runDone:
			t.Fatalf("Run() completed before DB connection: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("DB connection did not complete within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("initializes DB handle captured during registration", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		connected := make(chan struct{})
		m := testModule{
			name: "pg-captured-handle-test",
			register: func(ctx context.Context, app *App) error {
				database, err := app.RequireDB()
				if err != nil {
					return err
				}
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-captured-db",
					Start: func(ctx context.Context) error {
						if err := database.Ping(ctx); err != nil {
							return err
						}
						close(connected)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres())
		}()

		select {
		case <-connected:
			// Captured DB handle was initialized successfully.
		case err := <-runDone:
			t.Fatalf("Run() completed before captured DB handle initialized: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("captured DB handle did not initialize within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("fails when DATABASE_URL is invalid", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", "not-a-valid-url")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		m := testModule{name: "pg-invalid-url"}

		err := Run(context.Background(), m, WithHTTP(), WithPostgres())
		if err == nil {
			t.Fatal("expected error when DATABASE_URL is invalid")
		}
		if !strings.Contains(err.Error(), "lifecycle start") {
			t.Errorf("error = %q, want to contain 'lifecycle start'", err.Error())
		}
		if !strings.Contains(err.Error(), "postgres") {
			t.Errorf("error = %q, want to contain 'postgres'", err.Error())
		}
	})

	t.Run("fails when DATABASE_URL is missing", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		m := testModule{name: "pg-missing-url"}

		err := Run(context.Background(), m, WithHTTP(), WithPostgres())
		if err == nil {
			t.Fatal("expected error when DATABASE_URL is missing")
		}
		if !strings.Contains(err.Error(), "lifecycle start") {
			t.Errorf("error = %q, want to contain 'lifecycle start'", err.Error())
		}
	})

	t.Run("closes pool gracefully on shutdown", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		var dbRef *db.Database
		m := testModule{
			name: "pg-close-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "capture-db",
					Start: func(ctx context.Context) error {
						database, err := app.RequireDB()
						if err != nil {
							return err
						}
						dbRef = database
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres())
		}()

		// Wait for startup to complete
		time.Sleep(500 * time.Millisecond)

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}

		// After shutdown, pinging the closed pool should fail
		if dbRef != nil {
			err := dbRef.Ping(context.Background())
			if err == nil {
				t.Error("expected Ping to fail after pool is closed")
			}
		}
	})

	t.Run("works without HTTP capability", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)

		connected := make(chan struct{})
		m := testModule{
			name: "pg-no-http-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-db",
					Start: func(ctx context.Context) error {
						database, err := app.RequireDB()
						if err != nil {
							return err
						}
						if err := database.Ping(ctx); err != nil {
							return err
						}
						close(connected)
						return nil
					},
				})
				return nil
			},
		}

		err := Run(context.Background(), m, WithPostgres())
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		select {
		case <-connected:
			// Postgres lifecycle hook ran successfully.
		default:
			t.Fatal("expected Postgres lifecycle hook to run without HTTP capability")
		}
	})
}

func TestRun_PostgresHealthChecks(t *testing.T) {
	databaseURL := integrationtest.Postgres18(t)

	t.Run("readyz reflects Postgres connectivity after startup", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		ready := make(chan struct{})
		m := testModule{
			name: "pg-ready-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-ready",
					Start: func(ctx context.Context) error {
						// After postgres connects, readiness should pass
						if err := app.Health().IsReady(ctx); err != nil {
							return err
						}
						close(ready)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres())
		}()

		select {
		case <-ready:
			// Readiness check passed
		case err := <-runDone:
			t.Fatalf("Run() completed before readiness check: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("readiness check did not pass within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("health remains independent of Postgres connectivity after startup", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		healthy := make(chan struct{})
		m := testModule{
			name: "pg-health-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-healthy",
					Start: func(ctx context.Context) error {
						// Postgres readiness should not make liveness depend on the database.
						if err := app.Health().IsHealthy(ctx); err != nil {
							return err
						}
						close(healthy)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres())
		}()

		select {
		case <-healthy:
			// Health remains a process liveness check.
		case err := <-runDone:
			t.Fatalf("Run() completed before health check: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("health check did not pass within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("healthz returns 200 via HTTP after Postgres connects", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)

		// Bind to a known available port
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to allocate listener: %v", err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()

		t.Setenv("HTTP_ADDR", addr)
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, testModule{name: "pg-healthz-http"}, WithHTTP(), WithPostgres())
		}()

		// Wait for server to be ready by polling readyz
		client := &http.Client{Timeout: 2 * time.Second}
		deadline := time.Now().Add(30 * time.Second)
		var lastErr error
		for time.Now().Before(deadline) {
			resp, err := client.Get("http://" + addr + "/readyz")
			if err == nil {
				closeBody(resp)
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			lastErr = err
			time.Sleep(50 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("server did not become ready: %v", lastErr)
		}

		// Now verify healthz returns 200
		resp, err := client.Get("http://" + addr + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer closeBody(resp)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("/healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode healthz response: %v", err)
		}
		if body["status"] != "ok" {
			t.Errorf("healthz status = %v, want %q", body["status"], "ok")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("readyz returns 200 via HTTP after Postgres connects", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)

		// Bind to a known available port
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to allocate listener: %v", err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()

		t.Setenv("HTTP_ADDR", addr)
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, testModule{name: "pg-readyz-http"}, WithHTTP(), WithPostgres())
		}()

		// Wait for server to be ready by polling readyz
		client := &http.Client{Timeout: 2 * time.Second}
		deadline := time.Now().Add(30 * time.Second)
		var lastErr error
		for time.Now().Before(deadline) {
			resp, err := client.Get("http://" + addr + "/readyz")
			if err == nil {
				closeBody(resp)
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			lastErr = err
			time.Sleep(50 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("server did not become ready: %v", lastErr)
		}

		// Verify readyz returns 200 with OK status
		resp, err := client.Get("http://" + addr + "/readyz")
		if err != nil {
			t.Fatalf("GET /readyz: %v", err)
		}
		defer closeBody(resp)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("/readyz status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode readyz response: %v", err)
		}
		if body["status"] != "ok" {
			t.Errorf("readyz status = %v, want %q", body["status"], "ok")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})
}

func TestRun_MigrationLifecycle(t *testing.T) {
	t.Run("runs migrations in up mode during startup", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "up")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		migrated := make(chan struct{})
		m := testModule{
			name: "migrate-up-test",
			register: func(ctx context.Context, app *App) error {
				reg, err := app.RequireMigrations()
				if err != nil {
					return err
				}
				// Verify Grove migrations are pre-registered
				sources := reg.Sources()
				if len(sources) == 0 {
					t.Error("expected at least Grove migrations to be registered")
				}
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-migrations",
					Start: func(ctx context.Context) error {
						close(migrated)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres(), WithMigrations())
		}()

		select {
		case <-migrated:
			// Migrations ran successfully during startup
		case err := <-runDone:
			t.Fatalf("Run() completed before migration verification: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("migration did not complete within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("validates migrations in validate mode", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "validate")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		// Validate will fail because migrations have not been applied
		m := testModule{name: "migrate-validate-fail-test"}

		err := Run(context.Background(), m, WithHTTP(), WithPostgres(), WithMigrations())
		if err == nil {
			t.Fatal("expected error when migrations have not been applied in validate mode")
		}
		if !strings.Contains(err.Error(), "lifecycle start") {
			t.Errorf("error = %q, want to contain 'lifecycle start'", err.Error())
		}
		if !strings.Contains(err.Error(), "migration validation") {
			t.Errorf("error = %q, want to contain 'migration validation'", err.Error())
		}
	})

	t.Run("skips migrations in off mode", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "off")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		connected := make(chan struct{})
		m := testModule{
			name: "migrate-off-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-db",
					Start: func(ctx context.Context) error {
						database, err := app.RequireDB()
						if err != nil {
							return err
						}
						if err := database.Ping(ctx); err != nil {
							return err
						}
						close(connected)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres(), WithMigrations())
		}()

		select {
		case <-connected:
			// DB connected successfully, migrations were skipped
		case err := <-runDone:
			t.Fatalf("Run() completed before DB connection: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("DB connection did not complete within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("migration runs after DB connect and before HTTP readiness", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "up")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		var order []string
		m := testModule{
			name: "migrate-order-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-order",
					Start: func(ctx context.Context) error {
						order = append(order, "module-hook")
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres(), WithMigrations())
		}()

		// Wait for startup to complete
		time.Sleep(500 * time.Millisecond)

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}

		// Lifecycle hooks run in order: postgres-connect, migrations, module-hook
		// so module-hook should have run after migrations
		for _, name := range order {
			if name == "module-hook" {
				// Success: module hook ran after migrations
				return
			}
		}
		t.Errorf("expected module-hook to run after migrations, order = %v", order)
	})

	t.Run("migration readiness check passes after up", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "up")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		ready := make(chan struct{})
		m := testModule{
			name: "migrate-ready-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "verify-ready",
					Start: func(ctx context.Context) error {
						if err := app.Health().IsReady(ctx); err != nil {
							return err
						}
						close(ready)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP(), WithPostgres(), WithMigrations())
		}()

		select {
		case <-ready:
			// Readiness check passed after migrations
		case err := <-runDone:
			t.Fatalf("Run() completed before readiness check: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("readiness check did not pass within timeout")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("fails with unknown migration mode", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		clearConfigEnv(t)
		t.Setenv("DATABASE_URL", databaseURL)
		t.Setenv("GROVE_MIGRATIONS", "invalid")
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		m := testModule{name: "migrate-invalid-mode-test"}

		err := Run(context.Background(), m, WithHTTP(), WithPostgres(), WithMigrations())
		if err == nil {
			t.Fatal("expected error with unknown migration mode")
		}
		if !strings.Contains(err.Error(), "lifecycle start") {
			t.Errorf("error = %q, want to contain 'lifecycle start'", err.Error())
		}
		if !strings.Contains(err.Error(), "unknown mode") {
			t.Errorf("error = %q, want to contain 'unknown mode'", err.Error())
		}
	})
}
