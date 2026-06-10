package grove

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kusold/grove/db"
	"github.com/kusold/grove/internal/integrationtest"
	"github.com/kusold/grove/lifecycle"
)

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
}
