package grove

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/lifecycle"
)

// testModule is a minimal Module implementation for testing.
type testModule struct {
	name     string
	register func(ctx context.Context, app *App) error
}

func (m *testModule) Name() string { return m.name }

func (m *testModule) Register(ctx context.Context, app *App) error {
	if m.register != nil {
		return m.register(ctx, app)
	}
	return nil
}

func TestModuleInterface(t *testing.T) {
	m := &testModule{name: "test-svc"}
	if m.Name() != "test-svc" {
		t.Errorf("expected name 'test-svc', got %q", m.Name())
	}
}

func TestRun_NilModule(t *testing.T) {
	err := Run(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil module")
	}
}

func TestRun_ModuleRegistrationCalled(t *testing.T) {
	called := false
	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			called = true
			return nil
		},
	}

	// Use a context that cancels quickly so Run doesn't block.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := Run(ctx, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Register to be called")
	}
}

func TestRun_RegistrationError(t *testing.T) {
	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			return errors.New("registration failed")
		},
	}

	err := Run(context.Background(), m)
	if err == nil {
		t.Fatal("expected error from failed registration")
	}
}

func TestRun_OptionApplied(t *testing.T) {
	var capturedName string
	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			capturedName = app.Name()
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "test" {
		t.Errorf("expected app name 'test', got %q", capturedName)
	}
}

func TestRun_WithLoggerOption(t *testing.T) {
	logger := slog.Default()
	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			if app.Logger() != logger {
				t.Error("expected custom logger")
			}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, m, WithLogger(logger))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_WithConfigOption(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:            "custom",
			Environment:     "staging",
			Version:         "1.0",
			ShutdownTimeout: 5 * time.Second,
		},
		HTTP: config.HTTPConfig{
			Addr: ":9090",
		},
	}

	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			if app.Config().HTTP.Addr != ":9090" {
				t.Errorf("expected :9090, got %q", app.Config().HTTP.Addr)
			}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, m, WithConfig(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_OptionError(t *testing.T) {
	failOpt := Option(func(app *App) error {
		return errors.New("option failed")
	})

	m := &testModule{name: "test"}
	err := Run(context.Background(), m, failOpt)
	if err == nil {
		t.Fatal("expected error from failed option")
	}
}

func TestRun_LifecycleHooks(t *testing.T) {
	var order []string
	m := &testModule{
		name: "test",
		register: func(ctx context.Context, app *App) error {
			app.Lifecycle().Append(lifecycle.Hook{
				Name:  "hook1",
				Start: func(ctx context.Context) error { order = append(order, "start1"); return nil },
				Stop:  func(ctx context.Context) error { order = append(order, "stop1"); return nil },
			})
			app.Lifecycle().Append(lifecycle.Hook{
				Name:  "hook2",
				Start: func(ctx context.Context) error { order = append(order, "start2"); return nil },
				Stop:  func(ctx context.Context) error { order = append(order, "stop2"); return nil },
			})
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"start1", "start2", "stop2", "stop1"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("index %d: expected %q, got %q", i, v, order[i])
		}
	}
}
