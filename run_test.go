package grove

import (
	"context"
	"errors"
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

		// Option that overrides the app name
		overrideName := func(a *App) {
			a.name = "overridden"
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
		opt1 := func(a *App) {}
		opt2 := func(a *App) {}

		err := Run(context.Background(), m, opt1, opt2)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})
}

func TestNewApp(t *testing.T) {
	t.Run("sets name from argument", func(t *testing.T) {
		app := newApp("test-app")
		if got := app.Name(); got != "test-app" {
			t.Errorf("app.Name() = %q, want %q", got, "test-app")
		}
	})

	t.Run("applies options in order", func(t *testing.T) {
		var order []string
		opt1 := func(a *App) { order = append(order, "opt1") }
		opt2 := func(a *App) { order = append(order, "opt2") }

		_ = newApp("test", opt1, opt2)

		if len(order) != 2 || order[0] != "opt1" || order[1] != "opt2" {
			t.Errorf("options applied in wrong order: %v", order)
		}
	})

	t.Run("returns non-nil app with no options", func(t *testing.T) {
		app := newApp("bare")
		if app == nil {
			t.Error("newApp() returned nil")
		}
	})
}

func TestApp(t *testing.T) {
	t.Run("Name returns the app name", func(t *testing.T) {
		app := newApp("svc-name")
		if got := app.Name(); got != "svc-name" {
			t.Errorf("app.Name() = %q, want %q", got, "svc-name")
		}
	})
}
