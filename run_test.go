package grove

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kusold/grove/config"
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

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SERVICE_NAME", "SERVICE_ENV", "SERVICE_VERSION", "HTTP_ADDR", "LOG_FORMAT"} {
		t.Setenv(key, "")
	}
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

		overrideName := func(b *builder) error {
			b.name = "overridden"
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
		opt1 := func(b *builder) error { return nil }
		opt2 := func(b *builder) error { return nil }

		err := Run(context.Background(), m, opt1, opt2)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("returns error when option fails", func(t *testing.T) {
		optErr := errors.New("option boom")
		failOpt := func(b *builder) error { return optErr }

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
		opt1 := func(b *builder) error { order = append(order, "opt1"); return nil }
		opt2 := func(b *builder) error { order = append(order, "opt2"); return nil }

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
		opt1 := func(b *builder) error { applied = append(applied, "opt1"); return nil }
		optErr := errors.New("boom")
		opt2 := func(b *builder) error { return optErr }
		opt3 := func(b *builder) error { applied = append(applied, "opt3"); return nil }

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

	t.Run("Config returns non-nil config", func(t *testing.T) {
		app, err := newApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg == nil {
			t.Fatal("app.Config() returned nil")
		}
	})

	t.Run("Config uses module name as default service name", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := newApp("my-service")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Name != "my-service" {
			t.Errorf("Config().Service().Name = %q, want %q", cfg.Service().Name, "my-service")
		}
	})

	t.Run("Config applies environment defaults", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := newApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Environment != "development" {
			t.Errorf("Config().Service().Environment = %q, want %q", cfg.Service().Environment, "development")
		}
		if cfg.Service().Version != "dev" {
			t.Errorf("Config().Service().Version = %q, want %q", cfg.Service().Version, "dev")
		}
		if cfg.HTTP().Addr != ":8080" {
			t.Errorf("Config().HTTP().Addr = %q, want %q", cfg.HTTP().Addr, ":8080")
		}
	})

	t.Run("Config SERVICE_NAME overrides module name", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "overridden")
		app, err := newApp("module-name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Name != "overridden" {
			t.Errorf("Config().Service().Name = %q, want %q", cfg.Service().Name, "overridden")
		}
		// Module identity is unchanged
		if app.Name() != "module-name" {
			t.Errorf("app.Name() = %q, want %q — module identity should not change", app.Name(), "module-name")
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
	t.Run("no capabilities is valid", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"})
		if err != nil {
			t.Fatalf("expected no error with no capabilities, got: %v", err)
		}
	})

	t.Run("http capability without deps is valid", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"},
			WithHTTP(),
		)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("missing dependencies fail with helpful errors", func(t *testing.T) {
		capDependency := capability("dependency")
		capDependent := capability("dependent")
		withCapabilityMetadata(t,
			[]capability{capDependency, capDependent},
			map[capability][]capability{capDependent: {capDependency}},
			map[capability]string{
				capDependency: "WithDependency",
				capDependent:  "WithDependent",
			},
			map[capability]string{
				capDependency: "dependency",
				capDependent:  "dependent",
			},
		)

		err := Run(context.Background(), testModule{name: "test"}, func(b *builder) error {
			b.enableCapability(capDependent)
			return nil
		})
		if err == nil {
			t.Fatal("expected dependency validation error")
		}
		if !strings.Contains(err.Error(), "dependent requires dependency") {
			t.Errorf("error = %q, want to contain 'dependent requires dependency'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithDependency()") {
			t.Errorf("error = %q, want to contain 'grove.WithDependency()'", err.Error())
		}
	})

	t.Run("dependencies may be provided after dependents", func(t *testing.T) {
		capDependency := capability("dependency")
		capDependent := capability("dependent")
		withCapabilityMetadata(t,
			[]capability{capDependency, capDependent},
			map[capability][]capability{capDependent: {capDependency}},
			map[capability]string{
				capDependency: "WithDependency",
				capDependent:  "WithDependent",
			},
			map[capability]string{
				capDependency: "dependency",
				capDependent:  "dependent",
			},
		)

		err := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capDependent)
				return nil
			},
			func(b *builder) error {
				b.enableCapability(capDependency)
				return nil
			},
		)
		if err != nil {
			t.Fatalf("expected no error when deps are provided in reverse order, got: %v", err)
		}
	})
}

func TestCapabilityDeterministicOrder(t *testing.T) {
	t.Run("validation error is consistent regardless of option order", func(t *testing.T) {
		capFirst := capability("first")
		capSecond := capability("second")
		capFirstDep := capability("first-dep")
		capSecondDep := capability("second-dep")
		withCapabilityMetadata(t,
			[]capability{capFirstDep, capFirst, capSecondDep, capSecond},
			map[capability][]capability{
				capFirst:  {capFirstDep},
				capSecond: {capSecondDep},
			},
			map[capability]string{
				capFirstDep:  "WithFirstDependency",
				capFirst:     "WithFirst",
				capSecondDep: "WithSecondDependency",
				capSecond:    "WithSecond",
			},
			map[capability]string{
				capFirstDep:  "first dependency",
				capFirst:     "first",
				capSecondDep: "second dependency",
				capSecond:    "second",
			},
		)

		err2 := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capSecond)
				b.enableCapability(capFirst)
				return nil
			},
		)
		err1 := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capFirst)
				b.enableCapability(capSecond)
				return nil
			},
		)

		if err1 == nil || err2 == nil {
			t.Fatal("expected both to fail with dependency errors")
		}

		if err1.Error() != err2.Error() {
			t.Errorf("errors should be identical regardless of option order:\n  err1: %s\n  err2: %s", err1, err2)
		}
		if !strings.Contains(err1.Error(), "first requires first dependency") {
			t.Errorf("expected first error to be about the first capability, got: %s", err1)
		}
	})
}

func TestRequireCapability(t *testing.T) {
	t.Run("returns error when capability not enabled", func(t *testing.T) {
		app, err := newApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = app.requireCapability(capHTTP)
		if err == nil {
			t.Fatal("expected error when http capability not enabled")
		}
		if !strings.Contains(err.Error(), "http capability is required but was not enabled") {
			t.Errorf("error = %q, want to contain 'http capability is required but was not enabled'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("returns nil when capability is enabled", func(t *testing.T) {
		app, err := newApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := app.requireCapability(capHTTP); err != nil {
			t.Errorf("expected nil error when capability is enabled, got: %v", err)
		}
	})
}

func TestAppLogger(t *testing.T) {
	t.Run("Logger returns non-nil slog.Logger", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := newApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if app.Logger() == nil {
			t.Fatal("app.Logger() returned nil")
		}
	})

	t.Run("Logger defaults to text format", func(t *testing.T) {
		clearConfigEnv(t)

		var buf bytes.Buffer
		cfg := config.Load("dev-svc")
		logger := newLogger(cfg, &buf)
		logger.Info("test message")

		output := buf.String()
		if strings.HasPrefix(output, "{") {
			t.Error("expected text output by default, got JSON")
		}
		if !strings.Contains(output, "service=dev-svc") {
			t.Errorf("output should contain service attribute, got: %s", output)
		}
		if !strings.Contains(output, "environment=development") {
			t.Errorf("output should contain environment attribute, got: %s", output)
		}
		if !strings.Contains(output, "version=dev") {
			t.Errorf("output should contain version attribute, got: %s", output)
		}
	})

	t.Run("Logger uses JSON format when LOG_FORMAT=json", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "json")
		t.Setenv("SERVICE_VERSION", "v1.0.0")

		var buf bytes.Buffer
		cfg := config.Load("json-svc")
		logger := newLogger(cfg, &buf)
		logger.Info("test message")

		var record map[string]any
		if err := json.Unmarshal([]byte(buf.String()), &record); err != nil {
			t.Fatalf("output should be valid JSON: %v, got: %s", err, buf.String())
		}
		if record["service"] != "json-svc" {
			t.Errorf("service = %v, want %q", record["service"], "json-svc")
		}
		if record["environment"] != "development" {
			t.Errorf("environment = %v, want %q", record["environment"], "development")
		}
		if record["version"] != "v1.0.0" {
			t.Errorf("version = %v, want %q", record["version"], "v1.0.0")
		}
	})

	t.Run("Logger uses text format when LOG_FORMAT=text", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "text")

		var buf bytes.Buffer
		cfg := config.Load("text-svc")
		logger := newLogger(cfg, &buf)
		logger.Info("test message")

		output := buf.String()
		if strings.HasPrefix(output, "{") {
			t.Error("expected text output, got JSON")
		}
		if !strings.Contains(output, "service=text-svc") {
			t.Errorf("output should contain service attribute, got: %s", output)
		}
	})
}

func withCapabilityMetadata(
	t *testing.T,
	order []capability,
	deps map[capability][]capability,
	optionNames map[capability]string,
	displayNames map[capability]string,
) {
	t.Helper()

	oldOrder := capabilityOrder
	oldDeps := capabilityDeps
	oldOptionNames := capabilityOptionName
	oldDisplayNames := capabilityDisplayName

	capabilityOrder = order
	capabilityDeps = deps
	capabilityOptionName = optionNames
	capabilityDisplayName = displayNames

	t.Cleanup(func() {
		capabilityOrder = oldOrder
		capabilityDeps = oldDeps
		capabilityOptionName = oldOptionNames
		capabilityDisplayName = oldDisplayNames
	})
}
