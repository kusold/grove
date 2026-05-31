package grove

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// testModule is a minimal Module implementation for testing.
type testModule struct {
	name     string
	register func(ctx context.Context, app *App) error
}

func (m testModule) Name() string { return m.name }

func (m testModule) Register(ctx context.Context, app *App) error {
	if m.register != nil {
		return m.register(ctx, app)
	}
	return nil
}

func TestModuleInterface(t *testing.T) {
	t.Run("Name returns the module name", func(t *testing.T) {
		m := testModule{name: "test-service"}
		if got := m.Name(); got != "test-service" {
			t.Errorf("Name() = %q, want %q", got, "test-service")
		}
	})

	t.Run("Register is called with context and app", func(t *testing.T) {
		called := false
		m := testModule{
			name: "test-service",
			register: func(ctx context.Context, app *App) error {
				called = true
				if app == nil {
					t.Error("app should not be nil")
				}
				if ctx == nil {
					t.Error("ctx should not be nil")
				}
				return nil
			},
		}

		err := Run(context.Background(), m)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if !called {
			t.Error("Register was not called")
		}
	})
}

func TestRun(t *testing.T) {
	t.Run("creates app with module name", func(t *testing.T) {
		m := testModule{
			name: "my-service",
			register: func(ctx context.Context, app *App) error {
				if got := app.Name(); got != "my-service" {
					t.Errorf("app.Name() = %q, want %q", got, "my-service")
				}
				return nil
			},
		}

		if err := Run(context.Background(), m); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("applies options before registration", func(t *testing.T) {
		var capturedName string
		m := testModule{
			name: "original",
			register: func(ctx context.Context, app *App) error {
				capturedName = app.Name()
				return nil
			},
		}

		overrideName := func(a *App) error {
			a.name = "overridden"
			return nil
		}

		err := Run(context.Background(), m, overrideName)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if capturedName != "overridden" {
			t.Errorf("app.Name() = %q, want %q after option applied", capturedName, "overridden")
		}
	})

	t.Run("returns error when registration fails", func(t *testing.T) {
		regErr := errors.New("registration boom")
		m := testModule{
			name: "failing-service",
			register: func(ctx context.Context, app *App) error {
				return regErr
			},
		}

		err := Run(context.Background(), m)
		if err == nil {
			t.Fatal("Run() should return an error when registration fails")
		}
		if !errors.Is(err, regErr) {
			t.Errorf("Run() error = %v, want to wrap %v", err, regErr)
		}
		if !strings.Contains(err.Error(), "failing-service") {
			t.Errorf("Run() error = %q, want to contain module name %q", err.Error(), "failing-service")
		}
		if !strings.Contains(err.Error(), "registration failed") {
			t.Errorf("Run() error = %q, want to contain 'registration failed'", err.Error())
		}
	})

	t.Run("accepts multiple options", func(t *testing.T) {
		m := testModule{name: "multi-opts"}
		opt1 := func(a *App) error { return nil }
		opt2 := func(a *App) error { return nil }

		err := Run(context.Background(), m, opt1, opt2)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("returns error when option fails", func(t *testing.T) {
		optErr := errors.New("option boom")
		failOpt := func(a *App) error { return optErr }

		err := Run(context.Background(), testModule{name: "opt-fail"}, failOpt)
		if err == nil {
			t.Fatal("Run() should return an error when an option fails")
		}
		if !errors.Is(err, optErr) {
			t.Errorf("Run() error = %v, want to wrap %v", err, optErr)
		}
		if !strings.Contains(err.Error(), "option error") {
			t.Errorf("Run() error = %q, want to contain 'option error'", err.Error())
		}
	})
}

func TestNewApp(t *testing.T) {
	t.Run("sets name from argument", func(t *testing.T) {
		app, err := newApp("test-app")
		if err != nil {
			t.Fatalf("newApp() returned unexpected error: %v", err)
		}
		if got := app.Name(); got != "test-app" {
			t.Errorf("app.Name() = %q, want %q", got, "test-app")
		}
	})

	t.Run("applies options in order", func(t *testing.T) {
		var order []string
		opt1 := func(a *App) error { order = append(order, "opt1"); return nil }
		opt2 := func(a *App) error { order = append(order, "opt2"); return nil }

		_, err := newApp("test", opt1, opt2)
		if err != nil {
			t.Fatalf("newApp() returned unexpected error: %v", err)
		}

		if len(order) != 2 || order[0] != "opt1" || order[1] != "opt2" {
			t.Errorf("options applied in wrong order: %v", order)
		}
	})

	t.Run("returns non-nil app with no options", func(t *testing.T) {
		app, err := newApp("bare")
		if err != nil {
			t.Fatalf("newApp() returned unexpected error: %v", err)
		}
		if app == nil {
			t.Error("newApp() returned nil")
		}
	})

	t.Run("stops applying options on first error", func(t *testing.T) {
		var applied []string
		opt1 := func(a *App) error { applied = append(applied, "opt1"); return nil }
		optErr := errors.New("boom")
		opt2 := func(a *App) error { return optErr }
		opt3 := func(a *App) error { applied = append(applied, "opt3"); return nil }

		_, err := newApp("test", opt1, opt2, opt3)
		if err == nil {
			t.Fatal("newApp() should return an error")
		}
		if !errors.Is(err, optErr) {
			t.Errorf("newApp() error = %v, want to wrap %v", err, optErr)
		}
		if len(applied) != 1 || applied[0] != "opt1" {
			t.Errorf("expected only opt1 applied, got %v", applied)
		}
	})
}

func TestApp(t *testing.T) {
	t.Run("Name returns the app name", func(t *testing.T) {
		app, _ := newApp("svc-name")
		if got := app.Name(); got != "svc-name" {
			t.Errorf("app.Name() = %q, want %q", got, "svc-name")
		}
	})
}

func TestCapabilityOptions(t *testing.T) {
	t.Run("WithHTTP enables http capability", func(t *testing.T) {
		app, err := newApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capHTTP) {
			t.Error("expected http capability to be enabled")
		}
	})

	t.Run("WithPostgres enables postgres capability", func(t *testing.T) {
		app, err := newApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capPostgres) {
			t.Error("expected postgres capability to be enabled")
		}
	})

	t.Run("WithMigrations enables migrations capability", func(t *testing.T) {
		app, err := newApp("test", WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capMigrations) {
			t.Error("expected migrations capability to be enabled")
		}
	})

	t.Run("WithTenancy enables tenancy capability", func(t *testing.T) {
		app, err := newApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capTenancy) {
			t.Error("expected tenancy capability to be enabled")
		}
	})

	t.Run("WithOpenAPI enables openapi capability", func(t *testing.T) {
		app, err := newApp("test", WithHTTP(), WithOpenAPI())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capOpenAPI) {
			t.Error("expected openapi capability to be enabled")
		}
	})

	t.Run("WithObservability enables observability capability", func(t *testing.T) {
		app, err := newApp("test", WithObservability())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capObservability) {
			t.Error("expected observability capability to be enabled")
		}
	})

	t.Run("WithJobs enables jobs capability", func(t *testing.T) {
		app, err := newApp("test", WithPostgres(), WithJobs())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capJobs) {
			t.Error("expected jobs capability to be enabled")
		}
	})

	t.Run("WithOIDC enables oidc capability", func(t *testing.T) {
		app, err := newApp("test", WithHTTP(), WithOIDC())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capOIDC) {
			t.Error("expected oidc capability to be enabled")
		}
	})

	t.Run("options are idempotent", func(t *testing.T) {
		app, err := newApp("test", WithHTTP(), WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capHTTP) {
			t.Error("expected http capability to be enabled")
		}
	})
}

func TestCapabilityDependencyValidation(t *testing.T) {
	t.Run("WithMigrations requires WithPostgres", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"}, WithMigrations())
		if err == nil {
			t.Fatal("expected error when WithMigrations is used without WithPostgres")
		}
		if !strings.Contains(err.Error(), "migrations requires postgres") {
			t.Errorf("error = %q, want to contain 'migrations requires postgres'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithPostgres()") {
			t.Errorf("error = %q, want to contain 'grove.WithPostgres()'", err.Error())
		}
	})

	t.Run("WithOpenAPI requires WithHTTP", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"}, WithOpenAPI())
		if err == nil {
			t.Fatal("expected error when WithOpenAPI is used without WithHTTP")
		}
		if !strings.Contains(err.Error(), "openapi requires http") {
			t.Errorf("error = %q, want to contain 'openapi requires http'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("WithTenancy requires WithHTTP", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"}, WithTenancy())
		if err == nil {
			t.Fatal("expected error when WithTenancy is used without WithHTTP")
		}
		if !strings.Contains(err.Error(), "tenancy requires http") {
			t.Errorf("error = %q, want to contain 'tenancy requires http'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("WithJobs requires WithPostgres", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"}, WithJobs())
		if err == nil {
			t.Fatal("expected error when WithJobs is used without WithPostgres")
		}
		if !strings.Contains(err.Error(), "jobs requires postgres") {
			t.Errorf("error = %q, want to contain 'jobs requires postgres'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithPostgres()") {
			t.Errorf("error = %q, want to contain 'grove.WithPostgres()'", err.Error())
		}
	})

	t.Run("WithOIDC requires WithHTTP", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"}, WithOIDC())
		if err == nil {
			t.Fatal("expected error when WithOIDC is used without WithHTTP")
		}
		if !strings.Contains(err.Error(), "oidc requires http") {
			t.Errorf("error = %q, want to contain 'oidc requires http'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("valid dependency combination passes", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"},
			WithHTTP(),
			WithPostgres(),
			WithMigrations(),
			WithTenancy(),
			WithOpenAPI(),
		)
		if err != nil {
			t.Fatalf("expected no error for valid combination, got: %v", err)
		}
	})

	t.Run("options in reverse order still validate correctly", func(t *testing.T) {
		// Pass dependencies after dependents — validation should still pass
		// because validation runs after all options are applied.
		err := Run(context.Background(), testModule{name: "test"},
			WithMigrations(),
			WithPostgres(),
		)
		if err != nil {
			t.Fatalf("expected no error when deps are provided in reverse order, got: %v", err)
		}
	})

	t.Run("no capabilities is valid", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"})
		if err != nil {
			t.Fatalf("expected no error with no capabilities, got: %v", err)
		}
	})

	t.Run("standalone capabilities without deps are valid", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"},
			WithHTTP(),
			WithObservability(),
		)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestCapabilityDeterministicOrder(t *testing.T) {
	t.Run("validation error is consistent regardless of option order", func(t *testing.T) {
		// Register multiple capabilities with missing deps in different orders.
		// The first error reported should always be deterministic (based on
		// capabilityOrder, not option order).
		err1 := Run(context.Background(), testModule{name: "test"},
			WithOIDC(),
			WithOpenAPI(),
			WithMigrations(),
		)
		err2 := Run(context.Background(), testModule{name: "test"},
			WithMigrations(),
			WithOIDC(),
			WithOpenAPI(),
		)

		if err1 == nil || err2 == nil {
			t.Fatal("expected both to fail with dependency errors")
		}

		// Both should report the same first missing dependency.
		// capabilityOrder is: observability, postgres, migrations, http, tenancy, openapi, jobs, oidc
		// With Migrations, OpenAPI, OIDC all enabled but missing deps:
		//   - migrations (order 2) needs postgres -> missing -> first error
		if err1.Error() != err2.Error() {
			t.Errorf("errors should be identical regardless of option order:\n  err1: %s\n  err2: %s", err1, err2)
		}
		if !strings.Contains(err1.Error(), "migrations requires postgres") {
			t.Errorf("expected first error to be about migrations requiring postgres, got: %s", err1)
		}
	})
}

func TestRequireCapability(t *testing.T) {
	t.Run("returns error when capability not enabled", func(t *testing.T) {
		app, err := newApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = app.requireCapability(capPostgres)
		if err == nil {
			t.Fatal("expected error when postgres capability not enabled")
		}
		if !strings.Contains(err.Error(), "postgres capability is required but was not enabled") {
			t.Errorf("error = %q, want to contain 'postgres capability is required but was not enabled'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithPostgres()") {
			t.Errorf("error = %q, want to contain 'grove.WithPostgres()'", err.Error())
		}
	})

	t.Run("returns nil when capability is enabled", func(t *testing.T) {
		app, err := newApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := app.requireCapability(capPostgres); err != nil {
			t.Errorf("expected nil error when capability is enabled, got: %v", err)
		}
	})

	t.Run("error message includes capability-specific option name", func(t *testing.T) {
		app, _ := newApp("test")
		for _, tc := range []struct {
			cap      capability
			contains string
		}{
			{capHTTP, "grove.WithHTTP()"},
			{capPostgres, "grove.WithPostgres()"},
			{capMigrations, "grove.WithMigrations()"},
			{capTenancy, "grove.WithTenancy()"},
			{capOpenAPI, "grove.WithOpenAPI()"},
			{capObservability, "grove.WithObservability()"},
			{capJobs, "grove.WithJobs()"},
			{capOIDC, "grove.WithOIDC()"},
		} {
			t.Run(fmt.Sprintf("capability %s", tc.cap), func(t *testing.T) {
				err := app.requireCapability(tc.cap)
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tc.contains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.contains)
				}
			})
		}
	})
}
